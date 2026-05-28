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
	// ALL_FIELDS defines the set of all known fields for testing
	ALL_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_16, field.GF_8209}
	// DEFAULT_FIELDS set default fields for testing
	DEFAULT_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_16}
	// DEFAULT_WORDS set default words for testing
	DEFAULT_WORDS = []vm.WordConfig{vm.WORD_UINT}
	// DEFAULT_CONFIG sets a default testing configuration
	DEFAULT_CONFIG = Config{
		fields:      DEFAULT_FIELDS,
		words:       DEFAULT_WORDS,
		constraints: false,
		splitting:   false}
)

// Config for testing
type Config struct {
	// Fields to test over
	fields []field.Config
	// Words to test over
	words []vm.WordConfig
	// enable constraints checking, or not.
	constraints bool
	// enable register splitting
	splitting bool
}

// Fields determines which fields to test over.
func (p Config) Fields(fields ...field.Config) Config {
	p.fields = fields
	//
	return p
}

// Words determines which words to test over.
func (p Config) Words(words ...vm.WordConfig) Config {
	p.words = words
	//
	return p
}

// Constraints determines whether or not to check constraints.
func (p Config) Constraints(flag bool) Config {
	p.constraints = flag
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
	var (
		// Parse all JSON tests
		testcases = readTestCases(t, test)
	)
	// Enable testing each trace in parallel
	t.Parallel()
	// Check for each field requested
	for _, f := range config.fields {
		var (
			testfile = fmt.Sprintf("%s.%s", test, ext)
			// Setup default config
			cfg = codegen.DEFAULT_CONFIG.SplitRegisters(config.splitting).Field(f)
		)
		// Run all tests without lowering (and preventing the constraints check)
		checkValidInternal(t, testfile, cfg.LowerNatives(false), config.Constraints(false), testcases[f])
		// Run all tests with lowering
		checkValidInternal(t, testfile, cfg.LowerNatives(true), config, testcases[f])
	}
}

func checkValidInternal(t *testing.T, testfile string, cfg codegen.Config, config Config, testcases []TestCase) {
	var (
		// Compile test program
		m1 = compileTestProgram(t, testfile, cfg)
		m2 = marshallUnmarshallMachine(m1, cfg.GetField())
	)
	// check for original machine
	checkValidMachine(t, m1, cfg, config, testcases)
	// check for marshalled / unmarshalled machine
	checkValidMachine(t, m2, cfg, config, testcases)
}

func checkValidMachine(t *testing.T, m *vm.WordMachine[vm.Uint], cfg codegen.Config, config Config, tests []TestCase) {
	// Run execution tests
	for _, testcase := range tests {
		runExecutionTests(t, m, testcase, cfg.GetField(), config.words)
	}
	// Run constraint tests
	if config.constraints {
		for _, test := range tests {
			// FIXME: support reject tests
			if test.expected {
				runConstraintTest(t, m, test, cfg)
			}
		}
	}
}

func runExecutionTests(t *testing.T, m *vm.WordMachine[vm.Uint], tc TestCase, f field.Config, words []vm.WordConfig) {
	for _, w := range words {
		// Check for incompatible field/word combinations.  For example, we
		// cannot emulate a 254bit field using a 64bit word.
		if w.Bandwidth <= f.BandWidth {
			continue
		}
		// Run the test
		switch w {
		case vm.WORD_UINT:
			runExecutionTest(t, m, tc, w)
		case vm.WORD_UINT64:
			// Lower to 64bit machine
			m64 := vm.WordToWordMachine[vm.Uint, vm.Uint64](m)
			// Run execution test
			runExecutionTest(t, m64, tc, w)
		default:
			panic(fmt.Sprintf("unknown machine word: %s", w.Name))
		}
	}
}

func runExecutionTest[W vm.Word[W]](t *testing.T, wm vm.Machine[W], test TestCase, cfg vm.WordConfig) {
	var (
		err  error
		errs []error
		// decode inputs / outputs
		inputs, outputs = decodeInputsOutputs(t, wm, test.data)
	)
	// Execute machine
	if err = wm.Boot("main", inputs); err == nil {
		// Execute it
		if _, err = vm.ExecuteAll(wm, 1024); err == nil && test.expected {
			// Check outputs match
			errs = append(errs, checkExpectedOutputs(outputs, wm)...)
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
		t.Errorf("[%s]%s:%d %v", cfg.Name, test.filename, test.line, err)
	}
}

func runConstraintTest(t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase, cfg codegen.Config) {
	var f = cfg.GetField()
	// Dispatch based on field config
	switch f {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, wm, test, f)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, wm, test, f)
	case field.KOALABEAR_16:
		testConstraintsWithField[koalabear.Element](t, wm, test, f)
	case field.BLS12_377:
		testConstraintsWithField[bls12_377.Element](t, wm, test, f)
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase,
	f field.Config) {
	//
	var (
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, f, *wm)
		// decode inputs / outputs
		inputs, _ = decodeInputsOutputs(t, wm, test.data)
		// generate trace
		tr, errs = constraints.Trace(binf, inputs, constraints.DEFAULT_TRACE_CONFIG)
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

func checkExpectedOutputs[W vm.Word[W]](outputs map[string][]W, wm vm.Machine[W]) []error {
	var errors []error
	//
	for _, m := range wm.Modules() {
		// Check whether this is an output memory or not.
		if m, ok := m.(vm.InputOutputMemory[W]); ok && m.IsWriteOnly() {
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

func readTestCases(t *testing.T, test string) map[field.Config][]TestCase {
	var tests = make(map[field.Config][]TestCase)
	// Search for tests
	for _, cfg := range TESTFILE_EXTENSIONS {
		var fields []field.Config
		// Read tests from file
		tc := ReadTestsFile(t, cfg, test)
		//
		if cfg.field == nil {
			// all fields supported
			fields = ALL_FIELDS
		} else {
			// only specific field supported
			fields = []field.Config{*cfg.field}
		}
		// associate tests with appropriate fields
		for _, f := range fields {
			tests[f] = append(tests[f], tc...)
		}
	}
	//
	return tests
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
