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
	stdjson "encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/json"
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

// FIELD_REGEX is used to restrict which fields will be tested.  This is
// primarily useful for the CI pipeline where we want to test individual fields
// in separate runners.
var FIELD_REGEX *regexp.Regexp

// CheckCorset checks that all traces which we expect to be accepted are
// accepted by a given set of constraints, and all traces that we expect to be
// rejected are rejected.  All fields provided are tested against, both with and
// without padding (whereby every module's length is expanded up to the next
// power of two).
func CheckCorset(t *testing.T, test string, fields ...field.Config) {
	CheckWithFields(t, test, true, fields...)
}

// CheckCorsetNoPadding checks that all traces which we expect to be accepted
// are accepted by a given set of constraints, and all traces that we expect to
// be rejected are rejected.  All fields provided are tested against but without
// any padding.  This is useful to reduce unnecessary testing for cases where we
// know padding is not relevant.
func CheckCorsetNoPadding(t *testing.T, test string, fields ...field.Config) {
	CheckWithFields(t, test, false, fields...)
}

// CheckWithFields checks that all traces which we expect to be accepted are
// accepted by a given set of constraints, and all traces that we expect to be
// rejected are rejected.  All fields provided are tested against.
func CheckWithFields(t *testing.T, test string, padding bool, fields ...field.Config) {
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
			checkWithField[gf251.Element](t, test, padding, f)
		case field.GF_8209:
			checkWithField[gf8209.Element](t, test, padding, f)
		case field.KOALABEAR_16:
			checkWithField[koalabear.Element](t, test, padding, f)
		case field.BLS12_377:
			checkWithField[bls12_377.Element](t, test, padding, f)
		default:
			panic(fmt.Sprintf("unknown field configuration: %s", f.Name))
		}
	}
}

func checkWithField[F field.Element[F]](t *testing.T, test string, padding bool,
	field field.Config) {
	//
	var (
		filenames = matchSourceFiles(test)
		// Configure the stack for the given field.
		stacks = getSchemaStack[F](field, filenames...)
	)
	// Record how many tests executed.
	nTests := 0
	// Iterate possible testfile extensions
	for _, cfg := range LEGACY_TESTFILE_EXTENSIONS {
		var traces []trace.Trace[F]
		// Construct test filename
		testFilename := fmt.Sprintf("%s/%s.%s", TestDir, test, cfg.extension)
		// Sanity check field aligns
		if cfg.field == "" || cfg.field == field.Name {
			// Read traces from file
			traces = ReadTracesFile[F](testFilename)
			if len(traces) > 0 {
				// Run tests
				fullCheckTraces(t, testFilename, cfg, padding, traces, stacks)
			}
		}
		// Record how many tests we found
		nTests += len(traces)
	}
	// Iterate possible group (i.e. sharded) testfile extensions
	for _, cfg := range SHARDED_TESTFILE_EXTENSIONS {
		testFilename := fmt.Sprintf("%s/%s.%s", TestDir, test, cfg.extension)
		//
		if cfg.field == "" || cfg.field == field.Name {
			groups := ReadShardedTracesFile[F](testFilename)
			//
			if len(groups) > 0 {
				stack := stacks.WithOptimisationConfig(mir.DEFAULT_OPTIMISATION_LEVEL)
				checkShardedTraces(t, testFilename, padding, mir.DEFAULT_OPTIMISATION_INDEX, cfg, groups, stack)
			}
			//
			nTests += len(groups)
		}
	}
	// Sanity check at least one trace found.
	if nTests == 0 {
		panic(fmt.Sprintf("missing any tests for %s", test))
	}
}

func fullCheckTraces[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig, padding bool,
	traces []trace.Trace[F], stack cmd_util.SchemaStacker[F]) {
	// Run checks using schema compiled from source
	checkCompilerOptimisations(t, test, cfg, traces, stack)
	// Perform checks with different fields
	checkPadding(t, test, cfg, padding, traces, stack)
}

