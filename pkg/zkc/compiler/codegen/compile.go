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

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/lval"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/stmt"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
	"github.com/consensys/go-corset/pkg/zkc/vm"

	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen/lowerzkcnative"
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

// Function is a convenient alias
type Function = vm.WordFunction

// Stmt is a convenient alias
type Stmt = stmt.Stmt[symbol.Resolved]

// Condition is a convenient alias
type Condition = expr.Condition[symbol.Resolved]

// Expr is a convenient alias
type Expr = expr.Expr[symbol.Resolved]

// LVal is a convenient alias
type LVal = lval.LVal[symbol.Resolved]

// Instruction provides a convenient alias
type Instruction = vm.WordInstruction

// VectorInstruction provides a convenient alias
type VectorInstruction = vm.Vector[Instruction]

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
func (p *Compiler) Compile(declarations []Declaration) (*vm.WordMachine[vm.Uint], []source.SyntaxError) {
	//
	var (
		modules []vm.Module
		mapping = make([]uint, len(declarations))
		index   = uint(0)
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
		case *decl.ResolvedInclude:
			// ignore
		case *decl.ResolvedMemory:
			var regs = toMemoryRegisters(c.Address, c.Data, p.env)
			//
			switch c.Kind {
			case decl.PRIVATE_READ_ONLY_MEMORY, decl.PUBLIC_READ_ONLY_MEMORY:
				public := c.Kind == decl.PUBLIC_READ_ONLY_MEMORY
				modules = append(modules, vm.NewInputMemory[vm.Uint](c.Name(), public, regs))
			case decl.PRIVATE_WRITE_ONCE_MEMORY, decl.PUBLIC_WRITE_ONCE_MEMORY:
				public := c.Kind == decl.PUBLIC_WRITE_ONCE_MEMORY
				modules = append(modules, vm.NewOutputMemory[vm.Uint](c.Name(), public, regs))
			case decl.PRIVATE_STATIC_MEMORY, decl.PUBLIC_STATIC_MEMORY:
				public := c.Kind == decl.PUBLIC_STATIC_MEMORY
				// Compile the static initialiser
				words, errs := p.compileStaticInitialisers(declarations, p.env, p.srcmaps, c.Contents...)
				//
				if len(errs) == 0 {
					// Construct the read-only memory
					modules = append(modules, vm.NewStaticMemory(c.Name(), public, regs, words...))
				}
				// Include all errors
				errors = append(errors, errs...)
			case decl.RANDOM_ACCESS_MEMORY:
				if slices.Contains(c.Annotations(), "bipartite") {
					modules = append(modules, vm.NewLargeReadWriteMemory[vm.Uint](c.Name(), regs))
				} else {
					modules = append(modules, vm.NewReadWriteMemory[vm.Uint](c.Name(), regs))
				}
			}
		default:
			panic(fmt.Sprintf("unknown declaration %s", c.Name()))
		}
	}
	// Lower VM-level zkc-native instructions into arithmetic instructions.
	if len(errors) == 0 && p.config.lowerZkcNative {
		// Reduce chain bitwise operation in order to prepare the VM instructions for bitwise lowering.
		modules = lowerzkcnative.BinarizeBitwise[vm.Uint](modules)
		// Lower Bitwise operations into arithmetic instructions.
		modules = lowerzkcnative.LowerBitwise[vm.Uint](modules)
		// Lower INT_DIV/INT_REM into hint + arithmetic validation sequences.
		modules = lowerzkcnative.LowerDivisions[vm.Uint](modules)
		// Lower relational SkipIf (LT/GT/LTEQ/GTEQ) into sign-bit extraction sequences.
		// Must run after LowerBitwise and LowerDivisions, which may generate new relational SkipIf instructions.
		modules = lowerzkcnative.LowerComparisons[vm.Uint](modules)
	}
	// Vectorize modules (if no errors)
	if len(errors) == 0 && p.config.vectorize {
		Vectorize(modules, p.srcmaps)
	}
	//
	wm := vm.NewWordMachine[vm.Uint](p.config.field, modules...)
	// Apply register splitting (for now)
	if len(errors) == 0 && p.config.splitting {
		wm = vm.Subdivide(p.config.field, wm)
	}
	// Construct machine
	return wm, errors
}

// compileStaticInitialise evaluates the compile-time constant expressions from a static
// memory declaration into the vm.Uint representation required by the VM.
func (p *Compiler) compileStaticInitialisers(
	components []Declaration, env data.ResolvedEnvironment,
	srcmaps source.Maps[any], contents ...expr.Resolved,
) ([]vm.Uint, []source.SyntaxError) {
	//
	var (
		words  = make([]vm.Uint, len(contents))
		errors []source.SyntaxError
	)
	//
	for i, v := range contents {
		var errMsg string

		words[i], errMsg = EvalConstant(v, true, components, env)
		if errMsg != "" {
			errors = append(errors, srcmaps.SyntaxErrors(v, errMsg)...)
		}
	}

	return words, errors
}

// Convert a decl.Function instance into a fun.Function instance by flattening
// the variable descriptors into register descriptors.  Each variable may
// expand into one or more registers (e.g. a tuple variable produces one
// register per element).
func (p *Compiler) compileFunction(id uint, mapping []uint, program []Declaration,
) (*Function, []source.SyntaxError) {
	//
	var (
		fn        = program[id].(*decl.ResolvedFunction)
		registers []register.Register
		padding   big.Int // zero padding
		bootCode  = make([]VectorInstruction, len(fn.Code))
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

		flattern(v.DataType, v.Name, p.env, func(name string, bitwidth uint) {
			registers = append(registers, register.New(kind, name, bitwidth, padding))
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
	}
	//
	for i, stmt := range fn.Code {
		bootCode[i] = compiler.compileStatement(uint(i), mapping, stmt)
	}
	//
	native := slices.Contains(fn.Annotations(), "native")
	//
	return vm.NewFunction(fn.Name(), native, compiler.registers, bootCode), compiler.errors
}

func toMemoryRegisters(address []VariableDescriptor, datas []VariableDescriptor, env data.ResolvedEnvironment,
) []register.Register {
	var (
		registers []register.Register
		padding   big.Int
	)
	// Flattern address lines
	for _, v := range address {
		flattern(v.DataType, v.Name, env, func(name string, bitwidth uint) {
			registers = append(registers, register.NewInput(name, bitwidth, padding))
		})
	}
	// Flattern data lines
	for _, v := range datas {
		flattern(v.DataType, v.Name, env, func(name string, bitwidth uint) {
			registers = append(registers, register.NewOutput(name, bitwidth, padding))
		})
	}
	//
	return registers
}
