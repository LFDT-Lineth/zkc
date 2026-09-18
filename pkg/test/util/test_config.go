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

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// TestConfig for testing
type TestConfig struct {
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
	paddingStrategies []util.Pair[string, ir.PaddingStrategy]
	// maxStaticHeights controls the maximum heights (i.e. number of rows) of static
	// range tables.  Widths whose enumeration would exceed this are range-checked
	// recursively instead.  Defaults to codegen.DEFAULT_MAX_STATIC_HEIGHT.
	maxStaticHeights []uint
	// enable sharding.
	sharding util.Option[vm.ShardingStrategy]
}

// MaxStaticHeights sets the maximum heights number of rows) of static range
// tables to test with.
func (p TestConfig) MaxStaticHeights(heights ...uint) TestConfig {
	p.maxStaticHeights = heights
	//
	return p
}

// GoGen determines whether or not to additionally run the generated-Go ("native")
// executor and check its outputs against the test.  This is experimental and only
// applies to the Uint64 word; programs the generator cannot yet handle are skipped
// rather than failed (see runGogenExecutionTest).
func (p TestConfig) GoGen(flag bool) TestConfig {
	p.gogen = flag
	//
	return p
}

// Sharding enables trace sharding with checkpoints at every n ZkC instructions.
func (p TestConfig) Sharding(fn string, n uint64) TestConfig {
	p.sharding = util.Some(vm.NewShardingStrategy(fn, n))
	//
	return p
}

// Constraints determines whether or not to check constraints.
func (p TestConfig) Constraints(flag bool) TestConfig {
	p.constraints = flag
	//
	return p
}

// Fields overrides the default set of fields to use for testing.  This is
// useful, for example, when a given test only supports a subset of the default
// fields.
func (p TestConfig) Fields(fields ...field.Config) TestConfig {
	p.fields = fields
	//
	return p
}

// Sampling sets the sampling ratio to use for the given constraint set.
func (p TestConfig) Sampling(ratio float64) TestConfig {
	p.sampling = util.Some(ratio)
	//
	return p
}

// Verbose determines whether or not printf statements and calls to #[debug]
// functions are retained during code generation.
func (p TestConfig) Verbose(flag bool) TestConfig {
	p.verbose = flag
	//
	return p
}

// Padding determines how much front padding is added to the generated trace.
func (p TestConfig) Padding(strategies []util.Pair[string, ir.PaddingStrategy]) TestConfig {
	p.paddingStrategies = strategies
	//
	return p
}

// GenerateTests applies this configuration to a given zkc test program, and
// reads in all available test vectors.
func (p TestConfig) GenerateTests(t *testing.T, test string) (tests []TestCase) {
	var testvecs map[field.Config][]TestVector = readTestVectors(t, test, p.sampling)
	// Check for each field requested
	for _, f := range p.fields {
		// Check whether field is active
		if !FIELD_REGEX.MatchString(f.Name) {
			continue
		}
		// Execution tests don't consider height or padding strategy.
		tests = append(tests, genExecutionTests(t, test, p, f, testvecs[f])...)
		// Generate tracing tests (unless constraints disabled)
		if p.constraints {
			// Tracing tests check differing static heights / padding strategies.
			tests = append(tests, genTracingTests(t, test, p, f, testvecs[f])...)
		}
	}
	// Run marshalling on every other test.
	for i := range len(tests) {
		if i%2 == 0 {
			tests[i] = tests[i].Marshalling()
		}
	}
	// Done
	return tests
}

func genExecutionTests(t *testing.T, test string, config TestConfig, f field.Config, vecs []TestVector) []TestCase {
	var (
		tests []TestCase
		//
		build = codegen.DEFAULT_CONFIG.Verbose(config.verbose).Field(f)
		// Compile test program
		prog, err = compileTestProgram(test, "zkc", build)
	)
	// Sanity check for compilation errors
	if err != nil {
		t.Error(err)
		return nil
	}
	// Fast mode tests don't consider height or padding strategy.
	for _, tv := range vecs {
		// Only run fast mode tests for the default height / padding, since
		// neither static height nor padding impacts on fast mode.
		tests = append(tests, NewTestCase(test, tv, build, prog).FastMode())
	}
	// Apply GoGen (if requested)
	if config.gogen {
		tests = applyGoGen(t, test, prog, tests)
	}
	//
	return tests
}

