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
	// ALL_FIELDS defines the set of all known fields for testing
	ALL_FIELDS = []field.Config{field.BLS12_377, field.KOALABEAR_16, field.GF_8209}
	// DEFAULT_WORD sets the default word for fast mode execution.
	DEFAULT_WORD = vm.WORD_UINT128
	// DEFAULT_FIELDS set default fields for testing
	DEFAULT_FIELDS = []field.Config{field.KOALABEAR_16}
	// DEFAULT_CONFIG sets a default testing configuration
	DEFAULT_CONFIG = Config{
		fields:            DEFAULT_FIELDS,
		constraints:       true,
		splitting:         true,
		fastModeSplitting: true,
		gogen:             true,
		quiet:             false,
		maxStaticDepths:   []uint{codegen.DEFAULT_MAX_STATIC_DEPTH},
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
	// enable register splitting
	splitting bool
	// enable register splitting in fast mode
	fastModeSplitting bool
	// enable the generated-Go ("native") executor
	gogen bool
	// enable quiet mode, which elides printf statements and calls to #[debug]
	// functions during code generation.
	quiet bool
	// determines how much front padding is added to the generated trace.
	paddingStrategies map[string]ir.PaddingStrategy
	// maxStaticDepths controls the maximum depth (i.e. number of rows) of static
	// range tables.  Widths whose enumeration would exceed this are range-checked
	// recursively instead.  Defaults to codegen.DEFAULT_MAX_STATIC_DEPTH.
	maxStaticDepths []uint
	// enable checkpoint testing.
	checkpointing util.Option[util.Pair[string, util.Counter]]
}

