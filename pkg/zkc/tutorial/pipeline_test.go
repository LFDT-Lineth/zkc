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

package tutorial

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

func TestTutorial01SourceToAST(t *testing.T) {
	// First stop: the frontend. compiler.Compile parses the in-memory file,
	// resolves names, flattens block control-flow, type-checks, and validates.
	// It does not generate VM instructions yet.
	program, _, err := CompileProgram()
	if err != nil {
		t.Fatalf("compile source: %v", err)
	}

	// The source declares exactly three top-level components: input memory,
	// output memory, and the boot function.
	names := componentNames(program)
	assertEqual(t, names, []string{"args", "result", "main"})

	// Keeping this assertion in the tutorial catches accidental drift if the
	// example source changes.
	if SourceFilename != "tutorial.zkc" {
		t.Fatalf("unexpected source filename: %q", SourceFilename)
	}

	if KoalaBearConfig().Name != "KOALABEAR_16" {
		t.Fatalf("wrong field: got %s", KoalaBearConfig().Name)
	}
}

func TestTutorial02ASTToWordMachine(t *testing.T) {
	pipeline, err := BuildPipeline()
	if err != nil {
		t.Fatalf("build pipeline: %v", err)
	}

	// Second stop: code generation. The machine has one module per executable
	// thing: input memory, output memory, and function.
	moduleNames := machineModuleNames(pipeline.DebugMachine)
	assertEqual(t, moduleNames, []string{"args", "result", "main"})

	// The two memory modules both have an address register and a data register.
	// In the .zkc source those are "address:u16" and "word:u16".
	args := mustMemory(t, pipeline.DebugMachine, "args")
	result := mustMemory(t, pipeline.DebugMachine, "result")
	assertEqual(t, registerSummaries(args.Registers()), []string{
		"address:input:16",
		"word:output:16",
	})
	assertEqual(t, registerSummaries(result.Registers()), []string{
		"address:input:16",
		"word:output:16",
	})

	if !args.IsReadOnly() || !args.IsPublic() {
		t.Fatalf("args memory should be public read-only")
	}

	if !result.IsWriteOnly() || !result.IsPublic() {
		t.Fatalf("result memory should be public write-only")
	}

	// In the debug machine, vectorisation is disabled. Each source-level action
	// is therefore visible as its own macro instruction.
	main := mustFunction(t, pipeline.DebugMachine, "main")
	if got, want := len(main.Code()), 8; got != want {
		t.Fatalf("unvectorized main instruction count: got %d, want %d", got, want)
	}

	assertEqual(t, registerSummaries(main.Registers()), []string{
		"a:computed:16",
		"b:computed:16",
		"c:computed:16",
		"sum:computed:16",
		"$4:computed:16",
		"$5:computed:16",
		"$6:computed:16",
		"$7:computed:16",
		"$8:computed:16",
		"$9:computed:16",
		"$10:computed:16",
		"$11:computed:16",
		"$12:computed:16",
	})

	// These strings are not intended as golden docs for every opcode. They show
	// the important shape: compiler temporaries hold memory addresses / values,
	// reads come from args, arithmetic happens in registers, writes go to
	// result, and the function returns.
	assertEqual(t, instructionStrings(pipeline.DebugMachine, main), []string{
		"$4 = 0x0",
		"a = args[$4]",
		"$5 = 0x1",
		"b = args[$5]",
		"$6 = 0x2",
		"c = args[$6]",
		"sum = a + b",
		"$7 = 0x0",
		"$8 = sum",
		"result[$7] = $8",
		"$9 = 0x1",
		"$10 = sum * c * 0x1",
		"result[$9] = $10",
		"$11 = 0x2",
		"$12 = a - b",
		"result[$11] = $12",
		"ret",
	})
}