func genTracingTests(t *testing.T, test string, cfg TestConfig, f field.Config, vecs []TestVector) (tests []TestCase) {
	// Iterate requested static heights
	for _, height := range cfg.maxStaticHeights {
		// Construct build config
		var (
			build = codegen.DEFAULT_CONFIG.Verbose(cfg.verbose).Field(f).MaxStaticHeight(height)
			// Compile test program
			pU, err = compileTestProgram(test, "zkc", build)
		)
		// Sanity check for error
		if err != nil {
			t.Error(err)
		} else {
			// Iterate all vectors
			for _, vector := range vecs {
				// Construct base test
				var t = NewTestCase(test, vector, build, pU).Constraints().Sharding(cfg.sharding)
				// Only test different padding for default static height.
				if height == codegen.DEFAULT_MAX_STATIC_HEIGHT {
					for _, padding := range cfg.paddingStrategies {
						tests = append(tests, t.Padding(padding))
					}
				} else {
					tests = append(tests, t)
				}
			}
		}
	}
	//
	return tests
}

// Attempt enable gogen to a given set of test cases for a given zkcfile.  This
// results in a test failure if there is some compilation failure, but otherwise
// allows the tests to continue (without gogen).
func applyGoGen(t *testing.T, test string, program vm.Program[vm.Uint], tests []TestCase) []TestCase {
	// Attempt to extend tests to include gogen
	var (
		// Lower through the execution pipeline before handing to gogen.  GenerateGo
		// requires Program[Uint] with registers no wider than 64 bits, so we split
		// against a bounded word whilst staying in the Uint representation.
		pG = vm.TransformForExecutionRaw[vm.Uint, vm.Uint](program, vm.WORD_UINT128)
		// Build gogen program
		gogen, gerr = buildGogenProgram(t, pG)
	)
	// Check for any gogen errors
	if gerr != nil {
		t.Errorf("[gogen] %s: %v", test, gerr)
		// gogen cannot represent this program --- fail.
		return tests
	}
	// Apply gogen to each test
	for i := range tests {
		tests[i] = tests[i].GoGen(gogen)
	}
	// Done
	return tests
}

func readTestVectors(t *testing.T, test string, sampling util.Option[float64]) map[field.Config][]TestVector {
	var tests = make(map[field.Config][]TestVector)
	// Search for tests
	for _, cfg := range TESTFILE_EXTENSIONS {
		var fields []field.Config
		// Read tests from file
		tcs, ok := readTestsFile(t, cfg, test)
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

// readTestsFile reads a file containing zero or more tests expressed as JSON,
// where each test is on a separate line.  If the file doesn't exist, then an
// empty set of tests is returned along with false.
func readTestsFile(t *testing.T, cfg TestFileConfig, test string) ([]TestVector, bool) {
	//
	var (
		// Construct test filename
		filename = fmt.Sprintf("%s/%s.%s", TestDir, test, cfg.extension)
		// Read input file
		lines, exists = file.ReadInputFileAsLines(filename)
		//
		tests []TestVector
	)
	// Read constraints line by line
	for i, line := range lines {
		// Parse input line as JSON
		if line != "" && !strings.HasPrefix(line, ";;") {
			// Read inputs / outputs
			data, err := zkc_util.ParseJsonInputFile([]byte(line))
			//
			if err != nil {
				t.Errorf("%s:%d: %s", filename, i+1, err)
				continue
			}
			//
			tests = append(tests, TestVector{filename, uint(i + 1), cfg.expected, data})
		}
	}
	// Success
	return tests, exists
}

func applySampling(t *testing.T, test string, tc []TestVector, sampling util.Option[float64]) []TestVector {
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

// TestFileConfig provides a simple mechanism for searching for testfiles.
type TestFileConfig struct {
	extension string
	expected  bool
	// Indicates extension only suitable for specific field.  If nil, then
	// suitable for all fields.
	field *field.Config
}

// TESTFILE_EXTENSIONS identifies the possible file extensions used for
// different test inputs.
var TESTFILE_EXTENSIONS []TestFileConfig = []TestFileConfig{
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
