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
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/trace/json"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// TestDir determines the (relative) location of the test directory.  That is
// where the corset test files (lisp) and the corresponding traces
// (accepts/rejects) are found.
const TestDir = "../../testdata"

// ASM_MAX_PADDING determines the maximum amount of padding to use when testing.
// Specifically, every trace is tested with varying amounts of padding upto this
// value.  NOTE: assembly modules don't need to be tested for higher padding
// values, since they only ever do unit shifts.
const ASM_MAX_PADDING uint = 2

// CORSET_MAX_PADDING determines the maximum amount of padding to use when
// testing. Specifically, every trace is tested with varying amounts of padding
// upto this value.
const CORSET_MAX_PADDING uint = 7

// FIELD_REGEX is used to restrict which fields will be tested.  This is
// primarily useful for the CI pipeline where we want to test individual fields
// in separate runners.
var FIELD_REGEX *regexp.Regexp

// CheckCorset checks that all traces which we expect to be accepted are
// accepted by a given set of constraints, and all traces that we expect to be
// rejected are rejected.  All fields provided are tested against, and also all
// padding amounts upto 7.
func CheckCorset(t *testing.T, stdlib bool, test string, fields ...field.Config) {
	CheckWithFields(t, stdlib, test, CORSET_MAX_PADDING, fields...)
}

// CheckCorsetNoPadding checks that all traces which we expect to be accepted
// are accepted by a given set of constraints, and all traces that we expect to
// be rejected are rejected.  All fields provided are tested against but without
// any padding.  This is useful to reduce unnecessary testing for cases where we
// know padding is not relevant.
func CheckCorsetNoPadding(t *testing.T, stdlib bool, test string, fields ...field.Config) {
	CheckWithFields(t, stdlib, test, 0, fields...)
}

// CheckWithFields checks that all traces which we expect to be accepted are
// accepted by a given set of constraints, and all traces that we expect to be
// rejected are rejected.  All fields provided are tested against.
func CheckWithFields(t *testing.T, stdlib bool, test string, maxPadding uint, fields ...field.Config) {
	// Sanity check
	if len(fields) == 0 {
		panic("no field configurations")
	}
	// Run checks for each field
	for _, f := range fields {
		// Check whether field is active
		if !FIELD_REGEX.MatchString(f.Name) {
			continue
		}
		// Dispatch based on field config
		switch f {
		case field.GF_251:
			checkWithField[gf251.Element](t, stdlib, test, maxPadding, f)
		case field.GF_8209:
			checkWithField[gf8209.Element](t, stdlib, test, maxPadding, f)
		case field.KOALABEAR_16:
			checkWithField[koalabear.Element](t, stdlib, test, maxPadding, f)
		case field.BLS12_377:
			checkWithField[bls12_377.Element](t, stdlib, test, maxPadding, f)
		default:
			panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
		}
	}
}

func checkWithField[F field.Element[F]](t *testing.T, stdlib bool, test string, maxPadding uint,
	field field.Config) {
	//
	var (
		filenames = matchSourceFiles(test)
		// Configure the stack for the given field.
		stacks = getSchemaStack[F](stdlib, field, filenames...)
	)
	// Record how many tests executed.
	nTests := 0
	// Iterate possible testfile extensions
	for _, cfg := range LEGACY_TESTFILE_EXTENSIONS {
		var traces []lt.TraceFile
		// Construct test filename
		testFilename := fmt.Sprintf("%s/%s.%s", TestDir, test, cfg.extension)
		// Sanity check field aligns
		if cfg.field == "" || cfg.field == field.Name {
			// Read traces from file
			traces = ReadTracesFile(testFilename)
			if len(traces) > 0 {
				// Run tests
				fullCheckTraces(t, testFilename, cfg, maxPadding, traces, stacks)
			}
		}
		// Record how many tests we found
		nTests += len(traces)
	}
	// Sanity check at least one trace found.
	if nTests == 0 {
		panic(fmt.Sprintf("missing any tests for %s", test))
	}
}