func TestTutorial03VectorizationCompactsWordMachine(t *testing.T) {
	pipeline, err := BuildPipeline()
	if err != nil {
		t.Fatalf("build pipeline: %v", err)
	}

	debugMain := mustFunction(t, pipeline.DebugMachine, "main")
	proverMain := mustFunction(t, pipeline.ProverMachine, "main")

	// Vectorisation groups non-conflicting micro-instructions into VLIW-style
	// macro instructions. The same program still runs, but the prover-facing
	// instruction stream has fewer rows to represent.
	if got, want := len(debugMain.Code()), 8; got != want {
		t.Fatalf("debug macro instructions: got %d, want %d", got, want)
	}

	if got, want := len(proverMain.Code()), 1; got != want {
		t.Fatalf("prover macro instructions: got %d, want %d", got, want)
	}

	// The straight-line program is compact enough that every micro instruction
	// fits into one vector macro instruction.
	assertEqual(t, macroInstructionStrings(pipeline.ProverMachine, proverMain), []string{
		"$4 = 0x0 ; a = args[$4] ; $5 = 0x1 ; b = args[$5] ; $6 = 0x2 ; " +
			"c = args[$6] ; sum = a + b ; $7 = 0x0 ; $8 = sum ; result[$7] = $8 ; " +
			"$9 = 0x1 ; $10 = sum * c * 0x1 ; result[$9] = $10 ; $11 = 0x2 ; " +
			"$12 = a - b ; result[$11] = $12 ; ret",
	})
}

