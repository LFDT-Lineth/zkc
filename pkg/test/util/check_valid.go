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
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

var (
	// MIN_SAMPLE_SIZE determines the least number of test cases for which
	// sampling is permitted.  Essentially, if sampling is enabled by there are
	// less than 10 test vectors, then it automatically causes a test failure.
	MIN_SAMPLE_SIZE = 10
	// DEFAULT_PADDING sets the default strategy to use
	DEFAULT_PADDING = util.NewPair("next-power-of-two-padding", ir.NextPowerOfTwoPadding)
	// ALL_FIELDS defines the set of all known fields for testing
	ALL_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_24, field.KOALABEAR_16, field.GF_8209, field.GF_251}
	// DEFAULT_FIELDS set default fields for testing
	DEFAULT_FIELDS = []field.Config{field.KOALABEAR_16, field.GF_8209}
	// DEFAULT_CONFIG sets a default testing configuration
	DEFAULT_CONFIG = TestConfig{
		fields:            DEFAULT_FIELDS,
		constraints:       true,
		gogen:             true,
		verbose:           false,
		sampling:          util.None[float64](),
		maxStaticHeights:  []uint{codegen.DEFAULT_MAX_STATIC_HEIGHT},
		paddingStrategies: []util.Pair[string, ir.PaddingStrategy]{DEFAULT_PADDING},
	}
)

// CheckValid checks that a given source file compiles without any errors.
// nolint
func CheckValid(t *testing.T, zkcfile string, config TestConfig) {
	// Run in parallel with the other files under test.  Test generation
	// compiles the program (once per field / height) and builds the gogen
	// binary, none of which is parallelised internally.  Without this, that
	// work is serialised across every file in the suite.
	t.Parallel()
	//
	var (
		// generate actual tests for the given file.
		testcases = config.GenerateTests(t, zkcfile)
	)
	// Check for each field requested
	for _, test := range testcases {
		t.Run(fmt.Sprintf("%s", test.Handle()), func(t *testing.T) {
			t.Parallel()
			//
			checkValidInternal(t, test)
		})
	}
}

func checkValidInternal(t *testing.T, test TestCase) {
	var p = test.program
	// Apply marshling (if requested)
	if test.marshal {
		p = marshallUnmarshallMachine(p, test.build.GetField())
	}
	// Run (fast mode) execution test
	if test.fastMode {
		runExecutionTests(t, p, test.vector)
	}
	// Run gogen (if requested)
	if test.gogen.HasValue() {
		runGogenExecutionTest(t, p, test.gogen.Unwrap(), test.vector)
	}
	// Run constraint check (if requsted)
	if test.constraints {
		var (
			field = test.build.GetField()
			// Sequential tracing
			traceConfig = vm.DEFAULT_TRACE_CONFIG.WithPadding(test.padding.Right)
		)
		// Apply sharding (if applicable)
		if test.sharding.HasValue() {
			traceConfig = traceConfig.WithSharding(test.sharding.Unwrap())
		}
		// Run the test
		runConstraintTest(t, p, test.vector, field, traceConfig)
	}
}

func runExecutionTests(t *testing.T, p vm.Program[vm.Uint], test TestVector) {
	// Dispatch based on field config
	switch p.Field() {
	case field.GF_251:
		runExecutionTest[gf251.Element](t, p, test)
	case field.GF_8209:
		runExecutionTest[gf8209.Element](t, p, test)
	case field.KOALABEAR_16, field.KOALABEAR_24:
		runExecutionTest[koalabear.Element](t, p, test)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, paddingStrategy)
		panic("BLS12_377 not currently supported for execution")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", p.Field().Name))
	}
}

func runExecutionTest[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestVector) {
	//
	var (
		// decode inputs / outputs
		inputs, _ = vm.FilterInputs(p, test.data)
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, p)
	)
	//
	if actuals, errs := binf.Execute(inputs); len(errs) == 0 {
		// Check outputs line up
		for name, actual := range actuals {
			if expected, ok := test.data[name]; ok {
				//
				if !bytes.Equal(expected, actual) {
					t.Errorf("test (%s:%d) has incorrect output (expected 0x%s, actual 0x%s)",
						test.filename, test.line, hex.EncodeToString(expected), hex.EncodeToString(actual))
				}
			}
		}
		// Sanity check enough outputs
		if uint(len(actuals)) != p.Outputs().Count() {
			t.Errorf("test (%s:%d) has incorrect output (expected %d outputs, got %d)",
				test.filename, test.line, p.Outputs().Count(), len(actuals))
		}
	} else {
		// Fail automatically on any panic arising during execution
		failIf[*schema.PanicFailure[F]](t, errs...)
		// Determine whether test accepted or not.
		accepted := len(errs) == 0
		// Process what happened versus what was supposed to happen.
		if !accepted && test.expected {
			t.Errorf("test incorrectly (%s:%d): %s", test.filename, test.line, errs)
		} else if accepted && !test.expected {
			//printTrace(tr)
			t.Errorf("test incorrectly (%s:%d)", test.filename, test.line)
		}
	}
}

func runConstraintTest(t *testing.T, p vm.Program[vm.Uint], test TestVector, f field.Config, traceCfg vm.TraceConfig) {
	// Dispatch based on field config
	switch f {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, p, test, traceCfg)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, p, test, traceCfg)
	case field.KOALABEAR_16, field.KOALABEAR_24:
		testConstraintsWithField[koalabear.Element](t, p, test, traceCfg)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, paddingStrategy)
		panic("BLS12_377 not currently supported for tracing")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestVector,
	traceCfg vm.TraceConfig) {
	//
	var (
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, p)
		// decode inputs / outputs
		inputs, _ = vm.FilterInputs(p, test.data)
		// generate trace
		_, tr, errs = binf.Trace(inputs, traceCfg)
	)
	// Fail automatically on any internal error arising during tracing
	failIfNot[*vm.Failure](t, errs...)
	// Check for errors
	if test.expected {
		// Fail on any machine failure, since this test was not expected to
		// generate any failures.
		failIf[*vm.Failure](t, errs...)
	}
	// Check constraints
	failures := binf.Check(traceCfg, tr)
	// Fail automatically on any panic arising during constraint checking
	failIf[*schema.PanicFailure[F]](t, failures...)
	// Determine whether trace accepted or not.
	accepted := len(failures) == 0
	// Process what happened versus what was supposed to happen.
	if !accepted && test.expected {
		t.Errorf("Trace rejected incorrectly (%s:%d): %s", test.filename, test.line, failures)
	} else if accepted && !test.expected {
		//printTrace(tr)
		t.Errorf("Trace accepted incorrectly (%s:%d)", test.filename, test.line)
	}
}
