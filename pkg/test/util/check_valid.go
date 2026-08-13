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
	"math/rand"
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
	// DEFAULT_WORD sets the default word for fast mode execution.
	DEFAULT_WORD = vm.WORD_UINT128
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
	// enable checkpoint testing.
	checkpointing util.Option[util.Pair[string, util.Counter]]
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

// Checkpoints enables checkpoint testing with checkpoints at every n ZkC
// instructions.
func (p Config) Checkpoints(fn string, n uint64) Config {
	p.checkpointing = util.Some(util.NewPair(fn, util.NewCounter(n)))
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
	// Run checkpointing tests (if requested and in fastmode)
	if cfg.checkpointing.HasValue() && cfg.fastMode {
		for _, testcase := range tests {
			runCheckpointTests(t, p, testcase, cfg.checkpointing.Unwrap())
		}
	}
	// Run constraint tests
	if cfg.constraints {
		var field = cfg.build.GetField()
		//
		for _, test := range tests {
			// Test all configured padding strategies
			for name, strategy := range cfg.paddingStrategies {
				t.Run(name, func(t *testing.T) {
					runConstraintTest(t, p, test, field, strategy)
				})
			}
		}
	}
}

func runExecutionTests(t *testing.T, pU vm.Program[vm.Uint], tc TestCase) {
	// Run the test
	switch DEFAULT_WORD {
	case vm.WORD_UINT64:
		// Lower to fixed-width machine
		pW := vm.TransformForExecution[vm.Uint, vm.Uint64](pU)
		// Run execution test
		runExecutionTest(t, pW, tc)
	case vm.WORD_UINT128:
		// Lower to fixed-width machine
		pW := vm.TransformForExecution[vm.Uint, vm.Uint128](pU)
		// Run execution test
		runExecutionTest(t, pW, tc)
	default:
		panic(fmt.Sprintf("unknown machine word: %s", DEFAULT_WORD.Name))
	}
}

