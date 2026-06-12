// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0
package codegen

import (
	"fmt"
	"math"
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// StmtCompiler provides a working environment for compiling individual statements
// within a given function.  For example, it provides the ability to allocate
// new temporary registers as required.
type StmtCompiler struct {
	components  []Declaration
	variables   []VariableDescriptor
	registers   []register.Register
	environment data.ResolvedEnvironment
	field       field.Config
	srcmaps     source.Maps[any]
	errors      []source.SyntaxError
	// quiet suppresses printf output
	quiet bool
	// costReport records generated WIR micro-instruction counts for annotated statements.
	costReport *CostReport
	// function identifies the VM function currently being compiled.
	function uint
}

func (p *StmtCompiler) compileStatement(pc uint, mapping []uint, s Stmt) VectorInstruction {
	var insns []Instruction
	//
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		targets, pre, post := p.mapLVals(mapping, s.Targets)
		//
		insns = p.compileRootExprs(s.Source, mapping, targets...)
		// Configure pre/post instructions
		insns = append(pre, insns...)
		insns = append(insns, post...)
	case *stmt.Cost[symbol.Resolved]:
		vec := p.compileStatement(pc, mapping, s.Body)
		if p.costReport != nil {
			p.costReport.Add(s.Label, p.function, vec)
		}

		return vec
	case *stmt.IfGoto[symbol.Resolved]:
		return p.compileCondition(pc, s.Cond, mapping, s.Target)
	case *stmt.Goto[symbol.Resolved]:
		return instruction.NewVector[Instruction](instruction.NewJump(s.Target))
	case *stmt.Fail[symbol.Resolved]:
		return p.compileFail(mapping, s.Chunks, s.Arguments)
	case *stmt.Printf[symbol.Resolved]:
		if p.quiet {
			return instruction.NewVector[Instruction]()
		}
		//
		return p.compilePrintf(mapping, s.Chunks, s.Arguments)
	case *stmt.Return[symbol.Resolved]:
		return instruction.NewVector[Instruction](instruction.NewReturn())
	default:
		panic("unknown statement encountered")
	}
	//
	return instruction.NewVector(insns...)
}

// Map lvals down to their corresponding registers.  For example, consider the
// following:
//
// > struct tmp { x u32, y u32 }
// > ...
// > var t tmp > tmp = f(...)
//
// In this case, we want to "compile out" the struct, so we end up with this:
//
// > var tmp$x, tmp$y u32
// >
// > tmp$x, tmp$y = f(...)
//
// Here, we have compiled out variable tmp into two registers, one for each
// field.
func (p *StmtCompiler) mapLVals(mapping []uint, lvals []LVal) ([]register.Vector, []Instruction, []Instruction) {
	var (
		regs                []register.Vector
		preInsns, postInsns []Instruction
	)
	//
	for _, lv := range lvals {
		switch lv := lv.(type) {
		case *lval.Variable[symbol.Resolved]:
			var ids = make([]register.Id, len(lv.Ids))

			for j, id := range lv.Ids {
				ids[j] = register.NewId(id)
			}
			// reverse ids as NewDestruct expects them in little endian order
			ids = array.Reverse(ids)
			//
			regs = append(regs, register.NewVector(ids...))
		case *lval.MemAccess[symbol.Resolved]:
			var (
				ext = p.components[lv.Name.Index].(*decl.ResolvedMemory)
				// Determine vm module identifier
				id = mapping[lv.Name.Index]
			)
			if !ext.IsWriteable() {
				panic(fmt.Sprintf("unwritable memory \"%s\" encountered", ext.Name()))
			}
			//
			dataLines := make([]register.Id, len(ext.Data))
			addressLines, pre := p.compileNonUniformArgs(mapping, lv.Args...)
			// Allocate data lines as needed
			for j, t := range ext.Data {
				var bitwidth uint
				if t.DataType.AsField(p.environment) != nil {
					bitwidth = math.MaxUint
				} else {
					bitwidth, _ = data.BitWidthOf(t.DataType, p.environment)
				}

				dataLines[j] = p.allocate(bitwidth)
				regs = append(regs, register.NewVector(dataLines[j]))
			}
			//
			preInsns = append(preInsns, pre...)
			postInsns = append(postInsns, instruction.NewMemWrite(id, addressLines, dataLines))
		}
	}
	//
	return regs, preInsns, postInsns
}

