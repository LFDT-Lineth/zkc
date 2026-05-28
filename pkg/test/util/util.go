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
package util

import (
	"fmt"
	"strings"
	"testing"

	cmd_util "github.com/consensys/go-corset/pkg/cmd/zkc"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/field/bls12_377"
	"github.com/consensys/go-corset/pkg/util/field/gf251"
	"github.com/consensys/go-corset/pkg/util/field/gf8209"
	"github.com/consensys/go-corset/pkg/util/field/koalabear"
	"github.com/consensys/go-corset/pkg/util/file"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler"
	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen"
	"github.com/consensys/go-corset/pkg/zkc/constraints"
	"github.com/consensys/go-corset/pkg/zkc/util"
	"github.com/consensys/go-corset/pkg/zkc/vm"
)

// TestCase represents a line in a file
type TestCase struct {
	// name of enclosing file
	filename string
	// line in the file reprensented by this test
	line uint
	// indicates whether this test is expected to pass or fail.
	expected bool
	// raw data obtained from JSON
	data map[string][]byte
}

// CompileMachine compiles one or more zkc source files into a base machine for
// executing tests with.
func CompileMachine(field field.Config, srcfiles ...source.File) []source.SyntaxError {
	_, _, errors := compiler.Compile(field, srcfiles...)
	//
	return errors
}

// CompileZkc compiles a single zkc source file, potentially producing errors.
// This includes both the validation phase and the code generation phase.
func CompileZkc(field field.Config, srcfile source.File) []source.SyntaxError {
	program, _, errors := compiler.Compile(field, srcfile)
	if len(errors) == 0 {
		_, errors = program.Compile(codegen.DEFAULT_CONFIG)
	}
	//
	return errors
}

// ReadTestsFile reads a file containing zero or more tests expressed as JSON,
// where each test is on a separate line.
func ReadTestsFile(t *testing.T, cfg TestConfig, test string) []TestCase {
	//
	var (
		// Construct test filename
		filename = fmt.Sprintf("%s/%s.%s", TestDir, test, cfg.extension)
		// Read input file
		lines = file.ReadInputFileAsLines(filename)
		//
		tests []TestCase
	)
	// Read constraints line by line
	for i, line := range lines {
		// Parse input line as JSON
		if line != "" && !strings.HasPrefix(line, ";;") {
			// Read inputs / outputs
			data, err := util.ParseJsonInputFile([]byte(line))
			//
			if err != nil {
				msg := fmt.Sprintf("%s:%d: %s", filename, i+1, err)
				panic(msg)
			}
			//
			tests = append(tests, TestCase{filename, uint(i + 1), cfg.expected, data})
		}
	}

	return tests
}

func failIfErrors(t *testing.T, errs ...error) {
	//
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("unexpected tracing failure: %v", err)
		}
		// Don't continue
		t.FailNow()
	}
}

func compileTestProgram(t *testing.T, testfile string, cfg codegen.Config) (vm *vm.WordMachine[vm.Uint]) {
	var filename = fmt.Sprintf("%s/%s", TestDir, testfile)
	// Compile source file into Abstract Syntax Tree form.
	program := cmd_util.CompileSourceFiles(cfg.GetField(), filename)
	// Compile program into boot machine
	vm, errs := program.Compile(cfg)
	//
	//
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("%s", err.Error())
		}

		t.FailNow()
	}
	//
	return vm
}

func decodeInputsOutputs[W vm.Word[W]](t *testing.T, m vm.Machine[W], data map[string][]byte,
) (inputs map[string][]W, outputs map[string][]W) {
	inputs, outputs, errs := vm.DecodeInputsOutputs(m, data)
	//
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("%s", err.Error())
		}

		t.FailNow()
	}
	//
	return inputs, outputs
}

// Marshall / Unmarshall takes a machine and constructs a suitable BinaryFile
// for the given field configuration, and then marshalls it into a byte sequence
// and the unmarshalls this sequence back into a fresh machine.  The purpose of
// this is to ensure that the marshalling / unmarshalling process: (a) actually
// works; (b) does not change the machine internals in some subtle way.
func marshallUnmarshallMachine(m *vm.WordMachine[vm.Uint], f field.Config) *vm.WordMachine[vm.Uint] {
	switch f {
	case field.GF_251:
		return roundTripMachine[gf251.Element](m, f)
	case field.GF_8209:
		return roundTripMachine[gf8209.Element](m, f)
	case field.KOALABEAR_16:
		return roundTripMachine[koalabear.Element](m, f)
	case field.BLS12_377:
		return roundTripMachine[bls12_377.Element](m, f)
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func roundTripMachine[F field.Element[F]](m *vm.WordMachine[vm.Uint], f field.Config) *vm.WordMachine[vm.Uint] {
	var (
		original = constraints.NewBinaryFile[F](nil, nil, f, *m)
		decoded  constraints.BinaryFile[F]
	)
	//
	data, err := original.MarshalBinary()
	if err != nil {
		panic(fmt.Sprintf("marshalling machine failed: %s", err))
	}
	//
	if err := decoded.UnmarshalBinary(data); err != nil {
		panic(fmt.Sprintf("unmarshalling machine failed: %s", err))
	}
	//
	nm := decoded.WordMachine()
	//
	return &nm
}
