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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// Declaration represents a declaration which can contain macro
// instructions and where external identifiers are otherwise resolved. As such,
// it should not be possible that such a declaration refers to unknown (or
// otherwise incorrect) external components.
type Declaration = decl.Declaration[symbol.Resolved]

// VariableDescriptor represents a descriptor whose external identifiers are
// otherwise resolved. As such, it should not be possible that such a
// declaration refers to unknown (or otherwise incorrect) external components.
type VariableDescriptor = variable.Descriptor[symbol.Resolved]

// Stmt is a convenient alias
type Stmt = stmt.Stmt[symbol.Resolved]

// Condition is a convenient alias
type Condition = expr.Condition[symbol.Resolved]

// Expr is a convenient alias
type Expr = expr.Expr[symbol.Resolved]

// LVal is a convenient alias
type LVal = lval.LVal[symbol.Resolved]

// Bytecode provides a convenient alias for a single bytecode instruction
// emitted by codegen.
type Bytecode = vm.Bytecode[vm.Uint]

// BytecodeVector provides a convenient alias for a vector (trace line) of
// bytecode instructions.
type BytecodeVector = vm.BytecodeVector[vm.Uint]

// Compiler is responsible for compiling high-level programs into low-level
// machines which can be used (for example) to execute this program with some
// given inputs.  A compile is configurable in certain aspects.
type Compiler struct {
	env     data.ResolvedEnvironment
	srcmaps source.Maps[any]
	// configuration
	config Config
}

// NewCompiler constructs a code generator parameterised by a configuration,
// the resolved type environment, and the source maps recorded by earlier
// pipeline stages.  The configuration controls optional passes such as
// vectorisation; cfg=DEFAULT_CONFIG matches the prover-facing defaults.  The
// environment supplies type information needed when lowering expressions
// (e.g. bit-widths of named types), and the source maps allow generated
// instructions and any errors raised during compilation to be tied back to
// their originating source positions.
func NewCompiler(cfg Config, env data.ResolvedEnvironment, srcmaps source.Maps[any]) *Compiler {
	return &Compiler{
		env:     env,
		srcmaps: srcmaps,
		config:  cfg,
	}
}

// Compile attempts to compile a given high-level program into a low-level
// machine which can be used (for example) to execute this program with some
// given inputs.
func (p *Compiler) Compile(declarations []Declaration) (vm.Program[vm.Uint], []source.SyntaxError) {
	//
	var (
		modules []vm.BytecodeModule[vm.Uint]
		mapping = make([]uint, len(declarations))
		index   = uint(0)
		inlines []string
		errors  []source.SyntaxError
	)
	// Construct the mapping from ast declaration identifiers to vm module
	// identifiers.  Essentially, what is happening here is that some ast
	// declarations will no longer exist at the machine level.  So, when a
	// declaration is encountered that will no longer exist, then the id for all
	// declarations after it is shifted down.
	for i, d := range declarations {
		switch d.(type) {
		case *decl.ResolvedFunction, *decl.ResolvedMemory:
			mapping[i] = index
			index++
		default:
			mapping[i] = math.MaxUint
		}
	}
	// Initialise components
	for i, c := range declarations {
		switch c := c.(type) {
		case *decl.ResolvedConstant:
			// force detection of errors
			_, errs := p.compileStaticInitialisers(declarations, p.env, p.srcmaps, c.ConstExpr)
			//
			errors = append(errors, errs...)
		case *decl.ResolvedTypeAlias:
			// ignore
		case *decl.ResolvedFunction:
			fn, errs := p.compileFunction(uint(i), mapping, declarations)
			modules = append(modules, fn)
			errors = append(errors, errs...)
			// Record functions to be inlined (see below).
			if slices.Contains(c.Annotations(), "inline") {
				inlines = append(inlines, c.Name())
			}
		case *decl.ResolvedInclude:
			// ignore
		case *decl.ResolvedMemory:
			mem, errs := p.buildMemory(declarations, c)
			//
			if len(errs) == 0 {
				modules = append(modules, mem)
			}
			// Include any static-initialiser errors
			errors = append(errors, errs...)
		default:
			panic(fmt.Sprintf("unknown declaration %s", c.Name()))
		}
	}

	// Stop here if any errors were detected during compilation of the declarations.
	// There shouldn't be any compilation errors after this point.
	if len(errors) > 0 {
		return vm.Program[vm.Uint]{}, errors
	}
	// Construct bytecode program from descriptor modules.
	program := vm.NewBytecodeProgram(p.config.field, modules...)
	//
	if p.config.inlining && len(inlines) > 0 {
		// Apply function inlining
		program = vm.InlineFunctions(program, inlines)
	}
	//
	if p.config.fastMode {
		// Apply transforms suitable for fast mode
		program = vm.OptimizeDivisions(program)
		program = vm.Vectorize(program)
		// NOTE: eventually this will always be applied
		if p.config.splitting {
			// FIXME: this is broken as we should be splitting for the target
			// word, not the target field.
			program = vm.SplitRegisters(p.config.field, program)
		}
	} else {
		// Apply transformations required for tracing and constraint generation.
		program = vm.LowerBitwise(program)
		program = vm.LowerDivisions(program)
		program = vm.LowerComparisons(program)
		program = vm.LowerSwitch(program)
		program = vm.Vectorize(program)
		program = vm.FactorSkipConditions(program)
		program = vm.FlattenCalls(program)
		// NOTE: eventually this will always be applied
		if p.config.splitting {
			program = vm.SplitRegisters(p.config.field, program)
		}
		//
		program = vm.AddRangeConstraints(p.config.field, program, p.config.maxStaticDepth)
	}
	// Insert check casts to ensure appropriate safety checks during execution.
	program = vm.InsertCheckCasts(program)
	// Done
	return program, errors
}

