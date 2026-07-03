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
package zkc

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/debug"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var compileCmd = &cobra.Command{
	Use:   "compile [flags] file1.zkc file2.zkc ...",
	Short: "compile zkc source files into a binary package.",
	Long:  `Compile a given set of source file(s) into a single binary package.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, compileCmds)
	},
}

// Available instances
var compileCmds = []FieldAgnosticCmd{
	{field.GF_251, runCompileCmd[gf251.Element]},
	{field.GF_8209, runCompileCmd[gf8209.Element]},
	{field.KOALABEAR_16, runCompileCmd[koalabear.Element]},
	{field.BLS12_377, runCompileCmd[bls12_377.Element]},
}

// CompileConfig brings together various configuration options specific to this
// command.
type CompileConfig struct {
	build BuildConfig
	// indicates whether or not to print the Abstract Syntax Tree (when available)
	ast bool
	// indicates whether or not to print the Intermediate Reprentation
	ir bool
	// indicates whether or not to print the Mid-level Intermediate Reprentation (MIR).
	mir bool
	// indicates whether or not to print the Mid-level Intermediate Reprentation (AIR).
	air bool
	// indicates whether or not to print everything in full (e.g. including
	// static reference tables).
	verbose bool
}

func runCompileCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build  = GetBuildConfig[F](cmd, field)
		output = GetString(cmd, "output")
		config CompileConfig
	)
	//
	config.ast = GetFlag(cmd, "ast")
	config.mir = GetFlag(cmd, "mir")
	config.air = GetFlag(cmd, "air")
	config.ir = !(config.ast || config.mir || config.air)
	config.verbose = GetFlag(cmd, "verbose")
	// Build all artifacts
	artifacts := Build[F](build, args...)
	//
	if output != "" {
		writeArtifacts[F](output, build, artifacts)
	} else {
		// Print out requested artifacts
		printArtifacts[F](field, artifacts, config)
	}
}

func writeArtifacts[F field.Element[F]](filename string, build BuildConfig, artifacts BuildArtifacts) {
	// Word-level Intermediate Representation
	//nolint
	// Construct binary file
	var binfile = constraints.NewBinaryFile[F](build.metadata, nil, build.field,
		build.config.GetMaxStaticDepth(), artifacts.wir)
	// Write to disk
	WriteBinaryFile(binfile, filename)
}

func printArtifacts[F field.Element[F]](field field.Config, artifacts BuildArtifacts, config CompileConfig) {
	// lower to a 64bit machine
	var (
		m64 = vm.WordToWordMachine[vm.Uint, vm.Uint64](&artifacts.wir)
		// Compile bytecode interpreter
		bci = vm.WordToBytecodeProgram(m64)
	)
	// Abstract Syntax Tree
	if config.ast && artifacts.ast.HasValue() {
		writeAbstractSyntaxTree(artifacts.ast.Unwrap())
	} else if config.ast {
		log.Warn("Abstract Syntax Tree unavailable")
	}
	// Word-level Intermediate Representation
	if config.ir {
		writeIntermediateRepresentation(artifacts.wir, artifacts.annotations)
		// Bytecode Intermediate Representation
		writeBytecodeProgram(true, bci, artifacts.annotations)
	}
	// Mid-level Intermediate Representation
	if config.mir {
		fir := vm.WordToFieldMachine[vm.Uint, F](field, &artifacts.wir)
		mir := constraints.GenerateMirConstraints(fir, config.build.config.GetMaxStaticDepth())
		//
		debug.PrintAnySchema(mir, 80, config.verbose)
	}
	// // Arithmetic Intermediate Representation
	if config.mir {
		fir := vm.WordToFieldMachine[vm.Uint, F](field, &artifacts.wir)
		air := constraints.GenerateAirConstraints(fir, field, config.build.config.GetMaxStaticDepth())
		//
		debug.PrintAnySchema(air, 80, config.verbose)
	}
}

// ============================================================================
// AST
// ============================================================================

func writeAbstractSyntaxTree(program ast.Program) {
	var env = ast.NewEnvironment()
	//
	for i, d := range program.Components() {
		if i != 0 {
			fmt.Println()
		}
		//
		writeDeclaration(d, env)
	}
}

func writeDeclaration(d decl.Resolved, env data.ResolvedEnvironment) {
	switch d := d.(type) {
	case *decl.ResolvedConstant:
		writeConstant(d, env)
	case *decl.ResolvedFunction:
		writeFunction(d, env)
	case *decl.ResolvedMemory:
		writeMemory(d, env)
	case *decl.ResolvedTypeAlias:
		writeTypeAlias(d, env)
	default:
		panic(fmt.Sprintf("unknown declaration encountered (%v)", d))
	}
}

func writeTypeAlias(t *decl.ResolvedTypeAlias, env data.ResolvedEnvironment) {
	fmt.Printf("type %s = %s\n", t.Name(), t.DataType.String(env))
}

func writeConstant(m *decl.ResolvedConstant, env data.ResolvedEnvironment) {
	var mapping = variable.ArrayMap[symbol.Resolved]()
	//
	fmt.Print("const ")
	// type
	fmt.Printf("%s ", m.DataType.String(env))
	// name
	fmt.Printf("%s = ", m.Name())
	// contents
	fmt.Println(m.ConstExpr.String(mapping))
}

func writeAnnotations(annotations []string) {
	for _, a := range annotations {
		fmt.Printf("#[%s]\n", a)
	}
}

func writeMemory(m *decl.ResolvedMemory, env data.ResolvedEnvironment) {
	writeAnnotations(m.Annotations())
	//
	switch m.Kind {
	case decl.PUBLIC_READ_ONLY_MEMORY:
		fmt.Printf("public input")
	case decl.PRIVATE_READ_ONLY_MEMORY:
		fmt.Printf("input")
	case decl.PUBLIC_WRITE_ONCE_MEMORY:
		fmt.Printf("public output")
	case decl.PRIVATE_WRITE_ONCE_MEMORY:
		fmt.Printf("output")
	case decl.PUBLIC_STATIC_MEMORY:
		fmt.Printf("public static")
	case decl.PRIVATE_STATIC_MEMORY:
		fmt.Printf("static")
	case decl.RANDOM_ACCESS_MEMORY:
		fmt.Printf("memory")
	}
	// address lines
	fmt.Printf(" %s(", m.Name())
	writeMemoryParams(m.Address, env)
	fmt.Printf(") -> (")
	writeMemoryParams(m.Data, env)
	fmt.Printf(")")
	//
	if m.Contents != nil {
		fmt.Println(" = {")
		writeMemoryContents(m.Contents)
		fmt.Printf("}")
	}
	//
	fmt.Println()
}

func writeMemoryParams(params []variable.ResolvedDescriptor, env data.ResolvedEnvironment) {
	for i, p := range params {
		if i > 0 {
			fmt.Printf(", ")
		}

		fmt.Printf("%s %s", p.DataType.String(env), p.Name)
	}
}

func writeMemoryContents(values []expr.Resolved) {
	var N = 20
	//
	for i := 0; i < len(values); i += N {
		var left = len(values) - i
		//
		for j := range min(N, left) {
			fmt.Printf("%s", values[i+j].String(variable.ArrayMap[symbol.Resolved]()))
			//
			if i+j+1 != len(values) {
				fmt.Printf(", ")
			}
		}
		//
		fmt.Println()
	}
}

func writeFunction(f *decl.ResolvedFunction, env data.ResolvedEnvironment) {
	writeAnnotations(f.Annotations())
	//
	fmt.Printf("fn %s", f.Name())
	// Write optional effects
	if len(f.Effects) > 0 {
		writeEffects(f.Effects)
	}
	//
	fmt.Printf("(")
	// parameters
	writeFunctionArgs(variable.PARAMETER, f.Variables, env)
	//
	fmt.Printf(") -> (")
	// returns
	writeFunctionArgs(variable.RETURN, f.Variables, env)
	//
	fmt.Println(") {")
	//
	writeFunctionVariables(f, env)
	//
	for pc, insn := range f.Code {
		fmt.Printf("[%d]\t%s\n", pc, insn.String(f))
	}
	// Done
	fmt.Println("}")
}

func writeEffects(effects []*symbol.Resolved) {
	fmt.Print("<")
	//
	for i, effect := range effects {
		if i != 0 {
			fmt.Print(",")
		}
		//
		fmt.Print(effect)
	}
	//
	fmt.Print(">")
}

func writeFunctionArgs(kind variable.Kind, variables []variable.ResolvedDescriptor, env data.ResolvedEnvironment) {
	var first = true
	//
	for _, r := range variables {
		if r.Kind == kind {
			if !first {
				fmt.Printf(", ")
			} else {
				first = false
			}
			//
			fmt.Printf("%s:%s", r.Name, r.DataType.String(env))
		}
	}
}

func writeFunctionVariables(f *decl.ResolvedFunction, env data.ResolvedEnvironment) {
	for _, r := range f.Variables {
		if r.IsLocal() {
			fmt.Printf("\tvar %s:%s\n", r.Name, r.DataType.String(env))
		}
	}
}

// ============================================================================
// Intermediate Representation (IR)
// ============================================================================

func writeIntermediateRepresentation[W vm.MachineWord[W], I vm.Instruction, T vm.Executor[W, I]](
	machine vm.BaseMachine[W, I, T], annotations map[string][]string) {
	//
	// Write memories
	for i, m := range machine.Modules() {
		if i != 0 {
			fmt.Println()
		}
		//
		writeAnnotations(annotations[m.Name()])
		//
		switch m := m.(type) {
		case vm.Memory[W]:
			writeIrMemory(m)
		case *vm.Function[I]:
			mapping := instruction.NewSystemMap(m.RegisterMap(), machine.Modules())
			writeIrFunction[W](m, mapping)
		}
	}
}

func writeIrMemory[W vm.MachineWord[W]](m vm.Memory[W]) {
	var (
		regs = m.Geometry().Registers()
		kind = memoryKind(m)
	)
	//
	fmt.Printf("%s %s(", kind, m.Name())
	// parameters
	writeIrFunctionArgs(register.INPUT_REGISTER, regs)
	//
	fmt.Printf(")")
	//
	fmt.Printf(" -> (")
	// returns
	writeIrFunctionArgs(register.OUTPUT_REGISTER, regs)
	//
	fmt.Println(")")
}

func writeIrFunction[W vm.MachineWord[W], I vm.Instruction](f *vm.Function[I], mapping instruction.SystemMap) {
	fmt.Printf("fn %s(", f.Name())
	// parameters
	writeIrFunctionArgs(register.INPUT_REGISTER, f.Registers())
	//
	fmt.Printf(")")
	//
	if f.NumOutputs() != 0 {
		//
		fmt.Printf(" -> (")
		// returns
		writeIrFunctionArgs(register.OUTPUT_REGISTER, f.Registers())
		//
		fmt.Printf(")")
	}
	//
	fmt.Println(" {")
	//
	writeIrFunctionVariables[W](f)
	//
	for pc, insn := range f.Code() {
		fmt.Printf("[%d]\t%s\n", pc, insn.String(mapping))
	}
	// Done
	fmt.Println("}")
}

func writeIrFunctionArgs(kind register.Type, regs []register.Register) {
	var first = true
	//
	for _, r := range regs {
		if r.Kind() == kind {
			if !first {
				fmt.Printf(", ")
			} else {
				first = false
			}
			//
			fmt.Printf("%s %s", registerType(r), r.Name())
		}
	}
}

func writeIrFunctionVariables[W vm.MachineWord[W], I vm.Instruction](f *vm.Function[I]) {
	for _, r := range f.Registers() {
		if !r.IsInputOutput() {
			fmt.Printf("\t%s %s\n", registerType(r), r.Name())
		}
	}
}

func memoryKind[W vm.MachineWord[W]](m vm.Memory[W]) string {
	switch {
	case m.IsStatic():
		return "static"
	case m.IsReadOnly():
		return "input"
	case m.IsWriteOnly():
		return "output"
	default:
		return "memory"
	}
}

func registerType(r register.Register) string {
	if r.IsNative() {
		return "𝔽"
	}
	//
	return fmt.Sprintf("u%d", r.Width())
}

// ============================================================================
// Bytecode Interpreter
// ============================================================================

func writeBytecodeProgram[W vm.Word[W]](binary bool, program vm.Program[W], annotations map[string][]string) {
	var (
		bin     [][]uint32
		address uint32
	)
	//
	if binary {
		// Extract encoding for all bytecodes
		bin = vm.CompileProgram(program).Encoding()
	}
	//
	for i, m := range program.Modules() {
		if i != 0 {
			fmt.Println()
		}
		// Write any annotations for this module
		writeAnnotations(annotations[m.Name()])
		//  Write module contents
		switch m := m.(type) {
		case *vm.BytecodeFunction[W]:
			fid := uint16(i)
			address, bin = writeBytecodeFunction(address, program.EnvironmentOf(fid), m, bin)
		case *vm.BytecodeMemory[W]:
			writeBytecodeMemory(m)
		default:
			panic(fmt.Sprintf("unknown module \"%s\" encountered", m.Name()))
		}
	}
}

func writeBytecodeFunction[W vm.Word[W]](address uint32, env vm.BytecodeEnvironment,
	f *vm.BytecodeFunction[W], bin [][]uint32) (uint32, [][]uint32) {
	//
	var width = maxBytecodeLength(f, bin)
	//
	fmt.Printf("fn %s\n", signatureOf(f))
	//
	for _, r := range f.Registers() {
		if !r.IsInputOutput() {
			fmt.Printf("\t%s %s\n", regType(r), r.Name())
		}
	}
	//
	for pc, vec := range f.Vectors() {
		for cc, b := range vec.Bytecodes {
			// Include low-level information (if requested)
			if bin != nil {
				var codes = array.Reverse(bin[0])
				//
				fmt.Printf("0x%04x\t%s\t", address, codeStr(width, codes))
				//
				address += uint32(len(codes))
				bin = bin[1:]
			}
			//
			if cc == 0 {
				fmt.Printf("[%02d]", pc)
			} else {
				fmt.Print("    ")
			}
			// Sanity check to prevent crashing even in the presence of invalid
			// structure.
			if b == nil {
				fmt.Print("\t???")
			} else {
				fmt.Printf("\t%s", b.String(env))
			}
			//
			fmt.Println()
		}
	}
	//
	return address, bin
}

func regType[W vm.Word[W]](r vm.Register[W]) string {
	if r.IsNative() {
		return "𝔽"
	}
	//
	return fmt.Sprintf("u%d", r.Bitwidth().Unwrap())
}

func maxBytecodeLength[W vm.Word[W]](f *vm.BytecodeFunction[W], encoding [][]uint32) uint {
	var maxWidth int
	//
	if encoding == nil {
		// Width doesn't matter when no low-level information is requested.
		return 0
	}
	//
	for _, vec := range f.Vectors() {
		for range vec.Bytecodes {
			maxWidth = max(maxWidth, len(encoding[0]))
			encoding = encoding[1:]
		}
	}
	//
	return uint(maxWidth)
}

func writeBytecodeMemory[W vm.Word[W]](m *vm.BytecodeMemory[W]) {
	if m.IsPublic() {
		fmt.Printf("pub ")
	}
	//
	switch {
	case m.IsStatic():
		fmt.Print("static ")
	case m.IsReadOnly():
		fmt.Print("input ")
	case m.IsWriteOnly():
		fmt.Print("output ")
	default:
		fmt.Print("memory ")
	}
	//
	fmt.Println(signatureOf(m))
}

func codeStr(width uint, codes []uint32) string {
	var (
		n   = (width * 9) + 1
		str = fmt.Sprintf("%08x", codes)
	)
	//
	return fmt.Sprintf("%-*s", n, str)
}

func signatureOf[W vm.Word[W]](m vm.BytecodeModule[W]) string {
	var (
		args = array.Filter(m.Registers(), func(r vm.Register[W]) bool {
			return r.IsInput()
		})
		returns = array.Filter(m.Registers(), func(r vm.Register[W]) bool {
			return r.IsOutput()
		})
	)
	//
	return fmt.Sprintf("%s(%s) -> (%s)", m.Name(), fnArgs(args), fnArgs(returns))
}

func fnArgs[W vm.Word[W]](regs []vm.Register[W]) string {
	var builder strings.Builder
	//
	for i, r := range regs {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(r.Name())
		builder.WriteString(":")
		//
		if r.IsNative() {
			builder.WriteString("𝔽")
		} else {
			fmt.Fprintf(&builder, "u%d", r.Bitwidth().Unwrap())
		}
	}
	//
	return builder.String()
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(compileCmd)
	compileCmd.Flags().StringP("output", "o", "", "specify output file for writing binary constraints")
	compileCmd.PersistentFlags().Bool("ast", false, "Output Abstract Syntax Tree (AST)")
	compileCmd.PersistentFlags().Bool("bci", false, "Output Bytecode Representation (BCI)")
	compileCmd.PersistentFlags().Bool("mir", false, "Output Mid-Level Intermediate Representation (MIR)")
	compileCmd.PersistentFlags().Bool("air", false, "Output Arithmetic Intermediate Representation (AIR)")
}
