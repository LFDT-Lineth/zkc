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
	"errors"
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
	// ALL_FIELDS defines the set of all known fields for testing
	ALL_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_16, field.GF_8209}
	// DEFAULT_FIELDS set default fields for testing
	DEFAULT_FIELDS = []field.Config{field.KOALABEAR_16}
	// DEFAULT_CONFIG sets a default testing configuration
	DEFAULT_CONFIG = Config{
		fields:           DEFAULT_FIELDS,
		constraints:      true,
		gogen:            true,
		verbose:          false,
		sampling:         util.None[float64](),
		maxStaticHeights: []uint{codegen.DEFAULT_MAX_STATIC_HEIGHT},
		paddingStrategies: map[string]ir.PaddingStrategy{
			"next-power-of-two-padding": ir.NextPowerOfTwoPadding,
		}}
)

// Config for testing
type Config struct {
	// Fields to test over
	fields []field.Config
	// enable constraints checking, or not.
	constraints bool
	// enable the generated-Go ("native") executor
	gogen bool
	// enable verbose mode.
	verbose bool
	// optional sampling percentage
	sampling util.Option[float64]
	// determines how much front padding is added to the generated trace.
	paddingStrategies map[string]ir.PaddingStrategy
	// maxStaticHeights controls the maximum heights (i.e. number of rows) of static
	// range tables.  Widths whose enumeration would exceed this are range-checked
	// recursively instead.  Defaults to codegen.DEFAULT_MAX_STATIC_HEIGHT.
	maxStaticHeights []uint
	// enable sharding.
	sharding util.Option[vm.ShardingStrategy]
}

// MaxStaticHeights sets the maximum heights number of rows) of static range
// tables to test with.
func (p Config) MaxStaticHeights(heights ...uint) Config {
	p.maxStaticHeights = heights
	//
	return p
}

// GoGen determines whether or not to additionally run the generated-Go ("native")
// executor and check its outputs against the test.  This is experimental and only
// applies to the Uint64 word; programs the generator cannot yet handle are skipped
// rather than failed (see runGogenExecutionTest).
func (p Config) GoGen(flag bool) Config {
	p.gogen = flag
	//
	return p
}

// Sharding enables trace sharding with checkpoints at every n ZkC instructions.
func (p Config) Sharding(fn string, n uint64) Config {
	p.sharding = util.Some(vm.NewShardingStrategy(fn, n))
	//
	return p
}

// Constraints determines whether or not to check constraints.
func (p Config) Constraints(flag bool) Config {
	p.constraints = flag
	//
	return p
}

// Sampling sets the sampling ratio to use for the given constraint set.
func (p Config) Sampling(ratio float64) Config {
	p.sampling = util.Some(ratio)
	//
	return p
}

// Verbose determines whether or not printf statements and calls to #[debug]
// functions are retained during code generation.
func (p Config) Verbose(flag bool) Config {
	p.verbose = flag
	//
	return p
}

// Padding determines how much front padding is added to the generated trace.
func (p Config) Padding(strategies map[string]ir.PaddingStrategy) Config {
	p.paddingStrategies = strategies
	//
	return p
}

// instantiate this configuration for (fast mode) execution.  Constraints are
// never checked in fast mode, since the fast-mode pipeline does not generate
// them.
func (p Config) forExecution(build codegen.Config) testConfig {
	return testConfig{p.Constraints(false), build, true}
}

// instantiate this configuration for tracing.
func (p Config) forTracing(build codegen.Config) testConfig {
	return testConfig{p, build, false}
}

type testConfig struct {
	Config
	// build config
	build codegen.Config
	//
	fastMode bool
}

// CheckValid checks that a given source file compiles without any errors.
// nolint
func CheckValid(t *testing.T, test, ext string, config Config) {
	var (
		// Parse all JSON tests
		testcases = readTestCases(t, test, config.sampling)
	)
	// Check for each field requested
	for _, f := range config.fields {
		var (
			testfile = fmt.Sprintf("%s.%s", test, ext)
			// Setup default config
			cfg = codegen.DEFAULT_CONFIG.
				Verbose(config.verbose).Field(f)
		)
		// Only run fast mode tests for the default height / padding, since
		// neither static height nor padding impacts on fast mode.
		t.Run(fmt.Sprintf("%s/fastmode", f.Name), func(t *testing.T) {
			t.Parallel()
			//
			checkValidInternal(t, testfile, config.forExecution(cfg), testcases[f])
		})
		// Run tracing tests across differing static heights to ensure resiliance
		// against changing the default height.
		for _, height := range config.maxStaticHeights {
			//
			t.Run(fmt.Sprintf("%s/height=%d", f.Name, height), func(t *testing.T) {
				t.Parallel()
				// Run all tests in tracing mode
				checkValidInternal(t, testfile, config.forTracing(cfg.MaxStaticHeight(height)), testcases[f])
			})
		}
	}
}