func (p *StmtCompiler) compilePrintf(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) VectorInstruction {
	nchunks, insns := p.compileFormattedChunks(mapping, chunks, args)
	//
	insns = append(insns, &instruction.Debug{Chunks: nchunks})
	//
	return instruction.NewVector(insns...)
}

func (p *StmtCompiler) compileFail(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) VectorInstruction {
	//
	nchunks, insns := p.compileFormattedChunks(mapping, chunks, args)
	//
	insns = append(insns, instruction.NewFail(nchunks...))
	//
	return instruction.NewVector(insns...)
}

// compileFormattedChunks compiles each argument expression into a temporary
// register and pairs it with the corresponding format chunk.  Chunks without a
// format directive are passed through unchanged with an unused argument
// register.  Returns the resulting chunk list together with the
// micro-instructions needed to evaluate the arguments.
func (p *StmtCompiler) compileFormattedChunks(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) ([]instruction.FormattedChunk, []Instruction) {
	var (
		nchunks     []instruction.FormattedChunk
		regs, insns = p.compileNonUniformArgs(mapping, args...)
		index       uint
	)
	//
	for _, chunk := range chunks {
		if chunk.Format.HasFormat() {
			nchunks = append(nchunks, instruction.FormattedChunk{
				Text: chunk.Text, Format: chunk.Format, Argument: register.NewVector(regs[index]),
			})
			//
			index++
		} else {
			nchunks = append(nchunks, instruction.FormattedChunk{
				Text: chunk.Text, Format: util.EMPTY_FORMAT, Argument: register.NewVector(),
			})
		}
	}
	//
	return nchunks, insns
}

func (p *StmtCompiler) compileCondition(pc uint, e Condition, mapping []uint, target uint,
) VectorInstruction {
	switch e := e.(type) {
	case *expr.Cmp[symbol.Resolved]:
		var (
			args, insns = p.compileNonUniformArgs(mapping, e.Left, e.Right)
		)
		//
		insns = append(insns, instruction.NewSkipIf(opcode.Condition(e.Operator), args[0], args[1], 1))
		insns = append(insns, instruction.NewJump(pc+1))
		insns = append(insns, instruction.NewJump(target))
		//
		return instruction.NewVector(insns...)
	default:
		panic("unknown condition encountered")
	}
}

func (p *StmtCompiler) compileRootExprs(e Expr, mapping []uint, targets ...register.Vector) []Instruction {
	switch e := e.(type) {
	case *expr.TupleInitialiser[symbol.Resolved]:
		return p.compileTupleInitialiser(e, mapping, targets...)
	case *expr.ExternAccess[symbol.Resolved]:
		//
		switch ext := p.components[e.Name.Index].(type) {
		case *decl.ResolvedConstant:
			// fall through
		case *decl.ResolvedMemory:
			if !ext.IsReadable() {
				panic(fmt.Sprintf("unreadable memory \"%s\" encountered", e.Name.String()))
			}
			//
			return destructMultiway(p, e, mapping, targets, p.compileMemoryRead)
		case *decl.ResolvedFunction:
			// Calls to #[debug] functions are elided in quiet mode, exactly as
			// printf statements are.  Such functions cannot return values or
			// write memories (enforced by validate.DebugFunctions), so
			// dropping the call has no effect on the surrounding computation.
			if p.quiet && slices.Contains(ext.Annotations(), "debug") {
				return nil
			}
			//
			return destructMultiway(p, e, mapping, targets, p.compileFunctionCall)
		default:
			panic(fmt.Sprintf("unknown symbol \"%s\" encountered", e.Name.String()))
		}
	}
	// unit expression
	if len(targets) != 1 {
		panic(fmt.Sprintf("unit expression cannot have %d targets", len(targets)))
	}
	//
	return p.compileRootExpr(e, mapping, targets[0])
}

