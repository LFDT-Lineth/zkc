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

// Package tutorial is an executable companion to docs/zkc-deep-dive.html.
//
// It deliberately avoids cmd/zkc: every step starts from a .zkc program held in
// a Go string, calls the compiler / VM / constraint packages directly, and lets
// the unit tests assert the concrete artifacts produced along the way.
package tutorial

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

const (
	// SourceFilename is only used for source maps and diagnostics. The source
	// still lives entirely in memory.
	SourceFilename = "tutorial.zkc"
	// MaxExecutionSteps is intentionally generous for this tiny program. If the
	// VM ever loops, tests fail with the execution error instead of hanging.
	MaxExecutionSteps = 1024
)

// Source is the whole ZkC program used by the tutorial.
//
// The program has one public input memory and one public output memory. Each
// memory row contains one u16 data word, addressed by a u16 index. main reads
// three input rows and writes three derived output rows:
//
//   - result[0] = a + b
//   - result[1] = (a + b) * c
//   - result[2] = a - b
//
// All chosen test vectors keep the arithmetic inside u16 so the tutorial can
// focus on the pipeline rather than overflow behavior.
const Source = `pub input args(address:u16) -> (word:u16)
pub output result(address:u16) -> (word:u16)

fn main() {
    var a:u16 = args[0]
    var b:u16 = args[1]
    var c:u16 = args[2]
    var sum:u16 = a + b

    result[0] = sum
    result[1] = sum * c
    result[2] = a - b
    return
}
`

// Pipeline keeps the major artifacts for the same source program together so a
// test can inspect whichever stage it is trying to explain.
type Pipeline struct {
	// Program is the linked, validated, high-level AST. The compiler has parsed
	// the source string and resolved references such as args/result/main.
	Program ast.Program
	// SourceMaps connect AST / generated nodes back to SourceFilename.
	SourceMaps source.Maps[any]
	// DebugMachine disables vectorisation so the code stream is easy to read one
	// macro instruction at a time.
	DebugMachine *vm.WordMachine[vm.Uint]
	// ProverMachine uses the production-facing KoalaBear codegen settings used
	// by BinaryFile, AIR generation, trace generation, and constraint checking.
	ProverMachine *vm.WordMachine[vm.Uint]
	// BinaryFile is the in-memory equivalent of what zkc would serialize for a
	// prover. It owns a machine and lazily builds AIR constraints from it.
	BinaryFile *constraints.BinaryFile[koalabear.Element]
}

// KoalaBearConfig centralizes the field choice for the tutorial. Production
// callers care about KoalaBear here, so the package intentionally avoids BLS
// examples.
func KoalaBearConfig() field.Config {
	return field.KOALABEAR_16
}

// DebugCodegenConfig returns a machine that is easier to inspect in tests:
// vectorisation is disabled, while the target field stays KoalaBear.
func DebugCodegenConfig() codegen.Config {
	return codegen.DEFAULT_CONFIG.
		Field(KoalaBearConfig()).
		LowerNatives(false).
		Vectorize(false)
}

// ProverCodegenConfig returns the configuration used for constraint-oriented
// stages. LowerNatives is enabled because native VM operations must be lowered
// before AIR constraints can represent them.
func ProverCodegenConfig() codegen.Config {
	return codegen.DEFAULT_CONFIG.
		Field(KoalaBearConfig()).
		LowerNatives(true).
		Vectorize(true)
}

// CompileProgram parses, links, lowers block control-flow, type-checks, and
// validates Source. It stops before VM code generation.
func CompileProgram() (ast.Program, source.Maps[any], error) {
	src := source.NewSourceFile(SourceFilename, []byte(Source))

	program, srcmaps, errs := compiler.Compile(KoalaBearConfig(), *src)
	if len(errs) != 0 {
		return ast.Program{}, source.Maps[any]{}, syntaxErrors(errs)
	}

	return program, srcmaps, nil
}

// CompileMachine compiles the validated high-level program into a WordMachine.
// Supplying different Config values lets tests compare pre-vectorized and
// prover-facing shapes without reparsing details hidden by a CLI.
func CompileMachine(cfg codegen.Config) (*vm.WordMachine[vm.Uint], error) {
	program, _, err := CompileProgram()
	if err != nil {
		return nil, err
	}

	machine, errs := program.Compile(cfg)
	if len(errs) != 0 {
		return nil, syntaxErrors(errs)
	}

	return machine, nil
}

