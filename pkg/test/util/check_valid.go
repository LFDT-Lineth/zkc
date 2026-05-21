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
	"testing"

	cmd_util "github.com/consensys/go-corset/pkg/cmd/zkc"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/field/bls12_377"
	"github.com/consensys/go-corset/pkg/util/field/gf251"
	"github.com/consensys/go-corset/pkg/util/field/gf8209"
	"github.com/consensys/go-corset/pkg/util/field/koalabear"
	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen"
	"github.com/consensys/go-corset/pkg/zkc/constraints"
	"github.com/consensys/go-corset/pkg/zkc/vm"
)

var (
	// DEFAULT_FIELDS set default fields for testing
	DEFAULT_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_16}
	// DEFAULT_CONFIG sets a default testing configuration
	DEFAULT_CONFIG = Config{fields: DEFAULT_FIELDS, constraints: false, nativeLowering: true, splitting: false}
)

// Config for testing
type Config struct {
	// Fields to test over
	fields []field.Config
	// enable constraints checking, or not.
	constraints bool
	// enable testing for native lowering
	nativeLowering bool
	// enable register splitting
	splitting bool
}

// Fields determines which fields to test over.
func (p Config) Fields(fields ...field.Config) Config {
	p.fields = fields
	//
	return p
}

// Constraints determines whether or not to check constraints.
func (p Config) Constraints(flag bool) Config {
	p.constraints = flag
	// One needs to lower zkc native to enable constraints checks
	if flag {
		p.nativeLowering = true
	}
	//
	return p
}

// NativeLowering determines whether or not to test native lowerings as well.
func (p Config) NativeLowering(flag bool) Config {
	p.nativeLowering = flag
	//
	return p
}

// Splitting determines whether or not to apply register splitting.
func (p Config) Splitting(flag bool) Config {
	p.splitting = flag
	//
	return p
}

// CheckValid checks that a given source file compiles without any errors.
// nolint
func CheckValid(t *testing.T, test, ext string, config Config) {
	// Enable testing each trace in parallel
	t.Parallel()
	//
	if len(config.fields) == 0 {
		panic("at least one target field is required")
	}
	// Check for each field requested
	for _, f := range config.fields {
		cfg := codegen.DEFAULT_CONFIG.SplitRegisters(config.splitting)
		// check wo lowering and wo constraints check
		checkValidInternal(t, test, ext, cfg.LowerZkcNative(false), false, f)
		// check whether to enable lowering as well. If constraints enable, must enable lowering as well.
		if config.nativeLowering || config.constraints {
			checkValidInternal(t, test, ext, cfg.LowerZkcNative(true), config.constraints, f)
		}
	}
}

func checkValidInternal(t *testing.T, test, ext string, codeCfg codegen.Config, constraints bool, field field.Config) {
	var filename = fmt.Sprintf("%s/%s.%s", TestDir, test, ext)
	// Compile source file into Abstract Syntax Tree form.
	program := cmd_util.CompileSourceFiles(field, filename)
	// Compile program into boot machine
	vm, errs := program.Compile(codeCfg.Field(field))
	for _, err := range errs {
		t.Errorf("%s", err.Error())
	}
	//
	if len(errs) > 0 {
		return
	}
	// Search for tests
	for _, cfg := range TESTFILE_EXTENSIONS {
		// Check suitable field
		if cfg.field == nil || *cfg.field == field {
			// Read tests from file
			tests := ReadTestsFile(t, cfg, field, test, vm)
			// Run execution tests
			for _, test := range tests {
				runExecutionTest(t, vm, test)
			}
			// Run constraint tests
			if constraints {
				for _, test := range tests {
					// FIXME: support reject tests
					if test.expected {
						runConstraintTest(t, vm, test)
					}
				}
			}
		}
	}
}

func runExecutionTest(t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase) {
	var (
		err  error
		errs []error
	)
	// Execute machine
	if err = wm.Boot("main", test.inputs); err == nil {
		// Execute it
		if _, err = vm.ExecuteAll(wm, 1024); err == nil && test.expected {
			// Check outputs match
			errs = append(errs, checkExpectedOutputs(test.outputs, wm)...)
		} else if err == nil && !test.expected {
			errs = append(errs, fmt.Errorf("test accepted incorrectly"))
		} else if !test.expected {
			// prevent error as this was expected
			err = nil
		}
	}
	// Include single error
	if err != nil {
		errs = append(errs, err)
	}
	// Fail if errors found
	for _, err := range errs {
		t.Errorf("%s:%d %v", test.filename, test.line, err)
	}
}

func runConstraintTest(t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase) {
	// Dispatch based on field config
	switch test.field {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, wm, test)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, wm, test)
	case field.KOALABEAR_16:
		testConstraintsWithField[koalabear.Element](t, wm, test)
	case field.BLS12_377:
		testConstraintsWithField[bls12_377.Element](t, wm, test)
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", test.field.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase) {
	//
	var (
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, test.field, *wm)
		// generate trace
		tr, errs = constraints.Trace(binf, test.inputs, constraints.DEFAULT_TRACE_CONFIG)
	)
	//
	if test.expected {
		// test expected to pass, but tracing generated failures.
		failIfErrors(t, errs...)
	}
	//
	failures := binf.Check(tr, constraints.DEFAULT_TRACE_CONFIG)
	// Determine whether trace accepted or not.
	accepted := len(failures) == 0
	// Process what happened versus what was supposed to happen.
	if !accepted && test.expected {
		//table.PrintTrace(tr)
		t.Errorf("Trace rejected incorrectly (%s:%d): %s", test.filename, test.line, failures)
	} else if accepted && !test.expected {
		//printTrace(tr)
		t.Errorf("Trace accepted incorrectly (%s:%d)", test.filename, test.line)
	}
}

func checkExpectedOutputs(outputs map[string][]vm.Uint, wm *vm.WordMachine[vm.Uint]) []error {
	var errors []error
	//
	for _, m := range wm.Modules() {
		// Check whether this is an output memory or not.
		if m, ok := m.(vm.InputOutputMemory[vm.Uint]); ok && m.IsWriteOnly() {
			if output, ok := outputs[m.Name()]; ok {
				if c := array.Compare(output, m.Contents()); c != 0 {
					errors = append(errors, fmt.Errorf("incorrect output (expected %v, actual %v)", output, m.Contents()))
				}
			}
		}
	}
	//
	return errors
}

// TestConfig provides a simple mechanism for searching for testfiles.
type TestConfig struct {
	extension string
	expected  bool
	// Indicates extension only suitable for specific field.  If nil, then
	// suitable for all fields.
	field *field.Config
}

// TESTFILE_EXTENSIONS identifies the possible file extensions used for
// different test inputs.
var TESTFILE_EXTENSIONS []TestConfig = []TestConfig{
	// should all pass
	{"accepts", true, nil},
	{"accepts.bz2", true, nil},
	{"gf_251.accepts", true, &field.GF_251},
	{"gf_8209.accepts", true, &field.GF_8209},
	{"koalabear_16.accepts", true, &field.KOALABEAR_16},
	{"bls12_377.accepts", true, &field.BLS12_377},
	// should all fail
	{"rejects", false, nil},
	{"rejects.bz2", false, nil},
	{"gf_251.rejects", false, &field.GF_251},
	{"gf_8209.rejects", false, &field.GF_8209},
	{"koalabear_16.rejects", false, &field.KOALABEAR_16},
	{"bls12_377.rejects", false, &field.BLS12_377},
}