func runExecutionTest[W vm.Word[W]](t *testing.T, p vm.Program[W], test TestCase) {
	//
	var (
		err  error
		errs []error
		// decode inputs / outputs
		inputs, outputs = decodeInputsOutputs(t, p, test.data)
		// construct interpreter
		interpreter = vm.NewBytecodeInterpreter(p)
	)
	// Boot & Execute machine
	if err = interpreter.Boot("main", inputs); err == nil {
		// Execute it
		if _, err = vm.ExecuteAll(interpreter, 131072); err == nil && test.expected {
			// Check outputs match
			errs = append(errs, checkExpectedOutputs(outputs, interpreter)...)
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
		t.Errorf("[%s]%s:%d %v", DEFAULT_WORD.Name, test.filename, test.line, err)
	}
}

// runCheckpointTests exercises checkpoint/resume for a single test case.  The
// program is first run, under the fast bytecode interpreter, to generate a
// sequence of checkpoints (taken at the interval configured via
// Config.Checkpoints).  One checkpoint is then chosen at random, a fresh
// interpreter is resumed from it and run to completion, and the resulting
// behaviour is checked against what the test expected.  Checkpoint testing is
// (for now) restricted to the Uint128 word.
func runCheckpointTests(t *testing.T, pU vm.Program[vm.Uint], tc TestCase, spec util.Pair[string, util.Counter]) {
	// Run the test
	switch DEFAULT_WORD {
	case vm.WORD_UINT64:
		// Lower to fixed-width machine
		pW := vm.TransformForExecution[vm.Uint, vm.Uint64](pU)
		// Run test
		runFixedWidthCheckpointTest(t, pW, tc, spec)
	case vm.WORD_UINT128:
		// Lower to fixed-width machine
		pW := vm.TransformForExecution[vm.Uint, vm.Uint128](pU)
		// Run test
		runFixedWidthCheckpointTest(t, pW, tc, spec)
	default:
		panic(fmt.Sprintf("unknown machine word: %s", DEFAULT_WORD.Name))
	}
}

func runFixedWidthCheckpointTest[W vm.Word[W]](t *testing.T, m vm.Program[W], tc TestCase,
	spec util.Pair[string, util.Counter]) {
	//
	program, checkpoints, outputs := bootAndCheckpoint(t, m, tc, spec)
	// Nothing to resume from (the checkpointed function ran fewer than the
	// configured interval, or a reject test failed early): skip phase 2.
	if len(checkpoints) == 0 {
		return
	}
	//
	t.Logf("Generated %d checkpoints for %s", len(checkpoints), tc.filename)
	//
	var (
		idx     = rand.Intn(len(checkpoints))
		resumed = vm.NewBytecodeInterpreter(program)
		errs    []error
	)
	// Phase 2: resume a fresh interpreter from a randomly-chosen checkpoint and
	// run it to completion.  The resume runs against the *plain* program: the
	// checkpoints share its coordinates (see Program.BreakPoint), and it has no
	// breakpoint to re-trigger.
	resumed.Restore(checkpoints[idx])
	//
	_, err := vm.ExecuteAll(resumed, 131072)
	//
	if err == nil && tc.expected {
		// Resumed execution succeeded: check it produced the expected outputs.
		errs = append(errs, checkExpectedOutputs(outputs, resumed)...)
	} else if err == nil && !tc.expected {
		errs = append(errs, fmt.Errorf("test accepted incorrectly"))
	} else if tc.expected {
		// Resumed execution failed, but the test was expected to pass.
		errs = append(errs, err)
	}
	// Report any failures, noting which checkpoint was resumed from.
	for _, err := range errs {
		t.Errorf("[checkpoint:%s]%s:%d (checkpoint %d/%d) %v", DEFAULT_WORD.Name, tc.filename, tc.line,
			idx, len(checkpoints), err)
	}
}

func bootAndCheckpoint[W vm.Word[W]](t *testing.T, program vm.Program[W], tc TestCase,
	spec util.Pair[string, util.Counter]) (vm.Program[W], []vm.CheckPoint[W], map[string][]W) {
	//
	var (
		entry       vm.ProgramPoint
		checkpoints []vm.CheckPoint[W]
		fn          = spec.Left
		counter     = spec.Right
		err         error
	)
	// Locate the function whose calls are to be checkpointed.
	fid, ok := program.HasModule(fn)
	if !ok {
		t.Errorf("[%s]%s:%d unknown checkpoint function %q", DEFAULT_WORD.Name, tc.filename, tc.line, fn)
		return program, nil, nil
	}
	//
	var (
		// decode inputs/outputs
		inputs, outputs = decodeInputsOutputs(t, program, tc.data)
		// construct interpreter with a breakpoint at fn's entry
		interpreter = vm.NewBytecodeInterpreter(program.BreakPoint(fid, entry))
	)
	// Phase 1: run the program (with a breakpoint at fn's entry) to completion,
	// collecting the checkpoints it produces.  The counter governs how frequently
	// a checkpoint is actually recorded.
	gen := interpreter.
		BreakPointer(func(_ uint32) {
			if counter.Tick() {
				checkpoints = append(checkpoints, interpreter.CheckPoint())
			}
		})
	//
	if err = gen.Boot("main", inputs); err == nil {
		_, err = vm.ExecuteAll(gen, 131072)
	}
	//
	// An accepting test's generation run is the reference execution and must
	// succeed; a rejecting test, by contrast, is expected to fail at some point.
	if tc.expected && err != nil {
		t.Errorf("[%s]%s:%d checkpoint generation failed: %v", DEFAULT_WORD.Name, tc.filename, tc.line, err)
		return program, nil, outputs
	}
	//
	return program, checkpoints, outputs
}

func runConstraintTest(t *testing.T, p vm.Program[vm.Uint], test TestCase, f field.Config,
	paddingStrategy ir.PaddingStrategy) {
	// Dispatch based on field config
	switch f {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, p, test, paddingStrategy)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, p, test, paddingStrategy)
	case field.KOALABEAR_16:
		testConstraintsWithField[koalabear.Element](t, p, test, paddingStrategy)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, paddingStrategy)
		panic("BLS12_377 not currently supported for tracing")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestCase,
	paddingStrategy ir.PaddingStrategy) {
	//
	var (
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, p)
		// decode inputs / outputs
		inputs = vm.FilterInputs(p, test.data)
		// trace configuration (optionally expanding each module up to the next
		// power of two)
		traceCfg = constraints.DEFAULT_TRACE_CONFIG.WithPadding(paddingStrategy)
		// generate trace
		_, _, tr, errs = binf.Trace(inputs, traceCfg)
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
	failures := binf.Check(tr, traceCfg)
	// Fail automatically on any panic arising during constraint checking
	failIf[*schema.PanicFailure](t, failures...)
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

func checkExpectedOutputs[W vm.Word[W]](outputs map[string][]W, wm vm.Core[W]) []error {
	var errors []error
	//
	for iter := wm.Outputs(); iter.HasNext(); {
		var (
			mem  = iter.Next()
			name = mem.Descriptor().Name()
		)
		//
		if output, ok := outputs[name]; ok {
			// Compare canonical byte encodings rather than the raw cell arrays:
			// a memory whose live cell-count is odd encodes the same bytes as an
			// expected value that (being byte-granular input) carries a trailing
			// padding cell, so a length-sensitive array compare would spuriously
			// fail.  This mirrors compareGogenOutputs.
			expected := vm.EncodeBytes(output, *mem.Descriptor())
			actual := vm.EncodeBytes(mem.Contents(), *mem.Descriptor())
			//
			if !bytes.Equal(expected, actual) {
				errors = append(errors, fmt.Errorf("incorrect output (expected 0x%s, actual 0x%s)",
					hex.EncodeToString(expected), hex.EncodeToString(actual)))
			}
		}
	}
	//
	return errors
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