// BuildPipeline constructs all tutorial artifacts from the in-memory source.
func BuildPipeline() (*Pipeline, error) {
	program, srcmaps, err := CompileProgram()
	if err != nil {
		return nil, err
	}

	debugMachine, errs := program.Compile(DebugCodegenConfig())
	if len(errs) != 0 {
		return nil, syntaxErrors(errs)
	}

	proverMachine, errs := program.Compile(ProverCodegenConfig())
	if len(errs) != 0 {
		return nil, syntaxErrors(errs)
	}

	binf := constraints.NewBinaryFile[koalabear.Element](nil, nil, KoalaBearConfig(), *proverMachine)

	return &Pipeline{
		Program:       program,
		SourceMaps:    srcmaps,
		DebugMachine:  debugMachine,
		ProverMachine: proverMachine,
		BinaryFile:    binf,
	}, nil
}

// NewBinaryFile compiles a fresh in-memory binary file. This is useful because
// execution mutates VM memory modules; fresh binaries avoid accidental state
// sharing in tests and benchmarks.
func NewBinaryFile() (*constraints.BinaryFile[koalabear.Element], error) {
	machine, err := CompileMachine(ProverCodegenConfig())
	if err != nil {
		return nil, err
	}

	return constraints.NewBinaryFile[koalabear.Element](nil, nil, KoalaBearConfig(), *machine), nil
}

// RoundTripBinaryFile serializes and deserializes the prover artifact. The
// returned value owns a fresh VM, which is a cheap way for tests to demonstrate
// exactly what survives the binary-file boundary.
func RoundTripBinaryFile(binf *constraints.BinaryFile[koalabear.Element],
) (*constraints.BinaryFile[koalabear.Element], int, error) {
	data, err := binf.MarshalBinary()
	if err != nil {
		return nil, 0, err
	}

	var decoded constraints.BinaryFile[koalabear.Element]
	if err := decoded.UnmarshalBinary(data); err != nil {
		return nil, 0, err
	}

	return &decoded, len(data), nil
}

// Execute runs a fresh compiled binary file and returns the raw encoded output
// bytes. For this tutorial, the raw bytes are useful because they show the
// boundary format accepted by vm.DecodeInputs / vm.EncodeOutputs.
func Execute(input map[string][]byte) (map[string][]byte, error) {
	binf, err := NewBinaryFile()
	if err != nil {
		return nil, err
	}

	output, errs := binf.Execute(input, MaxExecutionSteps)
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	return output, nil
}

// InputBytes packs u16 words exactly as the VM memory decoder expects them:
// big-endian, tightly packed, one word after another.
func InputBytes(values ...uint16) map[string][]byte {
	return map[string][]byte{"args": PackU16(values...)}
}

// ExpectedOutputBytes is the same encoding for the tutorial result memory.
func ExpectedOutputBytes(values ...uint16) map[string][]byte {
	return map[string][]byte{"result": PackU16(values...)}
}

// PackU16 converts readable Go integers into the raw byte representation used
// for a memory whose data row is one u16.
func PackU16(values ...uint16) []byte {
	bytes := make([]byte, 2*len(values))
	for i, value := range values {
		binary.BigEndian.PutUint16(bytes[2*i:], value)
	}

	return bytes
}

// UnpackU16 is the inverse of PackU16. Tests use it when the assertion is more
// readable as numbers than as a byte string.
func UnpackU16(bytes []byte) []uint16 {
	if len(bytes)%2 != 0 {
		panic(fmt.Sprintf("u16 byte slice has odd length %d", len(bytes)))
	}

	values := make([]uint16, len(bytes)/2)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(bytes[2*i:])
	}

	return values
}

type syntaxErrors []source.SyntaxError

func (p syntaxErrors) Error() string {
	var builder strings.Builder

	for i, err := range p {
		if i != 0 {
			builder.WriteString("\n")
		}

		builder.WriteString(err.Error())
	}

	return builder.String()
}