// Sanity check same outcome for all optimisation levels
func checkCompilerOptimisations[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig,
	traces []trace.Trace[F], stack cmd_util.SchemaStacker[F]) {
	// Run checks using schema compiled from source
	for _, opt := range cfg.optlevels {
		// Only check optimisation levels other than the default.
		if opt != mir.DEFAULT_OPTIMISATION_INDEX {
			// Set optimisation level
			stack = stack.WithOptimisationConfig(mir.OPTIMISATION_LEVELS[opt])
			// Apply stack
			checkTraces(t, test, false, opt, cfg, traces, stack)
		}
	}
}

// Run default optimisation over all fields, and check padding for the primary
// stack only.
func checkPadding[F field.Element[F]](t *testing.T, test string, cfg LegacyTestConfig, padding bool,
	traces []trace.Trace[F], stack cmd_util.SchemaStacker[F]) {
	//
	if cfg.field == "" || cfg.field == stack.Field().Name {
		// Set default optimisation level
		stack = stack.WithOptimisationConfig(mir.DEFAULT_OPTIMISATION_LEVEL)
		// Apply stack
		checkTraces(t, test, padding, mir.DEFAULT_OPTIMISATION_INDEX, cfg, traces, stack)
	}
}

// Check a given set of tests have an expected outcome (i.e. are
// either accepted or rejected) by a given set of constraints.
func checkTraces[F field.Element[F]](t *testing.T, test string, padding bool, opt uint, cfg LegacyTestConfig,
	traces []trace.Trace[F], stacker cmd_util.SchemaStacker[F]) {
	// For unexpected traces, we never want to explore padding (because that's
	// the whole point of unexpanded traces --- they are raw).
	if !cfg.expand {
		padding = false
	}
	// Always test without padding; additionally test with padding when
	// requested (whereby every module is expanded up to the next power of two).
	paddings := []bool{false}
	if padding {
		paddings = append(paddings, true)
	}
	// Configure stack.
	stack := stacker.Build()
	// Run through all configurations.
	for _, padding := range paddings {
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
					if tf != nil {
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

func checkTrace[F field.Element[F], C sc.Constraint[F]](t *testing.T, tf trace.Trace[F], id traceId,
	schema sc.Schema[F, C], mapping module.LimbsMap) {
	// Map the legacy padding toggle onto a padding strategy.
	paddingStrategy := ir.NaryRowPadding(0)
	if id.padding {
		paddingStrategy = ir.NextPowerOfTwoPadding
	}
	// Construct the trace
	tr, errs := ir.NewTraceBuilder[F]().
		WithExpansion(id.expand).
		WithValidation(id.validate).
		WithPadding(paddingStrategy).
		WithParallelism(id.parallel).
		WithRegisterMapping(mapping).
		WithBatchSize(128).
		Build(sc.Any(schema), tf)
	// Sanity check construction
	if len(errs) > 0 {
		t.Errorf("Trace expansion failed (%s): %s", id.String(), errs)
	} else {
		// Check Constraints
		errs := sc.Accepts(id.parallel, schema, tr)
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
var SRC_EXTENSIONS = []string{"lisp"}

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
	// Sanity check we found something
	if len(filenames) == 0 {
		panic(fmt.Sprintf("did not match any source files for test \"%s\"", test))
	}
	// Done
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
	{"bls12_377.accepts", true, true, true, "BLS12_377", allOptLevels},
	{"koalabear_16.accepts", true, true, true, "KOALABEAR_16", allOptLevels},
	{"gf_8209.accepts", true, true, true, "GF_8209", allOptLevels},
	{"bls12_377.accepts.bz2", true, true, true, "BLS12_377", allOptLevels},
	{"koalabear_16.accepts.bz2", true, true, true, "KOALABEAR_16", allOptLevels},
	{"expanded.accepts", true, false, false, "BLS12_377", allOptLevels},
	{"expanded.O1.accepts", true, false, false, "BLS12_377", defaultOptLevel},
	// should all fail
	{"rejects", false, true, false, "", allOptLevels},
	{"rejects.bz2", false, true, false, "", allOptLevels},
	{"auto.rejects", false, true, false, "", allOptLevels},
	{"bls12_377.rejects", false, true, false, "BLS12_377", allOptLevels},
	{"koalabear_16.rejects", false, true, false, "KOALABEAR_16", defaultOptLevel},
	{"gf_8209.rejects", false, true, false, "GF_8209", defaultOptLevel},
	{"expanded.koalabear_16.rejects", false, false, false, "KOALABEAR_16", defaultOptLevel},
	{"expanded.gf_8209.rejects", false, false, false, "GF_8209", defaultOptLevel},
	{"expanded.rejects", false, false, false, "BLS12_377", allOptLevels},
	{"expanded.O1.rejects", false, false, false, "BLS12_377", defaultOptLevel},
}

// SHARDED_TESTFILE_EXTENSIONS identifies the file extensions used for sharded
// tests.  Each line of such a file holds a JSON array of traces, forming one
// group of shards judged together: every shard must pass all non-bus
// constraints on its own, whilst bus balance is required of the group as a
// whole (not of any single shard).
var SHARDED_TESTFILE_EXTENSIONS []LegacyTestConfig = []LegacyTestConfig{
	{"shards.accepts", true, true, true, "", defaultOptLevel},
	{"shards.rejects", false, true, false, "", defaultOptLevel},
}

// ReadShardedTracesFile reads a file containing zero or more trace groups
// expressed as JSON, where each group is on a separate line and consists of
// an array of traces (i.e. shards).
func ReadShardedTracesFile[F field.Element[F]](filename string) [][]trace.Trace[F] {
	lines, _ := file.ReadInputFileAsLines(filename)
	groups := make([][]trace.Trace[F], len(lines))
	//
	for i, line := range lines {
		if line != "" && !strings.HasPrefix(line, ";;") {
			var shards []stdjson.RawMessage
			//
			if err := stdjson.Unmarshal([]byte(line), &shards); err != nil {
				panic(fmt.Sprintf("%s:%d: %s", filename, i+1, err))
			}
			//
			group := make([]trace.Trace[F], len(shards))
			//
			for j, raw := range shards {
				tf, err := json.FromBytes[F](raw)
				//
				if err != nil {
					panic(fmt.Sprintf("%s:%d: shard %d: %s", filename, i+1, j+1, err))
				}
				//
				group[j] = tf
			}
			//
			groups[i] = group
		}
	}
	//
	return groups
}

// Check a given set of trace groups have an expected outcome, mirroring
// checkTraces.
func checkShardedTraces[F field.Element[F]](t *testing.T, test string, padding bool, opt uint,
	cfg LegacyTestConfig, groups [][]trace.Trace[F], stacker cmd_util.SchemaStacker[F]) {
	//
	paddings := []bool{false}
	if padding {
		paddings = append(paddings, true)
	}
	//
	stack := stacker.Build()
	//
	for _, padding := range paddings {
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			//
			for _, ir := range []string{"MIR", "AIR"} {
				for i, group := range groups {
					if group != nil {
						id := traceId{stack.RegisterMapping().Field().Name, ir, test,
							cfg.expected, cfg.expand, cfg.validate, opt, false, i + 1, padding}
						//
						checkShardGroup(t, group, id, stack.ConcreteSchemaOf(ir), stack.RegisterMapping())
					}
				}
			}
		})
	}
}

// checkShardGroup checks one group of shards: each shard is built and
// checked locally (any non-bus failure marks a broken fixture, regardless of
// the expected outcome), then every bus is required to balance across the
// whole group.  Only the group-level verdict is compared against the
// expected outcome.
func checkShardGroup[F field.Element[F], C sc.Constraint[F]](t *testing.T, group []trace.Trace[F], id traceId,
	schema sc.Schema[F, C], mapping module.LimbsMap) {
	//
	var (
		built    = make([]trace.Trace[F], len(group))
		failures []sc.Failure
	)
	// Build and locally check every shard
	for i, tf := range group {
		paddingStrategy := ir.NaryRowPadding(0)
		if id.padding {
			paddingStrategy = ir.NextPowerOfTwoPadding
		}
		//
		tr, errs := ir.NewTraceBuilder[F]().
			WithExpansion(id.expand).
			WithValidation(id.validate).
			WithPadding(paddingStrategy).
			WithRegisterMapping(mapping).
			WithBatchSize(128).
			Build(sc.Any(schema), tf)
		//
		if len(errs) > 0 {
			t.Errorf("Shard %d expansion failed (%s): %s", i+1, id.String(), errs)
			return
		}
		// Check shard locally, ignoring bus failures (balance is a property
		// of the group, not of any single shard).
		for _, err := range sc.Accepts(false, schema, tr) {
			if _, isBus := err.(*bus.Failure[F]); !isBus {
				t.Errorf("Shard %d locally invalid (%s): %s", i+1, id.String(), err.Message())
				return
			}
		}
		//
		built[i] = tr
	}
	// Check every bus balances across the whole group.
	for _, busc := range busConstraintsOf(schema) {
		if failure := busc.AcceptsGroup(built...); failure != nil {
			failures = append(failures, failure)
		}
	}
	//
	accepted := len(failures) == 0
	//
	if !accepted && id.expected {
		t.Errorf("Group rejected incorrectly (%s): %s", id.String(), failures)
	} else if accepted && !id.expected {
		t.Errorf("Group accepted incorrectly (%s)", id.String())
	}
}

// busConstraintsOf extracts the bus constraints of a schema, looking through
// the MIR / AIR wrappers.
func busConstraintsOf[F field.Element[F], C sc.Constraint[F]](schema sc.Schema[F, C]) []bus.Constraint[F] {
	var buses []bus.Constraint[F]
	//
	for iter := schema.Constraints(); iter.HasNext(); {
		switch c := any(iter.Next()).(type) {
		case mir.Constraint[F]:
			if b, ok := c.Unwrap().(bus.Constraint[F]); ok {
				buses = append(buses, b)
			}
		case air.BusConstraint[F]:
			buses = append(buses, c.Unwrap())
		}
	}
	//
	return buses
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
	// Identifies whether padding has been added to the expanded trace (whereby
	// every module's length is expanded up to the next power of two).
	padding bool
}

func (p *traceId) String() string {
	return fmt.Sprintf("[%s;%s;O%d], %s, line %d with padding %t", p.field, p.ir,
		p.optimisation, p.test, p.line, p.padding)
}

// ReadTracesFile reads a file containing zero or more traces expressed as JSON, where
// each trace is on a separate line.
func ReadTracesFile[F field.Element[F]](filename string) []trace.Trace[F] {
	lines, _ := file.ReadInputFileAsLines(filename)
	traces := make([]trace.Trace[F], len(lines))
	// Read constraints line by line
	for i, line := range lines {
		// Parse input line as JSON
		if line != "" && !strings.HasPrefix(line, ";;") {
			// Read traces
			tf, err := json.FromBytes[F]([]byte(line))
			//
			if err != nil {
				msg := fmt.Sprintf("%s:%d: %s", filename, i+1, err)
				panic(msg)
			}

			traces[i] = tf
		}
	}

	return traces
}

func getSchemaStack[F field.Element[F]](field field.Config, filenames ...string,
) cmd_util.SchemaStacker[F] {
	//
	var (
		stack        cmd_util.SchemaStacker[F]
		corsetConfig corset.CompilationConfig
	)
	// Configure corset for testing
	corsetConfig.Field = field
	//
	stack = stack.
		WithCorsetConfig(corsetConfig).
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
