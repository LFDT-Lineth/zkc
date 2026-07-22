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
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// RegisterId is the register identifier used throughout codegen.  It is the
// bytecode-layer RegisterId, so compiled instructions need no register-type
// conversion.
type RegisterId = vm.RegisterId

// StmtCompiler provides a working environment for compiling individual statements
// within a given function.  For example, it provides the ability to allocate
// new temporary registers as required.
type StmtCompiler struct {
	components  []Declaration
	variables   []VariableDescriptor
	registers   []vm.Register[vm.Uint]
	environment data.ResolvedEnvironment
	field       field.Config
	srcmaps     source.Maps[any]
	errors      []source.SyntaxError
	// quiet suppresses printf output
	quiet bool
	// fastMode disables constraint-only rewrites not required by the vm.
	fastMode bool
}

func (p *StmtCompiler) compileStatement(pc uint, mapping []uint, s Stmt) BytecodeVector {
	var insns []Bytecode
	//
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		targets, pre, post := p.mapLVals(mapping, s.Targets)
		insns = p.compileRootExprs(s.Source, mapping, targets...)
		// Configure pre/post instructions
		insns = append(pre, insns...)
		insns = append(insns, post...)
	case *stmt.IfGoto[symbol.Resolved]:
		return p.compileCondition(pc, s.Cond, mapping, s.Target)
	case *stmt.Dispatch[symbol.Resolved]:
		return p.compileDispatch(s, mapping)
	case *stmt.Goto[symbol.Resolved]:
		return vm.NewBytecodeVector(vm.Jump[vm.Uint](vm.Address(s.Target)))
	case *stmt.Fail[symbol.Resolved]:
		return p.compileFail(mapping, s.Chunks, s.Arguments)
	case *stmt.Printf[symbol.Resolved]:
		if p.quiet {
			return vm.NewBytecodeVector[vm.Uint]()
		}
		//
		return p.compilePrintf(mapping, s.Chunks, s.Arguments)
	case *stmt.Return[symbol.Resolved]:
		return vm.NewBytecodeVector(vm.Return[vm.Uint]())
	default:
		panic("unknown statement encountered")
	}
	//
	return vm.NewBytecodeVector(insns...)
}

// Map lvals down to their corresponding registers.  For example, consider the
// following:
//
// > struct tmp { x u32, y u32 }
// > ...
// > var t tmp
// > tmp = f(...)
//
// In this case, we want to "compile out" the struct, so we end up with this:
//
// > var tmp$x, tmp$y u32
// >
// > tmp$x, tmp$y = f(...)
//
// Here, we have compiled out variable tmp into two registers, one for each
// field.
func (p *StmtCompiler) mapLVals(mapping []uint, lvals []LVal) ([][]vm.RegisterId, []Bytecode, []Bytecode) {
	var (
		regs                [][]vm.RegisterId
		preInsns, postInsns []Bytecode
	)
	//
	for _, lv := range lvals {
		switch lv := lv.(type) {
		case *lval.Variable[symbol.Resolved]:
			var ids = make([]RegisterId, len(lv.Ids))

			for j, id := range lv.Ids {
				ids[j] = util.Cast[RegisterId](id)
			}
			// reverse ids as NewDestruct expects them in little endian order
			ids = array.Reverse(ids)
			//
			regs = append(regs, ids)
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
			dataLines := make([]RegisterId, len(ext.Data))
			addressLines, pre := p.compileNonUniformArgs(mapping, lv.Args...)
			// Allocate data lines as needed
			for j, t := range ext.Data {
				var bitwidth = data.BitWidthOf(t.DataType, p.environment)
				//
				dataLines[j] = p.allocate(bitwidth)
				regs = append(regs, []vm.RegisterId{dataLines[j]})
			}
			//
			preInsns = append(preInsns, pre...)
			// Emit the write bytecode.  The memory kind is resolved from the
			// environment at encode time; any outgoing cast checks on the data
			// lines are inserted later by the check-cast pass.
			postInsns = append(postInsns, vm.MemWrite[vm.Uint](uint16(id), addressLines, dataLines))
		}
	}
	//
	return regs, preInsns, postInsns
}