// compileStaticInitialise evaluates the compile-time constant expressions from a static
// memory declaration into the vm.Uint representation required by the VM.
func (p *Compiler) compileStaticInitialisers(
	components []Declaration, env data.ResolvedEnvironment,
	srcmaps source.Maps[any], contents ...expr.Resolved,
) ([]vm.Uint, []source.SyntaxError) {
	//
	var (
		words     = make([]vm.Uint, len(contents))
		errors    []source.SyntaxError
		evaluator = NewConstantEvaluator(p.config.field, env, components...)
	)
	//
	for i, v := range contents {
		var errMsg string

		words[i], errMsg = evaluator.Eval(v, true)
		if errMsg != "" {
			errors = append(errors, srcmaps.SyntaxErrors(v, errMsg)...)
		}
	}

	return words, errors
}

// Convert a decl.Function instance into a bytecode (descriptor) function by
// flattening the variable descriptors into register descriptors and compiling
// its statements directly into bytecode vectors.  Each variable may expand into
// one or more registers (e.g. a tuple variable produces one register per
// element).  Calls and memory accesses are resolved against the resolved AST
// (the program), so no separate signature table is required.
func (p *Compiler) compileFunction(id uint, mapping []uint, program []Declaration,
) (*vm.Function[vm.Uint], []source.SyntaxError) {
	//
	var (
		fn        = program[id].(*decl.ResolvedFunction)
		registers []vm.Register[vm.Uint]
		padding   vm.Uint // zero padding
		vectors   = make([]BytecodeVector, len(fn.Code))
	)
	//
	for _, v := range fn.Variables {
		var kind register.Type

		switch v.Kind {
		case variable.PARAMETER:
			kind = register.INPUT_REGISTER
		case variable.RETURN:
			kind = register.OUTPUT_REGISTER
		case variable.LOCAL:
			kind = register.COMPUTED_REGISTER
		default:
			panic(fmt.Sprintf("unexpected variable kind %d", v.Kind))
		}

		flatten(v.DataType, v.Name, p.env, func(name string, bitwidth uint) {
			registers = append(registers, vm.NewRegister(kind, name, bitwidthOf(bitwidth), padding))
		})
	}
	//
	compiler := StmtCompiler{
		components:  program,
		variables:   fn.Variables,
		registers:   registers,
		environment: p.env,
		field:       p.config.field,
		srcmaps:     p.srcmaps,
		quiet:       p.config.quiet,
		fastMode:    p.config.fastMode,
	}
	//
	for i, stmt := range fn.Code {
		vectors[i] = compiler.compileStatement(uint(i), mapping, stmt)
	}
	//
	native := slices.Contains(fn.Annotations(), "native")
	// Note: compiler.registers includes any temporaries allocated during
	// statement compilation.
	return vm.NewBytecodeFunction(fn.Name(), native, compiler.registers, vectors...), compiler.errors
}