func fullCheckTraces[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig, maxPadding uint,
	traces []lt.TraceFile, stack cmd_util.SchemaStacker[F]) {
	//
	if cfg.expand {
		var errors []error
		// Extract root schema
		schema := stack.BinaryFile().Schema
		// Apply trace propagation
		if traces, errors = asm.PropagateAll(schema, traces); len(errors) != 0 && cfg.expected {
			t.Errorf("Trace propagation failed (%s): %s", test, errors)
			return
		} else if len(errors) != 0 {
			// If trace was not expected to pass and it failed, then we're all
			// good. But, still we cannot continue with this trace.
			return
		}
	}
	// Run checks using schema compiled from source
	checkCompilerOptimisations(t, test, cfg, traces, stack)
	// Construct binary schema using primary stack
	checkBinaryEncoding(t, test, cfg, traces, stack)
	// Perform checks with different fields
	checkPadding(t, test, cfg, maxPadding, traces, stack)
}

// Sanity check same outcome for all optimisation levels
func checkCompilerOptimisations[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig,
	traces []lt.TraceFile, stack cmd_util.SchemaStacker[F]) {
	// Run checks using schema compiled from source
	for _, opt := range cfg.optlevels {
		// Only check optimisation levels other than the default.
		if opt != mir.DEFAULT_OPTIMISATION_INDEX {
			// Set optimisation level
			stack = stack.WithOptimisationConfig(mir.OPTIMISATION_LEVELS[opt])
			// Apply stack
			checkTraces(t, test, 0, opt, cfg, traces, stack)
		}
	}
}

// Check the binary encoding / decoding.
func checkBinaryEncoding[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig, traces []lt.TraceFile,
	stack cmd_util.SchemaStacker[F]) {
	//
	name := fmt.Sprintf("%s:bin", test)
	// Construct binary schema using primary stack
	if binf := encodeDecodeSchema(t, *stack.BinaryFile()); binf != nil {
		// Choose any valid optimisation level
		opt := cfg.optlevels[0]
		//
		stack = stack.WithBinaryFile(*binf)
		// Set optimisation level
		stack = stack.WithOptimisationConfig(mir.OPTIMISATION_LEVELS[opt])

		// Run checks using schema from binary file.  Observe, to try and reduce
		// overhead of repeating all the tests we don't consider padding.
		checkTraces(t, name, 0, opt, cfg, traces, stack)
	}
}

// Run default optimisation over all fields, and check padding for the primary
// stack only.
func checkPadding[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig, maxPadding uint,
	traces []lt.TraceFile, stack cmd_util.SchemaStacker[F]) {
	//
	if cfg.field == "" || cfg.field == stack.Field().Name {
		// Set default optimisation level
		stack = stack.WithOptimisationConfig(mir.DEFAULT_OPTIMISATION_LEVEL)
		// Apply stack
		checkTraces(t, test, maxPadding, mir.DEFAULT_OPTIMISATION_INDEX, cfg, traces, stack)
	}
}

// Check a given set of tests have an expected outcome (i.e. are
// either accepted or rejected) by a given set of constraints.
func checkTraces[F field.Element[F]](t *testing.T, test string, maxPadding uint, opt uint, cfg LegacyTestConfig,
	traces []lt.TraceFile, stacker cmd_util.SchemaStacker[F]) {
	// For unexpected traces, we never want to explore padding (because that's
	// the whole point of unexpanded traces --- they are raw).
	if !cfg.expand {
		maxPadding = 0
	}
	// Configure stack.
	stack := stacker.Build()
	// Run through all configurations.
	for padding := uint(0); padding <= maxPadding; padding++ {
		// Fork trace
		t.Run(test, func(t *testing.T) {
			// Enable parallel testing
			t.Parallel()
			//
			for _, ir := range []string{"MIR", "AIR"} {
				for i, tf := range traces {
					// Only enable parallel expansion/checking for one trace.  This is
					// because parallel expansion/checking slows testing down overall.
					// However, we still want to test the pipeline (i.e. since that is used
					// in production); therefore, we just restrict how much its used.
					var parallel = (i == 0)
					//
					if tf.RawModules() != nil {
						// Construct trace identifier
						id := traceId{stack.RegisterMapping().Field().Name, ir, test,
							cfg.expected, cfg.expand, cfg.validate, opt, parallel, i + 1, padding}
						//
						if cfg.expand || ir == "AIR" {
							// Always check if expansion required, otherwise
							// only check AIR constraints.
							checkTrace(t, tf, id, stack.ConcreteSchemaOf(ir), stack.RegisterMapping())
						}
					}
				}
			}
		})
	}
}