func (p *StmtCompiler) compilePrintf(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) BytecodeVector {
	nchunks, sources, insns := p.compileFormattedChunks(mapping, chunks, args)
	//
	insns = append(insns, vm.Debug[vm.Uint](nchunks, sources))
	//
	return vm.NewBytecodeVector(insns...)
}

func (p *StmtCompiler) compileFail(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) BytecodeVector {
	//
	nchunks, sources, insns := p.compileFormattedChunks(mapping, chunks, args)
	//
	insns = append(insns, vm.Fail[vm.Uint](nchunks, sources))
	//
	return vm.NewBytecodeVector(insns...)
}

// compileFormattedChunks compiles each argument expression into a temporary
// register and pairs it with the corresponding format chunk.  Chunks without a
// format directive are passed through unchanged with an unused argument
// register.  Returns the resulting chunk list together with the
// micro-instructions needed to evaluate the arguments.
func (p *StmtCompiler) compileFormattedChunks(mapping []uint, chunks []stmt.FormattedChunk, args []Expr,
) ([]vm.FormattedChunk, []vm.RegisterId, []Bytecode) {
	var (
		nchunks     []vm.FormattedChunk
		regs, insns = p.compileNonUniformArgs(mapping, args...)
		index       uint
	)
	//
	for _, chunk := range chunks {
		if chunk.Format.HasFormat() {
			nchunks = append(nchunks, vm.NewFormattedChunk(chunk.Text, chunk.Format))
			//
			index++
		} else {
			nchunks = append(nchunks, vm.NewFormattedChunk(chunk.Text, zkc_util.EMPTY_FORMAT))
		}
	}
	//
	return nchunks, regs, insns
}

func (p *StmtCompiler) compileCondition(pc uint, e Condition, mapping []uint, target uint,
) BytecodeVector {
	switch e := e.(type) {
	case *expr.Cmp[symbol.Resolved]:
		var (
			args, insns = p.compileNonUniformArgs(mapping, e.Left, e.Right)
		)
		//
		insns = append(insns, vm.SkipIf[vm.Uint](vm.Cond(e.Operator), 1, args[0], args[1]))
		insns = append(insns, vm.Jump[vm.Uint](vm.Address(pc+1)))
		insns = append(insns, vm.Jump[vm.Uint](vm.Address(target)))
		//
		return vm.NewBytecodeVector(insns...)
	default:
		panic("unknown condition encountered")
	}
}

// compileDispatch compiles a multiway dispatch into a single vector
// instruction.  The discriminant is evaluated into a register, then a
// MultiwaySkip selects between a jump table laid out immediately after it: the
// default jump (reached on no match) followed by one jump per branch.  Every
// label of branch i therefore skips by i+1 to land on that branch's jump.
func (p *StmtCompiler) compileDispatch(s *stmt.Dispatch[symbol.Resolved], mapping []uint) BytecodeVector {
	// Evaluate the discriminant into a single register.
	sources, insns := p.compileNonUniformArgs(mapping, s.Discriminant)
	source := sources[0]
	// Build the dispatch table from the (constant) case labels.
	var cases []vm.SwitchCase[vm.Uint]
	//
	for i, branch := range s.Branches {
		for _, label := range branch.Labels {
			value := p.evalConstant(label)
			// The multiway skip compares against a uint64 immediate.
			if !value.FitsWithin(64) {
				p.errors = append(p.errors,
					p.srcmaps.SyntaxErrors(label, "switch value too large for multiway dispatch")...)
				//
				continue
			}
			//
			cases = append(cases, vm.SwitchCase[vm.Uint]{
				Value: vm.Const64[vm.Uint](value.Uint64()), Skip: uint16(i + 1)})
		}
	}
	// Emit the dispatch followed by its jump table (default first).
	insns = append(insns, vm.Switch(source, cases))
	insns = append(insns, vm.Jump[vm.Uint](vm.Address(s.DefaultTarget)))
	//
	for _, branch := range s.Branches {
		insns = append(insns, vm.Jump[vm.Uint](vm.Address(branch.Target)))
	}
	//
	return vm.NewBytecodeVector(insns...)
}