func checkValidInternal(t *testing.T, testfile string, cfg testConfig, testcases []TestCase) {
	var (
		// Compile test program
		p1 = compileTestProgram(t, testfile, cfg.build)
		p2 = marshallUnmarshallMachine(p1, cfg.build.GetField())
	)
	// check for original machine
	checkValidMachine(t, p1, cfg, testcases)
	// check for marshalled / unmarshalled machine
	checkValidMachine(t, p2, cfg, testcases)
	// check gogen binaries (if requested)
	if cfg.gogen {
		checkValidGoGen(t, testfile, testcases, p1)
	}
}

func checkValidGoGen(t *testing.T, testfile string, tests []TestCase, pU vm.Program[vm.Uint]) {
	// Lower through the execution pipeline before handing to gogen.  GenerateGo
	// requires Program[Uint] with registers no wider than 64 bits, so we split
	// against a bounded word whilst staying in the Uint representation.
	var p = vm.TransformForExecutionRaw[vm.Uint, vm.Uint](pU, vm.WORD_UINT128)
	//
	var binary, err = buildGogenProgram(t, p)
	//
	if errors.Is(err, errGoGenUnsupported) {
		// gogen cannot represent this program --- fail.
		t.Errorf("[gogen] %s: %v", testfile, err)
	} else if err != nil {
		t.Errorf("[gogen] %v", err)
	} else {
		// Log binary location
		t.Logf("[gogen] compiled %s into binary at %s", testfile, binary)
		// Run each test vector
		for _, testcase := range tests {
			runGogenExecutionTest(t, p, binary, testcase)
		}
	}
}

func checkValidMachine(t *testing.T, p vm.Program[vm.Uint], cfg testConfig, tests []TestCase) {
	// Run execution tests
	for _, testcase := range tests {
		runExecutionTests(t, p, testcase)
	}
	// Run constraint tests
	if cfg.constraints {
		var field = cfg.build.GetField()
		// Sequential tracing
		for _, test := range tests {
			// Test all configured padding strategies
			for name, strategy := range cfg.paddingStrategies {
				traceCfg := vm.DEFAULT_TRACE_CONFIG.WithPadding(strategy)
				//
				t.Run(name, func(t *testing.T) {
					runConstraintTest(t, p, test, field, traceCfg)
				})
			}
			// Parallel tracing (if requested)
			if cfg.sharding.HasValue() {
				traceCfg := vm.DEFAULT_TRACE_CONFIG.WithSharding(cfg.sharding.Unwrap())
				//
				t.Run("sharded-tracing", func(t *testing.T) {
					runConstraintTest(t, p, test, field, traceCfg)
				})
			}
		}
	}
}

func runExecutionTests(t *testing.T, p vm.Program[vm.Uint], test TestCase) {
	// Dispatch based on field config
	switch p.Field() {
	case field.GF_251:
		runExecutionTest[gf251.Element](t, p, test)
	case field.GF_8209:
		runExecutionTest[gf8209.Element](t, p, test)
	case field.KOALABEAR_16:
		runExecutionTest[koalabear.Element](t, p, test)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, paddingStrategy)
		panic("BLS12_377 not currently supported for execution")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", p.Field().Name))
	}
}

func runExecutionTest[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestCase) {
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

func runConstraintTest(t *testing.T, p vm.Program[vm.Uint], test TestCase, f field.Config, traceCfg vm.TraceConfig) {
	// Dispatch based on field config
	switch f {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, p, test, traceCfg)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, p, test, traceCfg)
	case field.KOALABEAR_16:
		testConstraintsWithField[koalabear.Element](t, p, test, traceCfg)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, paddingStrategy)
		panic("BLS12_377 not currently supported for tracing")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestCase,
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

func readTestCases(t *testing.T, test string, sampling util.Option[float64]) map[field.Config][]TestCase {
	var tests = make(map[field.Config][]TestCase)
	// Search for tests
	for _, cfg := range TESTFILE_EXTENSIONS {
		var fields []field.Config
		// Read tests from file
		tcs, ok := ReadTestsFile(t, cfg, test)
		// Apply sampling only if the file existed, as otherwise this would trip
		// up the built-in protections we have.
		if ok {
			tcs = applySampling(t, test, tcs, sampling)
		}
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
			tests[f] = append(tests[f], tcs...)
		}
	}
	//
	return tests
}

func applySampling(t *testing.T, test string, tc []TestCase, sampling util.Option[float64]) []TestCase {
	if sampling.IsEmpty() {
		// no sampling
		return tc
	} else if len(tc) < MIN_SAMPLE_SIZE {
		t.Errorf("%s: insufficient test vectors for sampling (have %d < %d)", test, len(tc), MIN_SAMPLE_SIZE)
	} else if n := uint(float64(len(tc)) * sampling.Unwrap()); n == 0 {
		t.Errorf("%s: sampling would elimimate all test vectors! (had %d)", test, len(tc))
	} else {
		// sample test cases accordingly
		return util.SampleElements(n, tc)
	}
	//
	return tc
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