// A root expression is one which arises from a "concrete" target.  For example,
// "e" is a root expression in "x = e", and also "x = 1 + f(e)".  But, e is not
// a root expression in "x = 1 + e".
func (p *StmtCompiler) compileRootExpr(e Expr, mapping []uint, targets register.Vector) []Instruction {
	var bitwidth uint
	//
	if e.Type().AsField(p.environment) != nil {
		// Field-typed sub-expression — allocate a native register.
		bitwidth = math.MaxUint
	} else {
		bitwidth, _ = data.BitWidthOf(e.Type(), p.environment)
	}
	//
	return p.compileExpr(e, bitwidth, mapping, targets)
}

func (p *StmtCompiler) compileExpr(e Expr, bitwidth uint, mapping []uint, targets register.Vector) []Instruction {
	switch e := e.(type) {
	case *expr.Add[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldAdd(e.Exprs, mapping, targets.AsRegister())
		} else {
			return p.compileIntAdd(e.Exprs, bitwidth, mapping, targets)
		}
	case *expr.Cast[symbol.Resolved]:
		return p.compileRootExpr(e.Expr, mapping, targets)
	case *expr.Concat[symbol.Resolved]:
		return p.compileConcat(e.Exprs, bitwidth, mapping, targets)
	case *expr.BitwiseAnd[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileBitwiseAnd)
	case *expr.Const[symbol.Resolved]:
		var c vm.Uint
		//
		if p.isFieldOperation(targets) {
			return p.compileFieldConst(c.SetBigInt(e.Constant()), mapping, targets.AsRegister())
		} else {
			return p.compileIntConst(c.SetBigInt(e.Constant()), mapping, targets)
		}
	case *expr.ExternAccess[symbol.Resolved]:
		//
		if _, ok := p.components[e.Name.Index].(*decl.ResolvedConstant); ok {
			return p.compileIntConst(p.evalConstant(e), mapping, targets)
		}
		// memory access or function call
		return p.compileRootExprs(e, mapping, targets)
	case *expr.LocalAccess[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldAccess(e, mapping, targets.AsRegister())
		} else {
			return p.compileLocalAccess(e, mapping, targets)
		}
	case *expr.ArrayAccess[symbol.Resolved]:
		return p.compileArrayAccess(e, mapping, targets)
	case *expr.Mul[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldMul(e.Exprs, mapping, targets.AsRegister())
		} else {
			return p.compileIntMul(e.Exprs, bitwidth, mapping, targets)
		}
	case *expr.BitwiseNot[symbol.Resolved]:
		return destructUnit(p, e, bitwidth, mapping, targets, p.compileBitwiseNot)
	case *expr.BitwiseOr[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileBitwiseOr)
	case *expr.Div[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileIntDiv)
	case *expr.Rem[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileIntRem)
	case *expr.Shl[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileBitwiseShl)
	case *expr.Shr[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileBitwiseShr)
	case *expr.Sub[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldSub(e.Exprs, mapping, targets.AsRegister())
		} else {
			return p.compileIntSub(e.Exprs, bitwidth, mapping, targets)
		}
	case *expr.Xor[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth, mapping, targets, p.compileBitwiseXor)
	case *expr.Ternary[symbol.Resolved]:
		return p.compileTernary(e, bitwidth, mapping, targets)
	default:
		panic("unknown expression encountered")
	}
}

// UnitTranslator is for unit instructions which cannot target a vector
// instruction.
type UnitTranslator[T any] = func(T, uint, []uint, register.Id) []Instruction

// MultiTranslator is for multi-way instructions which cannot target a vector
// instruction.
type MultiTranslator[T any] = func(T, []uint, []register.Id) []Instruction

// Wrap a translator for a unit instruction which cannot target vectors (for
// whatever reason).  Essentially, this allocates fresh registers as required to
// handle any destructs encountered.
func destructUnit[T any](p *StmtCompiler, args T, bitwidth uint, mapping []uint, target register.Vector,
	fn UnitTranslator[T]) []Instruction {
	// Check for non-vector situation
	if target.Len() == 1 {
		return fn(args, bitwidth, mapping, target.AsRegister())
	}
	// Allocate temporary
	tmp := p.allocate(bitwidth)
	// Translate expression
	insns := fn(args, bitwidth, mapping, tmp)
	// Generate destruct
	return append(insns, instruction.UintDestruct[vm.Uint](target, tmp))
}