func (p *StmtCompiler) compileRootExprs(e Expr, mapping []uint, targets ...[]vm.RegisterId) []Bytecode {
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
			// printf statements are.  Such functions are elided as well.
			// Note that they cannot return values or
			// write memories (enforced by validate.DebugFunctions).
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
func (p *StmtCompiler) compileRootExpr(e Expr, mapping []uint, targets []vm.RegisterId) []Bytecode {
	var bitwidth = data.BitWidthOf(e.Type(), p.environment)
	//
	return p.compileExpr(e, bitwidth, mapping, targets)
}

func (p *StmtCompiler) compileExpr(e Expr, bitwidth util.Option[uint], mapping []uint, targets []vm.RegisterId,
) []Bytecode {
	//
	switch e := e.(type) {
	case *expr.Add[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldAdd(e.Exprs, mapping, targets[0])
		} else {
			return p.compileIntAdd(e.Exprs, bitwidth.Unwrap(), mapping, targets)
		}
	case *expr.Cast[symbol.Resolved]:
		return p.compileCast(e, bitwidth, mapping, targets)
	case *expr.Concat[symbol.Resolved]:
		return p.compileConcat(e.Exprs, mapping, targets)
	case *expr.BitwiseAnd[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseAnd)
	case *expr.Const[symbol.Resolved]:
		var c vm.Uint
		//
		if p.isFieldOperation(targets) {
			return p.compileFieldConst(c.SetBigInt(e.Constant()), mapping, targets[0])
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
			return p.compileFieldAccess(e, mapping, targets[0])
		} else {
			return p.compileLocalAccess(e, mapping, targets)
		}
	case *expr.ArrayAccess[symbol.Resolved]:
		return p.compileArrayAccess(e, mapping, targets)
	case *expr.Mul[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldMul(e.Exprs, mapping, targets[0])
		} else {
			return p.compileIntMul(e.Exprs, bitwidth.Unwrap(), mapping, targets)
		}
	case *expr.BitwiseNot[symbol.Resolved]:
		return destructUnit(p, e, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseNot)
	case *expr.BitwiseOr[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseOr)
	case *expr.Div[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileIntDiv)
	case *expr.Rem[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileIntRem)
	case *expr.Shl[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseShl)
	case *expr.Shr[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseShr)
	case *expr.Sub[symbol.Resolved]:
		if p.isFieldOperation(targets) {
			return p.compileFieldSub(e.Exprs, mapping, targets[0])
		} else {
			return p.compileIntSub(e.Exprs, bitwidth.Unwrap(), mapping, targets)
		}
	case *expr.Xor[symbol.Resolved]:
		return destructUnit(p, e.Exprs, bitwidth.Unwrap(), mapping, targets, p.compileBitwiseXor)
	case *expr.Ternary[symbol.Resolved]:
		return p.compileTernary(e, bitwidth, mapping, targets)
	default:
		panic("unknown expression encountered")
	}
}

// UnitTranslator is for unit instructions which cannot target a vector
// instruction.
type UnitTranslator[T any] = func(T, uint, []uint, RegisterId) []Bytecode

// MultiTranslator is for multi-way instructions which cannot target a vector
// instruction.
type MultiTranslator[T any] = func(T, []uint, []RegisterId) []Bytecode

// Wrap a translator for a unit instruction which cannot target vectors (for
// whatever reason).  Essentially, this allocates fresh registers as required to
// handle any destructs encountered.
func destructUnit[T any](p *StmtCompiler, args T, bitwidth uint, mapping []uint, targets []vm.RegisterId,
	fn UnitTranslator[T]) []Bytecode {
	// Check for non-vector situation
	if len(targets) == 1 {
		return fn(args, bitwidth, mapping, targets[0])
	}
	// Allocate temporary
	tmp := p.allocate(util.Some(bitwidth))
	// Translate expression
	insns := fn(args, bitwidth, mapping, tmp)
	// Generate destruct
	return append(insns, vm.AddVec[vm.Uint](targets, []RegisterId{tmp}))
}

func destructMultiway[T any](p *StmtCompiler, args T, mapping []uint, targets [][]vm.RegisterId, fn MultiTranslator[T],
) []Bytecode {
	var tmps = make([]RegisterId, len(targets))
	//
	for i, v := range targets {
		var bitwidth = p.bitwidthOf(v...)
		//
		if len(v) == 1 {
			tmps[i] = v[0]
		} else {
			// Allocate temporary
			tmps[i] = p.allocate(bitwidth)
		}
	}
	// Translate expression
	insns := fn(args, mapping, tmps)
	//  Generate destruct(s)
	for i, v := range targets {
		if len(v) != 1 {
			insns = append(insns, vm.AddVec[vm.Uint](v, []RegisterId{tmps[i]}))
		}
	}
	//
	return insns
}

// check whether this is a field operation, or not.
func (p *StmtCompiler) isFieldOperation(targets []vm.RegisterId) bool {
	for _, r := range targets {
		if p.registers[r].IsNative() {
			return true
		}
	}

	return false
}

func (p *StmtCompiler) compileTernary(e *expr.Ternary[symbol.Resolved], bitwidth util.Option[uint], mapping []uint,
	target []vm.RegisterId) []Bytecode {
	//
	cmp := e.Cond.(*expr.Cmp[symbol.Resolved])
	// Lazily compile both arms — their instructions are placed inside the
	// conditionally-skipped regions below, so only the taken arm runs.
	trueInsns := p.compileExpr(e.IfTrue, bitwidth, mapping, target)
	falseInsns := p.compileExpr(e.IfFalse, bitwidth, mapping, target)
	// Evaluate condition operands (always runs).
	condRegs, condInsns := p.compileNonUniformArgs(mapping, cmp.Left, cmp.Right)
	// Selection sequence (counts are in bytecodes, since the arms are already
	// lowered):
	//   condInsns                                  always
	//   skip_if(cond, lhs, rhs, |falseInsns|+1)    if TRUE skip false arm + skip
	//   falseInsns                                 (skipped on TRUE)
	//   skip(|trueInsns|)                          jump past true arm
	//   trueInsns                                  (skipped on FALSE)
	insns := append([]Bytecode{}, condInsns...)
	insns = append(insns, vm.SkipIf[vm.Uint](vm.Cond(cmp.Operator),
		uint16(len(falseInsns)+1), condRegs[0], condRegs[1]))
	insns = append(insns, falseInsns...)
	insns = append(insns, vm.Skip[vm.Uint](uint16(len(trueInsns))))
	//
	return append(insns, trueInsns...)
}

func (p *StmtCompiler) compileTupleInitialiser(e *expr.TupleInitialiser[symbol.Resolved], mapping []uint,
	targets ...[]vm.RegisterId) (insns []Bytecode) {
	// NOTE: we assume the right number of targets for the initialiser here, and
	// that this was checked earlier in the pipeline.
	for i, target := range targets {
		var (
			ith      = e.Exprs[i]
			bitwidth = data.BitWidthOf(e.Type(), p.environment)
		)
		//
		insns = append(insns, p.compileExpr(ith, bitwidth, mapping, target)...)
	}
	//
	return insns
}

func (p *StmtCompiler) compileCast(e *expr.Cast[symbol.Resolved], bitwidth util.Option[uint], mapping []uint,
	targets []vm.RegisterId) []Bytecode {
	var (
		e_bitwidth = data.BitWidthOf(e.Expr.Type(), p.environment)
	)
	//
	switch {
	case bitwidth.IsEmpty() && e_bitwidth.IsEmpty():
		// 𝔽 → 𝔽 (no-op cast): compile the source directly into the native target.
		return p.compileExpr(e.Expr, bitwidth, mapping, targets)
	case bitwidth.IsEmpty():
		// uint→𝔽: assemble the source limbs and reduce modulo P.
		source, insns := p.compileUniformArgs(e_bitwidth, mapping, e.Expr)
		return append(insns, vm.UintToField[vm.Uint](targets[0], source))
	case e_bitwidth.IsEmpty():
		// 𝔽→uint: extract the canonical representative into the target limbs.
		source, insns := p.compileUniformArgs(util.None[uint](), mapping, e.Expr)
		return append(insns, vm.FieldToUint[vm.Uint](targets, source[0]))
	case e_bitwidth.Unwrap() <= bitwidth.Unwrap():
		// uint upcast
		return p.compileExpr(e.Expr, e_bitwidth, mapping, targets)
	default:
		// uint downcast (of some kind).
		return p.compileRootExpr(e.Expr, mapping, targets)
	}
}

func (p *StmtCompiler) compileIntConst(c vm.Uint, _ []uint, targets []vm.RegisterId,
) []Bytecode {
	//
	return []Bytecode{vm.AddVecConst(targets, nil, c)}
}

func (p *StmtCompiler) compileFieldConst(c vm.Uint, _ []uint, target RegisterId,
) []Bytecode {
	//
	return []Bytecode{vm.AddModP(target, nil, c)}
}

func (p *StmtCompiler) compileConcat(args []Expr, mapping []uint, targets []vm.RegisterId) []Bytecode {
	var nargs []Expr
	//
	nargs = append(nargs, args...)
	// Compile arguments
	sources, insns := p.compileNonUniformArgs(mapping, nargs...)
	// Reverse sources (as concatenation requires them in little endian order)
	sources = array.Reverse(sources)
	// Done
	return append(insns, vm.AssignV[vm.Uint](targets, sources...))
}

func (p *StmtCompiler) compileIntAdd(args []Expr, bitwidth uint, mapping []uint, targets []vm.RegisterId) []Bytecode {
	//
	var (
		constant vm.Uint
		nargs    []Expr
	)
	//
	for _, e := range args {
		var overflow bool
		//
		if c, ok := p.asConstant(e); ok {
			constant, overflow = constant.Add(c)
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
	sources, insns := p.compileUniformArgs(util.Some(bitwidth), mapping, nargs...)
	// Done
	return append(insns, vm.AddVecConst(targets, sources, constant))
}

func (p *StmtCompiler) compileFieldAdd(args []Expr, mapping []uint, target RegisterId) []Bytecode {
	//
	var (
		constant vm.Uint
		nargs    []Expr
		modulus  vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for _, e := range args {
		if c, ok := p.asConstant(e); ok {
			constant = constant.AddMod(c, modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(util.None[uint](), mapping, nargs...)
	// Done
	return append(insns, vm.AddModP(target, sources, constant))
}

func (p *StmtCompiler) compileFunctionCall(e *expr.ExternAccess[symbol.Resolved], mapping []uint,
	returns []RegisterId) []Bytecode {
	var (
		// Determine vm module identifier
		id = vm.ModuleId(mapping[e.Name.Index])
	)
	// Compile arguments
	arguments, insns := p.compileNonUniformArgs(mapping, e.Args...)

	return append(insns, vm.Call[vm.Uint](id, arguments, returns))
}

func (p *StmtCompiler) compileLocalAccess(e *expr.LocalAccess[symbol.Resolved], _ []uint, targets []vm.RegisterId,
) []Bytecode {
	return []Bytecode{vm.AssignV[vm.Uint](targets, util.Cast[RegisterId](e.Variable))}
}

func (p *StmtCompiler) compileFieldAccess(e *expr.LocalAccess[symbol.Resolved], _ []uint, target RegisterId,
) []Bytecode {
	var (
		zero vm.Uint
		reg  = []RegisterId{util.Cast[RegisterId](e.Variable)}
	)
	// The source of a bare field access is always native (uint→𝔽 conversions go
	// through a field-cast; see compileCast), so this is a 𝔽→𝔽 copy.
	return []Bytecode{vm.AddModP(target, reg, zero)}
}

func (p *StmtCompiler) compileArrayAccess(e *expr.ArrayAccess[symbol.Resolved], mapping []uint, targets []vm.RegisterId,
) []Bytecode {
	panic(fmt.Sprintf("unexpected ArrayAccess node reached codegen (variable %d)", e.Id))
}

func (p *StmtCompiler) compileMemoryRead(e *expr.ExternAccess[symbol.Resolved], mapping []uint,
	data []RegisterId) []Bytecode {
	// Determine vm module identifier
	id := util.Cast[uint16](mapping[e.Name.Index])
	// Compile arguments
	address, insns := p.compileNonUniformArgs(mapping, e.Args...)
	// Emit the read bytecode.  The memory kind is resolved from the environment
	// at encode time; any incoming cast checks on the data registers are inserted
	// later by the check-cast pass.
	return append(insns, vm.MemRead[vm.Uint](id, address, data))
}

func (p *StmtCompiler) compileIntMul(args []Expr, bitwidth uint, mapping []uint, targets []vm.RegisterId,
) []Bytecode {
	//
	var (
		constant vm.Uint = vm.Const64[vm.Uint](1)
		nargs    []Expr
	)
	//
	for _, e := range args {
		var carry vm.Uint
		//
		if c, ok := p.asConstant(e); ok {
			carry, constant = constant.Mul(c)
		} else {
			nargs = append(nargs, e)
		}
		// NOTE: this error should be caught and reported earlier in the
		// pipeline.
		if carry.Cmp64(0) != 0 || !constant.FitsWithin(bitwidth) {
			panic("arithmetic overflow")
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(util.Some(bitwidth), mapping, nargs...)
	//
	return append(insns, vm.MulVec(targets, sources, constant))
}

func (p *StmtCompiler) compileFieldMul(args []Expr, mapping []uint, target RegisterId,
) []Bytecode {
	//
	var (
		constant vm.Uint = vm.Const64[vm.Uint](1)
		nargs    []Expr
		modulus  vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for _, e := range args {
		if c, ok := p.asConstant(e); ok {
			constant = constant.MulMod(c, modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(util.None[uint](), mapping, nargs...)
	// Done
	return append(insns, vm.MulModP(target, sources, constant))
}

func (p *StmtCompiler) compileIntDiv(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
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
	sources, insns := p.compileUniformArgs(util.Some(bitwidth), mapping, nargs...)
	// Chain divisions left-to-right: (((a / b) / c) / ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(util.Some(bitwidth))
		insns = append(insns, vm.Div[vm.Uint](tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns,
		vm.Div[vm.Uint](target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileIntRem(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain remainders left-to-right: (((a % b) % c) % ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns, vm.Rem[vm.Uint](tmp, value, sources[i]))
		value = tmp
	}
	//
	return append(insns,
		vm.Rem[vm.Uint](target, value, sources[len(sources)-1]))
}

func (p *StmtCompiler) compileBitwiseShl(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain shifts left-to-right: (((a << b) << c) << ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns,
			vm.BitShl[vm.Uint](tmp, value, sources[i], util.Cast[uint16](bitwidth)))
		value = tmp
	}
	//
	return append(insns,
		vm.BitShl[vm.Uint](target, value, sources[len(sources)-1], uint16(bitwidth)))
}

func (p *StmtCompiler) compileBitwiseShr(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile all operands upfront.
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain shifts left-to-right: (((a >> b) >> c) >> ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns,
			vm.BitShr[vm.Uint](tmp, value, sources[i], util.Cast[uint16](bitwidth)))
		value = tmp
	}
	//
	return append(insns,
		vm.BitShr[vm.Uint](target, value, sources[len(sources)-1], uint16(bitwidth)))
}

func (p *StmtCompiler) compileIntSub(args []Expr, bitwidth uint, mapping []uint, targets []vm.RegisterId,
) []Bytecode {
	//
	var (
		bw       = util.Some(bitwidth)
		constant vm.Uint
		nargs    []Expr
	)
	//
	for i, e := range args {
		var overflow bool

		if c, ok := p.asConstant(e); ok && i > 0 {
			constant, overflow = constant.Add(c)
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
	sources, insns := p.compileUniformArgs(bw, mapping, nargs...)
	// Done (subtraction never needs a cast check; cf. compileSub).
	return append(insns, vm.SubVec(targets, sources, constant))
}

func (p *StmtCompiler) compileFieldSub(args []Expr, mapping []uint, target RegisterId) []Bytecode {
	//
	var (
		constant vm.Uint
		nargs    []Expr
		modulus  vm.Uint
	)
	//
	modulus = modulus.SetBigInt(p.field.Modulus())
	//
	for i, e := range args {
		if c, ok := p.asConstant(e); ok && i > 0 {
			constant = constant.AddMod(c, modulus)
		} else {
			nargs = append(nargs, e)
		}
	}
	// Compile arguments
	sources, insns := p.compileUniformArgs(util.None[uint](), mapping, nargs...)
	// Done
	return append(insns, vm.SubModP[vm.Uint](target, sources, constant))
}

func (p *StmtCompiler) compileBitwiseAnd(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile arguments
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain left-to-right: (((a & b) & c) & ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns, vm.BitAnd[vm.Uint](tmp, value, sources[i], util.Cast[uint16](bitwidth)))
		value = tmp
	}
	//
	return append(insns,
		vm.BitAnd[vm.Uint](target, value, sources[len(sources)-1], uint16(bitwidth)))
}

func (p *StmtCompiler) compileBitwiseNot(e *expr.BitwiseNot[symbol.Resolved], bitwidth uint, mapping []uint,
	target RegisterId) []Bytecode {
	//
	var bw = util.Some(bitwidth)
	//
	sources, insns := p.compileUniformArgs(bw, mapping, e.Expr)
	// NOT takes a single source; no cast (cf. compileNot).
	return append(insns, vm.BitNot[vm.Uint](target, sources[0], util.Cast[uint16](bitwidth)))
}

func (p *StmtCompiler) compileBitwiseOr(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile arguments
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain left-to-right: (((a | b) | c) | ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns, vm.BitOr[vm.Uint](tmp, value, sources[i], uint16(bitwidth)))
		value = tmp
	}
	//
	return append(insns,
		vm.BitOr[vm.Uint](target, value, sources[len(sources)-1], uint16(bitwidth)))
}

func (p *StmtCompiler) compileBitwiseXor(args []Expr, bitwidth uint, mapping []uint, target RegisterId,
) []Bytecode {
	var bw = util.Some(bitwidth)
	// Compile arguments
	sources, insns := p.compileUniformArgs(bw, mapping, args...)
	// Chain left-to-right: (((a ^ b) ^ c) ^ ...).
	value := sources[0]
	//
	for i := 1; i < len(sources)-1; i++ {
		tmp := p.allocate(bw)
		insns = append(insns, vm.BitXor[vm.Uint](tmp, value, sources[i], util.Cast[uint16](bitwidth)))
		value = tmp
	}
	//
	return append(insns,
		vm.BitXor[vm.Uint](target, value, sources[len(sources)-1], uint16(bitwidth)))
}

func (p *StmtCompiler) compileUniformArgs(bitwidth util.Option[uint], mapping []uint, exprs ...Expr,
) ([]RegisterId, []Bytecode) {
	//
	var (
		insns   []Bytecode
		targets = make([]RegisterId, len(exprs))
	)
	//
	for i, e := range exprs {
		target, extra := p.compileArg(e, bitwidth, mapping)
		targets[i] = target

		insns = append(insns, extra...)
	}
	//
	return targets, insns
}

func (p *StmtCompiler) compileNonUniformArgs(mapping []uint, exprs ...Expr) ([]RegisterId, []Bytecode) {
	var (
		insns   []Bytecode
		targets = make([]RegisterId, len(exprs))
	)
	//
	for i, e := range exprs {
		target, extra := p.compileArg(e, data.BitWidthOf(e.Type(), p.environment), mapping)
		targets[i] = target

		insns = append(insns, extra...)
	}
	//
	return targets, insns
}

func (p *StmtCompiler) compileArg(e Expr, bitwidth util.Option[uint], mapping []uint) (RegisterId, []Bytecode) {
	if r, ok := p.asLocalAccess(e); ok {
		return util.Cast[RegisterId](r.Variable), nil
	}
	//
	target := p.allocate(bitwidth)

	return target, p.compileExpr(e, bitwidth, mapping, []vm.RegisterId{target})
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

func (p *StmtCompiler) allocate(bitwidth util.Option[uint]) RegisterId {
	var (
		n       = len(p.registers)
		name    = fmt.Sprintf("$%d", n)
		padding vm.Uint
	)
	//
	p.registers = append(p.registers, vm.NewComputedRegister[vm.Uint](name, bitwidth, padding))
	//
	return util.Cast[RegisterId](uint(n))
}

// bitwidthOf returns the bit-width to use when folding compile-time
// constants into a target register.  For integer-typed targets this is the
// register's declared width; for field-typed (native) targets this is the
// configured field bandwidth, since field elements have no fixed bit-width
// and only need enough room to hold a representative.
func (p *StmtCompiler) bitwidthOf(targets ...vm.RegisterId) util.Option[uint] {
	var bitwidth uint
	//
	for _, r := range targets {
		ith := p.registers[r]
		//
		if ith.IsNative() && len(targets) == 1 {
			return util.None[uint]()
		} else if ith.IsNative() {
			panic("cannot destructure field elements")
		}
		//
		bitwidth += ith.Bitwidth().Unwrap()
	}
	//
	return util.Some(bitwidth)
}

func (p *StmtCompiler) asConstant(e Expr) (vm.Uint, bool) {
	var w vm.Uint
	//
	if c, ok := e.(*expr.Const[symbol.Resolved]); ok {
		return w.SetBigInt(c.Constant()), true
	} else if p.isConstantAccess(e) {
		return p.evalConstant(e), true
	}
	//
	return w, false
}

// asLocalAccess unwraps e to a bare local-variable access, peeling casts that
// do not cross the 𝔽/uint boundary.  A representation-changing cast is not
// transparent — it must materialise a field-cast instruction — so it blocks
// the peel.
func (p *StmtCompiler) asLocalAccess(e Expr) (*expr.LocalAccess[symbol.Resolved], bool) {
	if c, ok := e.(*expr.LocalAccess[symbol.Resolved]); ok {
		return c, true
	} else if c, ok := e.(*expr.Cast[symbol.Resolved]); ok {
		var (
			outer = data.BitWidthOf(c.Type(), p.environment)
			inner = data.BitWidthOf(c.Expr.Type(), p.environment)
		)
		//
		if outer.IsEmpty() == inner.IsEmpty() {
			return p.asLocalAccess(c.Expr)
		}
	}
	//
	return nil, false
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