// buildMemory constructs the memory descriptor module for a resolved memory
// declaration.  For static memories, the (compile-time constant) contents are
// evaluated here; any evaluation errors are returned alongside the descriptor,
// which is still constructed (with whatever contents were produced) -- compilation
// aborts on the returned errors before the memory is ever executed.
func (p *Compiler) buildMemory(program []Declaration, c *decl.ResolvedMemory,
) (vm.BytecodeModule[vm.Uint], []source.SyntaxError) {
	var regs = toMemoryRegisters[vm.Uint](c.Address, c.Data, p.env)
	//
	switch c.Kind {
	case decl.PRIVATE_READ_ONLY_MEMORY:
		return vm.NewBytecodeMemory(c.Name(), vm.PRIVATE_READ_ONLY_MEMORY, regs), nil
	case decl.PUBLIC_READ_ONLY_MEMORY:
		return vm.NewBytecodeMemory(c.Name(), vm.PUBLIC_READ_ONLY_MEMORY, regs), nil
	case decl.PRIVATE_WRITE_ONCE_MEMORY:
		return vm.NewBytecodeMemory(c.Name(), vm.PRIVATE_WRITE_ONCE_MEMORY, regs), nil
	case decl.PUBLIC_WRITE_ONCE_MEMORY:
		return vm.NewBytecodeMemory(c.Name(), vm.PUBLIC_WRITE_ONCE_MEMORY, regs), nil
	case decl.PRIVATE_STATIC_MEMORY:
		// Compile the static initialiser
		words, errs := p.compileStaticInitialisers(program, p.env, p.srcmaps, c.Contents...)
		//
		return vm.NewBytecodeMemory(c.Name(), vm.PRIVATE_STATIC_MEMORY, regs, words...), errs
	case decl.PUBLIC_STATIC_MEMORY:
		// Compile the static initialiser
		words, errs := p.compileStaticInitialisers(program, p.env, p.srcmaps, c.Contents...)
		//
		return vm.NewBytecodeMemory(c.Name(), vm.PUBLIC_STATIC_MEMORY, regs, words...), errs
	case decl.RANDOM_ACCESS_MEMORY:
		// Check for paged memory
		if slices.Contains(c.Annotations(), "paged") {
			return vm.NewBytecodeMemory(c.Name(), vm.PAGED_READWRITE_MEMORY, regs), nil
		}
		//
		return vm.NewBytecodeMemory(c.Name(), vm.READWRITE_MEMORY, regs), nil
	default:
		panic(fmt.Sprintf("unknown memory kind for \"%s\"", c.Name()))
	}
}

func toMemoryRegisters[W vm.Word[W]](address []VariableDescriptor, datas []VariableDescriptor,
	env data.ResolvedEnvironment) []vm.Register[W] {
	var (
		registers []vm.Register[W]
		padding   W
	)
	// Flatten address lines
	for _, v := range address {
		flatten(v.DataType, v.Name, env, func(name string, bitwidth uint) {
			registers = append(registers, vm.NewInputRegister(name, bitwidthOf(bitwidth), padding))
		})
	}
	// Flatten data lines
	for _, v := range datas {
		flatten(v.DataType, v.Name, env, func(name string, bitwidth uint) {
			registers = append(registers, vm.NewOutputRegister(name, bitwidthOf(bitwidth), padding))
		})
	}
	//
	return registers
}

func bitwidthOf(bitwidth uint) util.Option[uint] {
	if bitwidth == math.MaxUint {
		return util.None[uint]()
	}
	//
	return util.Some(bitwidth)
}