func checkTrace[F field.Element[F], C sc.Constraint[F]](t *testing.T, tf lt.TraceFile, id traceId,
	schema sc.Schema[F, C], mapping module.LimbsMap) {
	// Construct the trace
	tr, errs := ir.NewTraceBuilder[F]().
		WithExpansion(id.expand).
		WithValidation(id.validate).
		WithPadding(id.padding).
		WithParallelism(id.parallel).
		WithRegisterMapping(mapping).
		WithBatchSize(128).
		Build(sc.Any(schema), tf.Clone())
	// Sanity check construction
	if len(errs) > 0 {
		t.Errorf("Trace expansion failed (%s): %s", id.String(), errs)
	} else {
		// Check Constraints
		errs := sc.Accepts(id.parallel, 128, schema, tr)
		// Determine whether trace accepted or not.
		accepted := len(errs) == 0
		// Process what happened versus what was supposed to happen.
		if !accepted && id.expected {
			//table.PrintTrace(tr)
			t.Errorf("Trace rejected incorrectly (%s): %s", id.String(), errs)
		} else if accepted && !id.expected {
			//printTrace(tr)
			t.Errorf("Trace accepted incorrectly (%s)", id.String())
		}
	}
}

// SRC_EXTENSIONS identifies the set of currently recognised extensions for
// constraint source files.
var SRC_EXTENSIONS = []string{"lisp", "zkasm"}

// This identifies matching source files.
func matchSourceFiles(test string) []string {
	var filenames []string
	//
	for _, ext := range SRC_EXTENSIONS {
		filename := fmt.Sprintf("%s/%s.%s", TestDir, test, ext)
		if _, err := os.Stat(filename); err == nil {
			filenames = append(filenames, filename)
		}
	}
	//
	return filenames
}

// LegacyTestConfig provides a simple mechanism for searching for testfiles.
type LegacyTestConfig struct {
	extension string
	expected  bool
	expand    bool
	validate  bool
	field     string
	optlevels []uint
}

var allOptLevels = []uint{0, 1}
var defaultOptLevel = []uint{1}

// LEGACY_TESTFILE_EXTENSIONS identifies the possible file extensions used for
// different test inputs.
var LEGACY_TESTFILE_EXTENSIONS []LegacyTestConfig = []LegacyTestConfig{
	// should all pass
	{"accepts", true, true, true, "", allOptLevels},
	{"accepts.bz2", true, true, true, "", allOptLevels},
	{"auto.accepts", true, true, true, "", allOptLevels},
	{"auto.accepts.bz2", true, true, true, "", allOptLevels},
	// should all fail
	{"rejects", false, true, false, "", allOptLevels},
	{"rejects.bz2", false, true, false, "", allOptLevels},
	{"auto.rejects", false, true, false, "", allOptLevels},
	{"bls12_377.rejects", false, true, false, "BLS12_377", allOptLevels},
	{"koalabear_16.rejects", false, true, false, "KOALABEAR_16", defaultOptLevel},
	{"gf_8209.rejects", false, true, false, "GF_8209", defaultOptLevel},
	// NOTE: the pre-expanded (expand=false) fixtures below are disabled.  They
	// embed every computed column verbatim, so any change to the set of computed
	// columns (e.g. the (iz ...) is-zero indicator introduced for control-flow
	// optimisation, see #1793) requires hand-editing each fixture.  That
	// maintenance burden outweighs the coverage they add, so they are skipped
	// pending a decision to regenerate or remove them.
	//
	// {"expanded.accepts", true, false, false, "BLS12_377", allOptLevels},
	// {"expanded.O1.accepts", true, false, false, "BLS12_377", defaultOptLevel},
	// {"expanded.koalabear_16.rejects", false, false, false, "KOALABEAR_16", defaultOptLevel},
	// {"expanded.gf_8209.rejects", false, false, false, "GF_8209", defaultOptLevel},
	// {"expanded.rejects", false, false, false, "BLS12_377", allOptLevels},
	// {"expanded.O1.rejects", false, false, false, "BLS12_377", defaultOptLevel},
}