func destructMultiway[T any](p *StmtCompiler, args T, mapping []uint, targets []register.Vector, fn MultiTranslator[T],
) []Instruction {
	var tmps = make([]register.Id, len(targets))
	//
	for i, v := range targets {
		var bitwidth = p.bitwidthOf(v)
		//
		if v.Len() == 1 {
			tmps[i] = v.AsRegister()
		} else {
			// Allocate temporary
			tmps[i] = p.allocate(bitwidth)
		}
	}
	// Translate expression
	insns := fn(args, mapping, tmps)
	//  Generate destruct(s)
	for i, v := range targets {
		if v.Len() != 1 {
			insns = append(insns, instruction.UintDestruct[vm.Uint](v, tmps[i]))
		}
	}
	//
	return insns
}

// check whether this is a field operation, or not.
func (p *StmtCompiler) isFieldOperation(target register.Vector) bool {
	for _, r := range target.Registers() {
		if p.registers[r.Unwrap()].IsNative() {
			return true
		}
	}

	return false
}

func (p *StmtCompiler) compileTernary(e *expr.Ternary[symbol.Resolved], bitwidth uint, mapping []uint,
	target register.Vector) []Instruction {
	//
	cmp := e.Cond.(*expr.Cmp[symbol.Resolved])
	// Lazily compile both arms — their instructions are placed inside the
	// conditionally-skipped regions below, so only the taken arm runs.
	trueInsns := p.compileExpr(e.IfTrue, bitwidth, mapping, target)
	falseInsns := p.compileExpr(e.IfFalse, bitwidth, mapping, target)
	// Evaluate condition operands (always runs).
	condRegs, condInsns := p.compileNonUniformArgs(mapping, cmp.Left, cmp.Right)
	// Selection sequence:
	//   condInsns                                  always
	//   skip_if(cond, lhs, rhs, |falseInsns|+2)    if TRUE skip false arm
	//   falseInsns                                 (skipped on TRUE)
	//   skip(|trueInsns|+1)                        jump past true arm
	//   trueInsns                                  (skipped on FALSE)
	insns := append([]Instruction{}, condInsns...)
	insns = append(insns, instruction.NewSkipIf(
		opcode.Condition(cmp.Operator), condRegs[0], condRegs[1], uint(len(falseInsns))+1))
	insns = append(insns, falseInsns...)
	insns = append(insns, &instruction.Skip{Skip: uint(len(trueInsns))})
	//
	return append(insns, trueInsns...)
}

func (p *StmtCompiler) compileTupleInitialiser(e *expr.TupleInitialiser[symbol.Resolved], mapping []uint,
	targets ...register.Vector) (insns []Instruction) {
	// NOTE: we assume the right number of targets for the initialiser here, and
	// that this was checked earlier in the pipeline.
	for i, target := range targets {
		var (
			ith      = e.Exprs[i]
			bitwidth uint
		)
		//
		if ith.Type().AsField(p.environment) != nil {
			// Field-typed sub-expression — allocate a native register.
			bitwidth = math.MaxUint
		} else {
			bitwidth, _ = data.BitWidthOf(e.Type(), p.environment)
		}
		//
		insns = append(insns, p.compileExpr(ith, bitwidth, mapping, target)...)
	}
	//
	return insns
}

func (p *StmtCompiler) compileIntConst(c vm.Uint, _ []uint, target register.Vector,
) []Instruction {
	//
	return []Instruction{instruction.UintAddV(target, nil, c)}
}

func (p *StmtCompiler) compileFieldConst(c vm.Uint, _ []uint, target register.Id,
) []Instruction {
	//
	return []Instruction{instruction.UintAddModP(target, nil, c)}
}

func (p *StmtCompiler) compileConcat(args []Expr, bitwidth uint, mapping []uint, target register.Vector) []Instruction {
	var nargs []Expr
	//
	nargs = append(nargs, args...)
	// Compile arguments
	sources, insns := p.compileNonUniformArgs(mapping, nargs...)
	// Reverse sources (as NewBitConcat requires them in little endian order)
	sources = array.Reverse(sources)
	// Done
	return append(insns, instruction.BitConcatV[vm.Uint](target, sources))
}