func TestTutorial04ExecuteWordMachine(t *testing.T) {
	input := InputBytes(7, 3, 2)
	expected := ExpectedOutputBytes(10, 20, 4)

	output, err := Execute(input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Raw bytes are the external boundary. For u16 rows, 10,20,4 becomes
	// 00 0a 00 14 00 04.
	if !bytes.Equal(output["result"], expected["result"]) {
		t.Fatalf("encoded output: got %x, want %x", output["result"], expected["result"])
	}

	// The same assertion as numbers is easier to read when following the code in
	// an IDE.
	assertEqual(t, UnpackU16(output["result"]), []uint16{10, 20, 4})
}

func TestTutorial05DecodeInputsAndOutputs(t *testing.T) {
	machine, err := CompileMachine(ProverCodegenConfig())
	if err != nil {
		t.Fatalf("compile machine: %v", err)
	}

	// vm.DecodeInputsOutputs is the project API that separates read-only input
	// memories from write-once output expectations. This is exactly what the
	// existing fixture tests do with JSON, just without a fixture file.
	data := map[string][]byte{}
	data["args"] = PackU16(42, 11, 5)
	data["result"] = PackU16(53, 265, 31)

	inputs, outputs, errs := vm.DecodeInputsOutputs(machine, data)
	if len(errs) != 0 {
		t.Fatalf("decode inputs/outputs: %v", errs)
	}

	assertEqual(t, uint64Words(inputs["args"]), []uint64{42, 11, 5})
	assertEqual(t, uint64Words(outputs["result"]), []uint64{53, 265, 31})
}

func TestTutorial06BinaryFileRoundTrip(t *testing.T) {
	binf, err := NewBinaryFile()
	if err != nil {
		t.Fatalf("new binary file: %v", err)
	}

	decoded, byteLen, err := RoundTripBinaryFile(binf)
	if err != nil {
		t.Fatalf("round trip binary file: %v", err)
	}

	// This is the in-memory form of "compile constraints to a zkc exec file and
	// load it again", without touching cmd/zkc or the filesystem.
	if byteLen <= 0 {
		t.Fatalf("empty binary file")
	}

	if decoded.Field() != KoalaBearConfig() {
		t.Fatalf("decoded field: got %s, want %s", decoded.Field().Name, KoalaBearConfig().Name)
	}

	assertEqual(t, machineModuleNamesPtr(decoded.WordMachine()), []string{"args", "result", "main"})

	output, errs := decoded.Execute(InputBytes(7, 3, 2), MaxExecutionSteps)
	if len(errs) != 0 {
		t.Fatalf("execute decoded binary: %v", errs)
	}

	assertEqual(t, UnpackU16(output["result"]), []uint16{10, 20, 4})
}

func TestTutorial07AIRAndTraceCheck(t *testing.T) {
	binf, err := NewBinaryFile()
	if err != nil {
		t.Fatalf("new binary file: %v", err)
	}

	// AirConstraints lowers the prover machine from word instructions to field
	// instructions, MIR, and then AIR. These are the constraints that Check
	// evaluates against a concrete execution trace.
	air := binf.AirConstraints()
	if got, want := air.Width(), uint(3); got != want {
		t.Fatalf("AIR module count: got %d, want %d", got, want)
	}

	if got := air.Constraints().Count(); got == 0 {
		t.Fatalf("AIR should contain constraints")
	}

	traceConfig := constraints.DEFAULT_TRACE_CONFIG.WithParallelism(false)

	tr, errs := binf.Trace(InputBytes(7, 3, 2), traceConfig)
	if len(errs) != 0 {
		t.Fatalf("trace: %v", errs)
	}

	// Trace modules mirror the machine modules. In this tiny fully vectorized
	// program, main occupies one trace row. The output memory module is still
	// present structurally, but its write-once rows are represented through the
	// main memory-write instruction constraints in this AIR trace view.
	assertEqual(t, traceSummaries(tr), []string{
		"args:height=3:width=2",
		"result:height=0:width=2",
		"main:height=1:width=13",
	})

	if failures := binf.Check(tr, traceConfig); len(failures) != 0 {
		t.Fatalf("constraint failures: %s", failureMessages(failures))
	}
}

func componentNames(program ast.Program) []string {
	components := program.Components()

	names := make([]string, len(components))
	for i, component := range components {
		names[i] = component.Name()
	}

	return names
}

func machineModuleNames(machine *vm.WordMachine[vm.Uint]) []string {
	names := make([]string, len(machine.Modules()))
	for i, module := range machine.Modules() {
		names[i] = module.Name()
	}

	return names
}

func machineModuleNamesPtr(machine vm.WordMachine[vm.Uint]) []string {
	return machineModuleNames(&machine)
}

func mustFunction(t *testing.T, machine *vm.WordMachine[vm.Uint], name string) *vm.WordFunction {
	t.Helper()

	for _, module := range machine.Modules() {
		if fn, ok := module.(*vm.WordFunction); ok && fn.Name() == name {
			return fn
		}
	}

	t.Fatalf("missing function %q", name)

	return nil
}

func mustMemory(t *testing.T, machine *vm.WordMachine[vm.Uint], name string) vm.Memory[vm.Uint] {
	t.Helper()

	for _, module := range machine.Modules() {
		if memory, ok := module.(vm.Memory[vm.Uint]); ok && memory.Name() == name {
			return memory
		}
	}

	t.Fatalf("missing memory %q", name)

	return nil
}

func instructionStrings(machine *vm.WordMachine[vm.Uint], fn *vm.WordFunction) []string {
	mapping := instruction.NewSystemMap(fn.RegisterMap(), machine.Modules())
	strings := make([]string, 0)

	for _, vec := range fn.Code() {
		for _, code := range vec.Codes {
			strings = append(strings, code.String(mapping))
		}
	}

	return strings
}

func macroInstructionStrings(machine *vm.WordMachine[vm.Uint], fn *vm.WordFunction) []string {
	mapping := instruction.NewSystemMap(fn.RegisterMap(), machine.Modules())

	strings := make([]string, len(fn.Code()))
	for i, vec := range fn.Code() {
		strings[i] = vec.String(mapping)
	}

	return strings
}

func registerSummaries(registers []register.Register) []string {
	summaries := make([]string, len(registers))
	for i, reg := range registers {
		summaries[i] = fmt.Sprintf("%s:%s:%d", reg.Name(), registerKind(reg), reg.Width())
	}

	return summaries
}

func registerKind(reg register.Register) string {
	switch {
	case reg.IsInput():
		return "input"
	case reg.IsOutput():
		return "output"
	case reg.IsComputed():
		return "computed"
	default:
		return "unknown"
	}
}

func uint64Words(words []vm.Uint) []uint64 {
	values := make([]uint64, len(words))
	for i, word := range words {
		values[i] = word.Uint64()
	}

	return values
}

func traceSummaries(tr trace.Trace[koalabear.Element]) []string {
	modules := tr.Modules().Collect()

	summaries := make([]string, len(modules))
	for i, module := range modules {
		summaries[i] = fmt.Sprintf("%s:height=%d:width=%d", module.Name(), module.Height(), module.Width())
	}

	return summaries
}

func failureMessages[T interface{ Message() string }](failures []T) []string {
	messages := make([]string, len(failures))
	for i, failure := range failures {
		messages[i] = failure.Message()
	}

	return messages
}

func assertEqual[T any](t *testing.T, got, want T) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