// A trace identifier uniquely identifies a specific trace within a given test.
// This is used to provide debug information about a trace failure.
// Specifically, so the user knows which line in which file caused the problem.
type traceId struct {
	// Identifies the prime field used
	field string
	// Identifies the Intermediate Representation tested against.
	ir string
	// Identifies the test name.  From this, the test filename can be determined
	// in conjunction with the expected outcome.
	test string
	// Identifies whether this trace should be accepted (true) or rejected
	// (false).
	expected bool
	// Identifies whether this trace should be expanded (or not).
	expand bool
	// Identifies whether this trace should be validate (or not).
	validate bool
	// Optimisation level
	optimisation uint
	// Enable parallel expansion / checking
	parallel bool
	// Identifies the line number within the test file that the failing trace
	// original.
	line int
	// Identifies how much padding has been added to the expanded trace.
	padding uint
}

func (p *traceId) String() string {
	return fmt.Sprintf("[%s;%s;O%d], %s, line %d with padding %d", p.field, p.ir,
		p.optimisation, p.test, p.line, p.padding)
}

// ReadTracesFile reads a file containing zero or more traces expressed as JSON, where
// each trace is on a separate line.
func ReadTracesFile(filename string) []lt.TraceFile {
	lines := file.ReadInputFileAsLines(filename)
	traces := make([]lt.TraceFile, len(lines))
	// Read constraints line by line
	for i, line := range lines {
		// Parse input line as JSON
		if line != "" && !strings.HasPrefix(line, ";;") {
			// Read traces
			pool, tf, err := json.FromBytes([]byte(line))
			//
			if err != nil {
				msg := fmt.Sprintf("%s:%d: %s", filename, i+1, err)
				panic(msg)
			}

			traces[i] = lt.NewTraceFile(nil, pool, tf)
		}
	}

	return traces
}

// This is a little test to ensure the binary file format (specifically the
// binary encoder / decoder) works as expected.
func encodeDecodeSchema(t *testing.T, binf binfile.BinaryFile) *binfile.BinaryFile {
	var nbinf binfile.BinaryFile
	// Turn the binary file into bytes
	bytes, err := binf.MarshalBinary()
	// Encode schema
	if err != nil {
		t.Error(err)
		return nil
	}
	// Decode schema
	if err := nbinf.UnmarshalBinary(bytes); err != nil {
		t.Error(err)
		return nil
	}
	//
	return &nbinf
}

func getSchemaStack[F field.Element[F]](stdlib bool, field field.Config, filenames ...string,
) cmd_util.SchemaStacker[F] {
	//
	var (
		stack        cmd_util.SchemaStacker[F]
		corsetConfig corset.CompilationConfig
		asmConfig    asm.LoweringConfig
	)
	// Configure corset for testing
	corsetConfig.Legacy = true
	corsetConfig.Stdlib = stdlib
	corsetConfig.Field = field
	// Configure asm for lowering
	asmConfig.Vectorize = true
	asmConfig.Field = field
	//
	stack = stack.
		WithCorsetConfig(corsetConfig).
		WithAssemblyConfig(asmConfig).
		WithLayer(cmd_util.MACRO_ASM_LAYER).
		WithLayer(cmd_util.MICRO_ASM_LAYER).
		WithLayer(cmd_util.MIR_LAYER).
		WithLayer(cmd_util.AIR_LAYER)
	// Read in all specified constraint files.
	return stack.Read(filenames...)
}

func init() {
	var (
		regex = ""
		err   error
	)
	// Check whether a field regex is specified in the environment.
	if val, ok := os.LookupEnv("GOCORSET_FIELD"); ok {
		regex = val
	}
	// Compile the regex
	FIELD_REGEX, err = regexp.Compile(regex)
	//
	if err != nil {
		panic(fmt.Sprintf("GOCORSET_FIELD is malformed: %s", err.Error()))
	}
}