func (p *StmtCompiler) compileIntAdd(args []Expr, bitwidth uint, mapping []uint, target register.Vector) []Instruction {
	//
	var (
		constant vm.Uint
		nargs    []Expr
		w        vm.Uint
	)
	//
	for _, e := range args {
		var overflow bool
		//
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
			constant, overflow = constant.Add(w.SetBigInt(c.Constant()))
		} else if p.isConstantAccess(e) {
			constant, overflow = constant.Add(p.evalConstant(e))
		} else {
			nargs = append(nargs, e)
		}
		// NOTE: this error should be caught and reported earlier in the
		// pipeline.
		if overflow || !constant.FitsWithin(bitwidth) {
			panic("arithmetic overflow")
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, nargs...)
	// Done
	return append(insns, instruction.UintAddV(target, sources, constant))
}

func (p *StmtCompiler) compileFieldAdd(args []Expr, mapping []uint, target register.Id) []Instruction {
	//
	var (
		constant vm.Uint
		nargs    []Expr
		w        vm.Uint
		modulus  vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for _, e := range args {
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
			constant = constant.AddMod(w.SetBigInt(c.Constant()), modulus)
		} else if p.isConstantAccess(e) {
			constant = constant.AddMod(p.evalConstant(e), modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(math.MaxUint, mapping, nargs...)
	// Done
	return append(insns, instruction.UintAddModP(target, sources, constant))
}

func (p *StmtCompiler) compileFunctionCall(e *expr.ExternAccess[symbol.Resolved], mapping []uint,
	returns []register.Id) []Instruction {
	var (
		// Determine vm module identifier
		id = mapping[e.Name.Index]
	)
	// Compile arguments
	arguments, insns := p.compileNonUniformArgs(mapping, e.Args...)
	// determine type of read
	return append(insns, instruction.NewCall(id, arguments, returns))
}

func (p *StmtCompiler) compileLocalAccess(e *expr.LocalAccess[symbol.Resolved], _ []uint, target register.Vector,
) []Instruction {
	return []Instruction{instruction.UintAssignV[vm.Uint](target, register.NewId(e.Variable))}
}

func (p *StmtCompiler) compileFieldAccess(e *expr.LocalAccess[symbol.Resolved], _ []uint, target register.Id,
) []Instruction {
	var (
		zero vm.Uint
		reg  = []register.Id{register.NewId(e.Variable)}
	)
	//
	return []Instruction{instruction.UintAddModP(target, reg, zero)}
}

func (p *StmtCompiler) compileArrayAccess(e *expr.ArrayAccess[symbol.Resolved], mapping []uint, target register.Vector,
) []Instruction {
	panic(fmt.Sprintf("unexpected ArrayAccess node reached codegen (variable %d)", e.Id))
}

func (p *StmtCompiler) compileMemoryRead(e *expr.ExternAccess[symbol.Resolved], mapping []uint,
	data []register.Id) []Instruction {
	var (
		// Determine vm module identifier
		id = mapping[e.Name.Index]
	)
	// Compile arguments
	address, insns := p.compileNonUniformArgs(mapping, e.Args...)
	// determine type of read
	return append(insns, instruction.NewMemRead(id, address, data))
}

func (p *StmtCompiler) compileIntMul(args []Expr, bitwidth uint, mapping []uint, target register.Vector,
) []Instruction {
	//
	var (
		constant vm.Uint = vm.Const64[vm.Uint](1)
		nargs    []Expr
		w        vm.Uint
	)
	//
	for _, e := range args {
		var overflow bool
		//
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
			constant, overflow = constant.Mul(w.SetBigInt(c.Constant()))
		} else if p.isConstantAccess(e) {
			constant, overflow = constant.Mul(p.evalConstant(e))
		} else {
			nargs = append(nargs, e)
		}
		// NOTE: this error should be caught and reported earlier in the
		// pipeline.
		if overflow || !constant.FitsWithin(bitwidth) {
			panic("arithmetic overflow")
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, nargs...)
	//
	return append(insns, instruction.UintMulV(target, sources, constant))
}

func (p *StmtCompiler) compileFieldMul(args []Expr, mapping []uint, target register.Id,
) []Instruction {
	//
	var (
		constant   vm.Uint = vm.Const64[vm.Uint](1)
		nargs      []Expr
		w, modulus vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for _, e := range args {
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
			constant = constant.MulMod(w.SetBigInt(c.Constant()), modulus)
		} else if p.isConstantAccess(e) {
			constant = constant.MulMod(p.evalConstant(e), modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(math.MaxUint, mapping, nargs...)
	// Done
	return append(insns, instruction.UintMulModP(target, sources, constant))
}

func (p *StmtCompiler) compileIntDiv(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Fold constant divisors: a/b/2/c/3 == a/b/c/6.
	var (
		product = big.NewInt(1)
		nargs   = []Expr{args[0]}
	)
	// args[0] is the dividend — never fold it.
	for _, e := range args[1:] {
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
			product.Mul(product, c.Constant())

			if uint(product.BitLen()) > bitwidth {
				msg := fmt.Sprintf("constant divisors overflow u%d", bitwidth)
				p.errors = append(p.errors, p.srcmaps.SyntaxErrors(c, msg)...)

				break
			}
		} else if p.isConstantAccess(e) {
			product.Mul(product, p.evalConstant(e).BigInt())

			if uint(product.BitLen()) > bitwidth {
				msg := fmt.Sprintf("constant divisors overflow u%d", bitwidth)
				p.errors = append(p.errors, p.srcmaps.SyntaxErrors(e, msg)...)

				break
			}
		} else {
			nargs = append(nargs, e)
		}
	}

	if product.Cmp(big.NewInt(1)) != 0 {
		nargs = append(nargs, expr.NewTypedConstant[symbol.Resolved](*product, 10, bitwidth))
	}

	if len(nargs) < 2 {
		p.errors = append(p.errors, p.srcmaps.SyntaxErrors(args[0], "division has no divisor")...)
	}

	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bitwidth, mapping, nargs...)
	// Chain divisions left-to-right: (((a / b) / c) / ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.UintDiv[vm.Uint](bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.UintDiv[vm.Uint](bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileIntRem(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain remainders left-to-right: (((a % b) % c) % ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.UintRem(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.UintRem(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileBitwiseShl(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain shifts left-to-right: (((a << b) << c) << ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.BitShl(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.BitShl(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileBitwiseShr(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain shifts left-to-right: (((a >> b) >> c) >> ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.BitShr(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.BitShr(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileIntSub(args []Expr, bitwidth uint, mapping []uint, target register.Vector,
) []Instruction {
	//
	var (
		constant vm.Uint
		nargs    []Expr
		w        vm.Uint
	)
	//
	for i, e := range args {
		var overflow bool

		if c, ok := e.(*expr.Const[symbol.Resolved]); ok && i > 0 {
			constant, overflow = constant.Add(w.SetBigInt(c.Constant()))
		} else if p.isConstantAccess(e) && i > 0 {
			constant, overflow = constant.Add(p.evalConstant(e))
		} else {
			nargs = append(nargs, e)
		}
		// NOTE: this error should be caught and reported earlier in the
		// pipeline.
		if overflow || !constant.FitsWithin(bitwidth) {
			panic("arithmetic underflow")
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, nargs...)
	// Done
	return append(insns, instruction.UintSubV(target, sources, constant))
}

func (p *StmtCompiler) compileFieldSub(args []Expr, mapping []uint, target register.Id,
) []Instruction {
	//
	var (
		constant   vm.Uint
		nargs      []Expr
		w, modulus vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for i, e := range args {
		if c, ok := e.(*expr.Const[symbol.Resolved]); ok && i > 0 {
			constant = constant.AddMod(w.SetBigInt(c.Constant()), modulus)
		} else if p.isConstantAccess(e) && i > 0 {
			constant = constant.AddMod(p.evalConstant(e), modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(math.MaxUint, mapping, nargs...)
	// Done
	return append(insns, instruction.UintSubModP(target, sources, constant))
}

func (p *StmtCompiler) compileBitwiseAnd(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain left-to-right: (((a & b) & c) & ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.BitAnd(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.BitAnd(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileBitwiseNot(e *expr.BitwiseNot[symbol.Resolved], bitwidth uint, mapping []uint,
	target register.Id) []Instruction {
	//
	sources, insns := p.compileUniformArgs(bitwidth, mapping, e.Expr)
	//
	return append(insns, instruction.BitNot(bitwidth, target, sources[0]))
}

func (p *StmtCompiler) compileBitwiseOr(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain left-to-right: (((a | b) | c) | ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.BitOr(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.BitOr(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileBitwiseXor(args []Expr, bitwidth uint, mapping []uint, target register.Id,
) []Instruction {
	// Compile arguments
	sources, insns := p.compileUniformArgs(bitwidth, mapping, args...)
	// Chain left-to-right: (((a ^ b) ^ c) ^ ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bitwidth)
		insns = append(insns, instruction.BitXor(bitwidth, tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns, instruction.BitXor(bitwidth, target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileUniformArgs(bitwidth uint, mapping []uint, exprs ...Expr) ([]register.Id, []Instruction) {
	var (
		insns   []Instruction
		targets = make([]register.Id, len(exprs))
	)
	//
	for i, e := range exprs {
		//
		if r, ok := e.(*expr.LocalAccess[symbol.Resolved]); ok {
			targets[i] = register.NewId(r.Variable)
		} else {
			// Allocate temporary variable
			targets[i] = p.allocate(bitwidth)
			// Compile expression, storing result in temporary
			insns = append(insns, p.compileExpr(e, bitwidth, mapping, register.NewVector(targets[i]))...)
		}
	}
	//
	return targets, insns
}

func (p *StmtCompiler) compileNonUniformArgs(mapping []uint, exprs ...Expr) ([]register.Id, []Instruction) {
	var (
		insns   []Instruction
		targets = make([]register.Id, len(exprs))
	)
	//
	for i, e := range exprs {
		//
		if r, ok := e.(*expr.LocalAccess[symbol.Resolved]); ok {
			targets[i] = register.NewId(r.Variable)
		} else {
			var bitwidth uint
			//
			if e.Type().AsField(p.environment) != nil {
				// Field-typed sub-expression — allocate a native register.
				bitwidth = math.MaxUint
			} else {
				bitwidth, _ = data.BitWidthOf(e.Type(), p.environment)
			}
			// Allocate temporary variable
			targets[i] = p.allocate(bitwidth)
			// Compile expression, storing result in temporary
			insns = append(insns, p.compileExpr(e, bitwidth, mapping, register.NewVector(targets[i]))...)
		}
	}
	//
	return targets, insns
}

func (p *StmtCompiler) evalConstant(e Expr) vm.Uint {
	var (
		evaluator   = NewConstantEvaluator(p.field, p.environment, p.components...)
		res, errMsg = evaluator.Eval(e, false)
	)
	//
	if errMsg != "" {
		p.errors = append(p.errors, p.srcmaps.SyntaxErrors(e, errMsg)...)
	}
	//
	return res
}

func (p *StmtCompiler) allocate(bitwidth uint) register.Id {
	var (
		padding big.Int
		n       = len(p.registers)
		name    = fmt.Sprintf("$%d", n)
	)
	//
	p.registers = append(p.registers, register.NewComputed(name, bitwidth, padding))
	//
	return register.NewId(uint(n))
}

// bitwidthOf returns the bit-width to use when folding compile-time
// constants into a target register.  For integer-typed targets this is the
// register's declared width; for field-typed (native) targets this is the
// configured field bandwidth, since field elements have no fixed bit-width
// and only need enough room to hold a representative.
func (p *StmtCompiler) bitwidthOf(target register.Vector) uint {
	var bitwidth uint
	//
	for _, r := range target.Registers() {
		ith := p.registers[r.Unwrap()]
		//
		if ith.IsNative() && target.Len() == 1 {
			return math.MaxUint
		} else if ith.IsNative() {
			panic("cannot destructure field elements")
		}
		//
		bitwidth += ith.Width()
	}
	//
	return bitwidth
}

func (p *StmtCompiler) isConstantAccess(e Expr) bool {
	ne, ok := e.(*expr.ExternAccess[symbol.Resolved])
	//
	if !ok {
		return false
	}
	// Check whethe ris constant
	_, ok = p.components[ne.Name.Index].(*decl.ResolvedConstant)
	//
	return ok
}