// MaxStaticDepths sets the maximum depths (i.e. number of rows) of static range
// tables to test with.
func (p Config) MaxStaticDepths(depths ...uint) Config {
	p.maxStaticDepths = depths
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

// Splitting determines whether or not to apply register splitting when tracing.
func (p Config) Splitting(flag bool) Config {
	p.splitting = flag
	//
	return p
}

// FastModeSplitting determines whether or not to apply register splitting in
// fast mode.
func (p Config) FastModeSplitting(flag bool) Config {
	p.fastModeSplitting = flag
	//
	return p
}

// Quiet determines whether or not printf statements and calls to #[debug]
// functions are elided during code generation.
func (p Config) Quiet(flag bool) Config {
	p.quiet = flag
	//
	return p
}

// Padding determines how much front padding is added to the generated trace.
func (p Config) Padding(strategies map[string]ir.PaddingStrategy) Config {
	p.paddingStrategies = strategies
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
	// Check for each field requested
	for _, f := range config.fields {
		var (
			testfile = fmt.Sprintf("%s.%s", test, ext)
			// Setup default config
			cfg = codegen.DEFAULT_CONFIG.
				SplitRegisters(config.splitting).
				Quiet(config.quiet).Field(f).
				Word(DEFAULT_WORD)
		)
		// Only run fast mode tests for the default depth / padding, since
		// neither static depth nor padding impacts on fast mode.
		t.Run(fmt.Sprintf("%s/fastmode", f.Name), func(t *testing.T) {
			t.Parallel()
			//
			checkValidInternal(t, testfile, cfg.FastMode(true).SplitRegisters(config.fastModeSplitting),
				config.Constraints(false), testcases[f])
		})
		// Run tracing tests across differing static depths to ensure resiliance
		// against changing the default depth.
		for _, depth := range config.maxStaticDepths {
			//
			t.Run(fmt.Sprintf("%s/depth=%d", f.Name, depth), func(t *testing.T) {
				t.Parallel()
				// Run all tests in tracing mode
				checkValidInternal(t, testfile, cfg.MaxStaticDepth(depth).FastMode(false), config, testcases[f])
			})
		}
	}
}

func checkValidInternal(t *testing.T, testfile string, cfg codegen.Config, config Config, testcases []TestCase) {
	var (
		// Compile test program
		p1 = compileTestProgram(t, testfile, cfg)
		p2 = marshallUnmarshallMachine(p1, cfg.GetField())
	)
	// check for original machine
	checkValidMachine(t, p1, cfg, config, testcases)
	// check for marshalled / unmarshalled machine
	checkValidMachine(t, p2, cfg, config, testcases)
	// check gogen binaries (if requested)
	if config.gogen {
		checkValidGoGen(t, testfile, testcases, p1)
	}
}

func checkValidGoGen(t *testing.T, testfile string, tests []TestCase, p vm.Program[vm.Uint]) {
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

func checkValidMachine(t *testing.T, p vm.Program[vm.Uint], cfg codegen.Config, config Config, tests []TestCase) {
	// Run execution tests
	for _, testcase := range tests {
		runExecutionTests(t, p, testcase)
	}
	// Run checkpointing tests (if requested and in fastmode)
	if config.checkpointing.HasValue() && cfg.IsFastMode() {
		for _, testcase := range tests {
			runCheckpointTests(t, p, testcase, config.checkpointing.Unwrap())
		}
	}
	// Run constraint tests
	if config.constraints {
		for _, test := range tests {
			// FIXME: support reject tests
			if test.expected {
				// Test all configure padding strategies
				for name, strategy := range config.paddingStrategies {
					t.Run(name, func(t *testing.T) {
						runConstraintTest(t, p, test, cfg, strategy)
					})
				}
			}
		}
	}
}

func runExecutionTests(t *testing.T, m vm.Program[vm.Uint], tc TestCase) {
	// Run the test
	switch DEFAULT_WORD {
	case vm.WORD_UINT64:
		runFixedWidthExecutionTest[vm.Uint64](t, m, tc)
	case vm.WORD_UINT128:
		runFixedWidthExecutionTest[vm.Uint128](t, m, tc)
	default:
		panic(fmt.Sprintf("unknown machine word: %s", DEFAULT_WORD.Name))
	}
}

func runFixedWidthExecutionTest[W vm.Word[W]](t *testing.T, pU vm.Program[vm.Uint], tc TestCase) {
	// Lower to fixed-width machine
	pW := vm.ProgramToProgram[vm.Uint, W](pU)
	// Run execution test
	runExecutionTest(t, pW, tc)
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
		// Lower machine to use 64bit words
		pW := vm.ProgramToProgram[vm.Uint, vm.Uint64](pU)
		// Run test
		runFixedWidthCheckpointTest(t, pW, tc, spec)
	case vm.WORD_UINT128:
		// Lower machine to use 128bit words
		pW := vm.ProgramToProgram[vm.Uint, vm.Uint128](pU)
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

func runConstraintTest(t *testing.T, p vm.Program[vm.Uint], test TestCase, cfg codegen.Config,
	paddingStrategy ir.PaddingStrategy) {
	var f = cfg.GetField()
	// Dispatch based on field config
	switch f {
	case field.GF_251:
		testConstraintsWithField[gf251.Element](t, p, test, f, cfg.GetMaxStaticDepth(), paddingStrategy)
	case field.GF_8209:
		testConstraintsWithField[gf8209.Element](t, p, test, f, cfg.GetMaxStaticDepth(), paddingStrategy)
	case field.KOALABEAR_16:
		testConstraintsWithField[koalabear.Element](t, p, test, f, cfg.GetMaxStaticDepth(), paddingStrategy)
	case field.BLS12_377:
		//testConstraintsWithField[bls12_377.Element](t, p, test, f, cfg.GetMaxStaticDepth())
		panic("BLS12_377 not currently supported for tracing")
	default:
		panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
	}
}

func testConstraintsWithField[F field.Element[F]](t *testing.T, p vm.Program[vm.Uint], test TestCase,
	f field.Config, maxStaticDepth uint, paddingStrategy ir.PaddingStrategy) {
	//
	var (
		// construct binary file
		binf = constraints.NewBinaryFile[F](nil, nil, f, maxStaticDepth, p)
		// decode inputs / outputs
		inputs = vm.FilterInputs(p, test.data)
		// trace configuration (optionally expanding each module up to the next
		// power of two)
		traceCfg = constraints.DEFAULT_TRACE_CONFIG.WithPadding(paddingStrategy)
		// generate trace
		_, _, tr, errs = binf.Trace(inputs, traceCfg)
	)
	//
	if test.expected {
		// test expected to pass, but tracing generated failures.
		failIfErrors(t, errs...)
	}
	//
	failures := binf.Check(tr, traceCfg)
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
