// Copyright Consensys Software Inc.
//
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
package corset

import (
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/corset/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// STDLIB is an import of the standard library.
//
//go:embed stdlib.lisp
var STDLIB []byte

// SyntaxError defines the kind of errors that can be reported by this compiler.
// Syntax errors are always associated with some line in one of the original
// source files.  For simplicity, we reuse existing notion of syntax error from
// the S-Expression library.
type SyntaxError = source.SyntaxError

// CompilationConfig encapsulates various options which can affect compilation.
type CompilationConfig = compiler.Config

// CompileSourceFiles compiles one or more source files into a schema.  This
// process can fail if the source files are mal-formed, or contain syntax errors
// or other forms of error (e.g. type errors).
func CompileSourceFiles(config CompilationConfig, srcfiles []source.File, extern asm.MacroProgram,
) (asm.MacroHirProgram, SourceMap, []SyntaxError) {
	// Include the standard library (if requested)
	srcfiles = includeStdlib(config.Stdlib, srcfiles)
	// Parse all source files (inc stdblib if applicable).
	circuit, srcmap, errs := compiler.ParseSourceFiles(srcfiles, config)
	// Check for parsing errors
	if errs != nil {
		return asm.MacroHirProgram{}, SourceMap{}, errs
	}
	// Compile each module into the schema
	comp := NewCompiler(circuit, srcmap, extern).
		SetDebug(config.Debug)
	// Configure register allocator (if requested)
	if config.Legacy {
		comp.SetAllocator(compiler.LegacyAllocator)
	} else {
		comp.SetAllocator(compiler.ImprovedAllocator)
	}
	//
	return comp.Compile(config)
}

// CompileSourceFile compiles exactly one source file into a schema.  This is
// really helper function for e.g. the testing environment.   This process can
// fail if the source file is mal-formed, or contains syntax errors or other
// forms of error (e.g. type errors).
func CompileSourceFile(config CompilationConfig, srcfile source.File) (asm.MacroHirProgram, SourceMap, []SyntaxError) {
	//
	var macroProgram asm.MacroProgram
	//
	return CompileSourceFiles(config, []source.File{srcfile}, macroProgram)
}

// Compiler packages up everything needed to compile a given set of module
// definitions down into an HIR schema.  Observe that the compiler may fail if
// the modules definitions are malformed in some way (e.g. fail type checking).
type Compiler struct {
	// The register allocation algorithm to be used by this compiler.
	allocator func(compiler.RegisterAllocation)
	// A high-level definition of a Corset circuit.
	circuit ast.Circuit
	// Externally defined modules
	asmProgram asm.MacroProgram
	// Determines whether debug
	debug bool
	// Determines whether to apply sanity checks
	checks bool
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
}

// NewCompiler constructs a new compiler for a given set of modules.
func NewCompiler(circuit ast.Circuit, srcmaps *source.Maps[ast.Node], extern asm.MacroProgram) *Compiler {
	//
	return &Compiler{compiler.DEFAULT_ALLOCATOR, circuit, extern, false, true, srcmaps}
}

// SetDebug enables or disables debug mode.  In debug mode, debug constraints
// will be compiled in.
func (p *Compiler) SetDebug(flag bool) *Compiler {
	p.debug = flag
	return p
}

// SetAllocator overrides the default register allocator.
func (p *Compiler) SetAllocator(allocator func(compiler.RegisterAllocation)) *Compiler {
	p.allocator = allocator
	return p
}

// Compile is the top-level function for the corset compiler which actually
// compiles the given modules down into a schema.  This can fail in a variety of
// ways if the given modules are malformed in some way.  For example, if some
// expression refers to a non-existent module or column, or is not well-typed,
// etc.
func (p *Compiler) Compile(config compiler.Config) (asm.MacroHirProgram, SourceMap, []SyntaxError) {
	var (
		scope  *compiler.ModuleScope
		errors []SyntaxError
	)
	// Resolve variables (via nested scopes)
	scope, errors = compiler.ResolveCircuit(p.srcmap, &p.circuit, p.asmProgram.Components()...)
	// Type check circuit.
	errors = append(errors, compiler.TypeCheckCircuit(p.srcmap, &p.circuit)...)
	// Catch errors
	if len(errors) > 0 {
		return asm.MacroHirProgram{}, SourceMap{}, errors
	}
	// Preprocess circuit to remove invocations, reductions, etc.
	if errors = compiler.PreprocessCircuit(p.debug, p.srcmap, &p.circuit); len(errors) > 0 {
		return asm.MacroHirProgram{}, SourceMap{}, errors
	}
	// Convert global scope into an environment by allocating all columns.
	environment := compiler.NewGlobalEnvironment(scope, p.allocator)
	// Translate everything and add it to the schema.
	asmProgram, errs := compiler.TranslateCircuit(environment, p.srcmap, &p.circuit, p.asmProgram, config)
	// Sanity check for errors
	if len(errs) > 0 {
		return asm.MacroHirProgram{}, SourceMap{}, errs
	} else if cerrs := asmProgram.Consistent(math.MaxUint); len(cerrs) > 0 {
		// Should be unreachable.
		for _, err := range cerrs {
			fmt.Println(err.Error())
		}
		//
		panic("inconsistent schema?")
	}
	// Construct source map
	source_map := constructSourceMap(&asmProgram, scope, environment)
	// Construct binary file
	return asmProgram, *source_map, errs
}

func includeStdlib(stdlib bool, srcfiles []source.File) []source.File {
	if stdlib {
		// Include stdlib file
		srcfile := source.NewSourceFile("stdlib.lisp", STDLIB)
		// Append to srcfile list
		srcfiles = append(srcfiles, *srcfile)
	}
	// Not included
	return srcfiles
}

func constructSourceMap(schema schema.AnySchema[word.BigEndian], scope *compiler.ModuleScope,
	env compiler.GlobalEnvironment) *SourceMap {
	//
	enumerations := []Enumeration{OPCODE_ENUMERATION}
	return &SourceMap{constructSourceModule(schema, scope, env), enumerations}
}

func constructSourceModule(schema schema.AnySchema[word.BigEndian], scope *compiler.ModuleScope,
	env compiler.GlobalEnvironment) SourceModule {
	//
	var (
		columns    []SourceColumn
		submodules []SourceModule
		constants  []SourceConstant
	)
	// Map source-level columns
	for _, col := range scope.DestructuredColumns() {
		// Determine register allocated to this (destructured) column.
		ref := determineRegisterRef(col.Name, schema, env)
		// Determine (unqualified) column name
		name := col.Name.Tail()
		//
		display := constructDisplayModifier(col.Display)
		// Translate register source into source column
		srcCol := SourceColumn{name,
			col.Multiplier,
			col.Bitwidth,
			col.MustProve,
			col.Computed,
			display,
			ref}
		columns = append(columns, srcCol)
	}
	// Map source-level constants
	for _, binding := range scope.DestructuredConstants() {
		var bitwidth uint = math.MaxUint
		// Convert data type
		if dt, ok := binding.DataType.(*ast.IntType); ok {
			bitwidth = dt.BitWidth()
		}
		//
		constants = append(constants, SourceConstant{
			binding.Path.Tail(),
			*binding.Value.AsConstant(),
			bitwidth,
			binding.Extern,
		})
	}
	//
	for _, child := range scope.Children() {
		submodules = append(submodules, constructSourceModule(schema, child, env))
	}
	//
	return SourceModule{
		Name:       scope.Name(),
		Public:     scope.IsPublic(),
		Synthetic:  false,
		Virtual:    scope.Virtual(),
		Selector:   scope.Selector(),
		Submodules: submodules,
		Columns:    columns,
		Constants:  constants,
	}
}

// Determine the reference reference in the schema which corresponds with a
// given (Corset) path.
func determineRegisterRef[F any](path file.Path, sc schema.AnySchema[F], env compiler.GlobalEnvironment,
) register.Ref {
	var (
		mid schema.ModuleId
		rid register.Id
		ok  bool
	)
	// First, determine the corresponding Corset register associated with the
	// given path.
	reg := env.Register(env.RegisterOf(&path))
	// Now, lookup the corresponding schema module.
	if mid, ok = sc.HasModule(reg.Context.ModuleName()); !ok {
		panic(fmt.Sprintf("unknown module \"%s\"", reg.Context.ModuleName()))
	}
	// Now, lookup the corresponding register.
	if rid, ok = sc.Module(mid).HasRegister(reg.Name()); !ok {
		// Should be unreachable
		panic(fmt.Sprintf("unknown register \"%s\"", reg.Name()))
	}
	//
	return register.NewRef(mid, rid)
}

func constructDisplayModifier(modifier string) uint {
	switch modifier {
	case "hex":
		return DISPLAY_HEX
	case "dec":
		return DISPLAY_DEC
	case "bytes":
		return DISPLAY_BYTES
	case "opcode":
		return DISPLAY_CUSTOM
	}
	// unknown, so default to hex
	return DISPLAY_HEX
}

// OPCODE_ENUMERATION provides a default enumeration for the existing ":opcode"
// display modifier.
var OPCODE_ENUMERATION map[uint64]string = map[uint64]string{}

// ContextOf attempts to reconstruct an AST context from a given module name.
// This is helpful if we want to know the "root" module for a given family of
// related modules (i.e. modules from the same Corset module which have
// different length multipliers).
func ContextOf(name string) ast.Context {
	var (
		split      = strings.Split(name, "×")
		multiplier = 1
		err        error
	)
	//
	if len(split) == 2 {
		multiplier, err = strconv.Atoi(split[1])
		//
		if err != nil {
			panic(fmt.Sprintf("invalid module name %s", name))
		}
	} else if len(split) != 1 {
		panic(fmt.Sprintf("invalid module name %s", name))
	}
	//
	return ast.NewContext(split[0], uint(multiplier))
}

func init() {
	OPCODE_ENUMERATION[0x0] = "STOP"
	OPCODE_ENUMERATION[0x1] = "ADD"
	OPCODE_ENUMERATION[0x2] = "MUL"
	OPCODE_ENUMERATION[0x3] = "SUB"
	OPCODE_ENUMERATION[0x4] = "DIV"
	OPCODE_ENUMERATION[0x5] = "SDIV"
	OPCODE_ENUMERATION[0x6] = "MOD"
	OPCODE_ENUMERATION[0x7] = "SMOD"
	OPCODE_ENUMERATION[0x8] = "ADDMOD"
	OPCODE_ENUMERATION[0x9] = "MULMOD"
	OPCODE_ENUMERATION[0xa] = "EXP"
	OPCODE_ENUMERATION[0xb] = "SIGNEXTEND"
	OPCODE_ENUMERATION[0x10] = "LT"
	OPCODE_ENUMERATION[0x11] = "GT"
	OPCODE_ENUMERATION[0x12] = "SLT"
	OPCODE_ENUMERATION[0x13] = "SGT"
	OPCODE_ENUMERATION[0x14] = "EQ"
	OPCODE_ENUMERATION[0x15] = "ISZERO"
	OPCODE_ENUMERATION[0x16] = "AND"
	OPCODE_ENUMERATION[0x17] = "OR"
	OPCODE_ENUMERATION[0x18] = "XOR"
	OPCODE_ENUMERATION[0x19] = "NOT"
	OPCODE_ENUMERATION[0x1a] = "BYTE"
	OPCODE_ENUMERATION[0x1b] = "SHL"
	OPCODE_ENUMERATION[0x1c] = "SHR"
	OPCODE_ENUMERATION[0x1d] = "SAR"
	OPCODE_ENUMERATION[0x20] = "SHA3"
	OPCODE_ENUMERATION[0x30] = "ADDRESS"
	OPCODE_ENUMERATION[0x31] = "BALANCE"
	OPCODE_ENUMERATION[0x32] = "ORIGIN"
	OPCODE_ENUMERATION[0x33] = "CALLER"
	OPCODE_ENUMERATION[0x34] = "CALLVALUE"
	OPCODE_ENUMERATION[0x35] = "CALLDATALOAD"
	OPCODE_ENUMERATION[0x36] = "CALLDATASIZE"
	OPCODE_ENUMERATION[0x37] = "CALLDATACOPY"
	OPCODE_ENUMERATION[0x38] = "CODESIZE"
	OPCODE_ENUMERATION[0x39] = "CODECOPY"
	OPCODE_ENUMERATION[0x3a] = "GASPRICE"
	OPCODE_ENUMERATION[0x3b] = "EXTCODESIZE"
	OPCODE_ENUMERATION[0x3c] = "EXTCODECOPY"
	OPCODE_ENUMERATION[0x3d] = "RETURNDATASIZE"
	OPCODE_ENUMERATION[0x3e] = "RETURNDATACOPY"
	OPCODE_ENUMERATION[0x3f] = "EXTCODEHASH"
	OPCODE_ENUMERATION[0x40] = "BLOCKHASH"
	OPCODE_ENUMERATION[0x41] = "COINBASE"
	OPCODE_ENUMERATION[0x42] = "TIMESTAMP"
	OPCODE_ENUMERATION[0x43] = "NUMBER"
	OPCODE_ENUMERATION[0x44] = "DIFFICULTY"
	OPCODE_ENUMERATION[0x45] = "GASLIMIT"
	OPCODE_ENUMERATION[0x46] = "CHAINID"
	OPCODE_ENUMERATION[0x47] = "SELFBALANCE"
	OPCODE_ENUMERATION[0x48] = "BASEFEE"
	OPCODE_ENUMERATION[0x50] = "POP"
	OPCODE_ENUMERATION[0x51] = "MLOAD"
	OPCODE_ENUMERATION[0x52] = "MSTORE"
	OPCODE_ENUMERATION[0x53] = "MSTORE8"
	OPCODE_ENUMERATION[0x54] = "SLOAD"
	OPCODE_ENUMERATION[0x55] = "SSTORE"
	OPCODE_ENUMERATION[0x56] = "JUMP"
	OPCODE_ENUMERATION[0x57] = "JUMPI"
	OPCODE_ENUMERATION[0x58] = "PC"
	OPCODE_ENUMERATION[0x59] = "MSIZE"
	OPCODE_ENUMERATION[0x5a] = "GAS"
	OPCODE_ENUMERATION[0x5b] = "JUMPDEST"
	// EIP-1153
	OPCODE_ENUMERATION[0x5c] = "TLOAD"
	OPCODE_ENUMERATION[0x5d] = "TSTORE"
	// EIP-5656
	OPCODE_ENUMERATION[0x5e] = "MCOPY"
	// Push (inc PUSH0)
	for i := uint64(0); i <= 32; i++ {
		OPCODE_ENUMERATION[i+0x5f] = fmt.Sprintf("PUSH%d", i)
	}
	// Dup
	for i := uint64(0); i < 16; i++ {
		OPCODE_ENUMERATION[i+0x80] = fmt.Sprintf("DUP%d", i+1)
	}
	// Swap
	for i := uint64(0); i < 16; i++ {
		OPCODE_ENUMERATION[i+0x90] = fmt.Sprintf("SWAP%d", i+1)
	}
	// Log
	for i := uint64(0); i <= 4; i++ {
		OPCODE_ENUMERATION[i+0xa0] = fmt.Sprintf("LOG%d", i)
	}
	//
	OPCODE_ENUMERATION[0xf0] = "CREATE"
	OPCODE_ENUMERATION[0xf1] = "CALL"
	OPCODE_ENUMERATION[0xf2] = "CALLCODE"
	OPCODE_ENUMERATION[0xf3] = "RETURN"
	OPCODE_ENUMERATION[0xf4] = "DELEGATECALL"
	OPCODE_ENUMERATION[0xf5] = "CREATE2"
	OPCODE_ENUMERATION[0xfa] = "STATICCALL"
	OPCODE_ENUMERATION[0xfd] = "REVERT"
	OPCODE_ENUMERATION[0xfe] = "INVALID"
	OPCODE_ENUMERATION[0xff] = "SELFDESTRUCT"
}
