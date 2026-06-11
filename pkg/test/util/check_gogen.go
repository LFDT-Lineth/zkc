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
	"os/exec"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// runGogenExecutionTest cross-checks the generated-Go ("native") executor against
// a test case: it generates Go from the word machine (over Uint — the same
// machine the reference executor interprets), compiles it once (cached), runs it
// on the test inputs as a subprocess, and verifies the outputs and accept/reject
// verdict match — exactly as runExecutionTest does for a Core.
//
// Programs the generator cannot yet handle (wide registers/constants/moduli, …)
// are skipped rather than failed: gogen coverage grows over time.
// Note: runGogenExecutionTest shares its *testing.T with the whole CheckValid run
// (there is no enclosing t.Run), so it must never call t.Skip / t.Fatal — doing so
// would abort the surrounding bytecode and constraint checks.  Unsupported programs
// are logged and skipped; only genuine mismatches use t.Errorf.
func runGogenExecutionTest(t *testing.T, wm *vm.WordMachine[vm.Uint], test TestCase, cfg vm.WordConfig) {
	if _, err := exec.LookPath("go"); err != nil {
		// No Go toolchain: silently skip this optional cross-check.
		return
	}
	// Generate native Go for this machine.
	src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
	if err != nil {
		t.Logf("[gogen %s]%s:%d unsupported, skipping: %v", cfg.Name, test.filename, test.line, err)
		return
	}
	// Compile once (binaries are cached by source across test cases / vectors).
	prog, err := gogen.Build(src)
	if err != nil {
		t.Errorf("[gogen %s]%s:%d build: %v", cfg.Name, test.filename, test.line, err)
		return
	}
	// Decode inputs and expected outputs (in word units, one uint64 per row word).
	inputs, outputs := decodeInputsOutputs(t, wm, test.data)

	in, ok := toUint64Map(inputs)
	if !ok {
		// An input word beyond 64 bits cannot cross the JSON boundary; such
		// vectors stay covered by the reference executor.
		t.Logf("[gogen %s]%s:%d input exceeds 64 bits, skipping", cfg.Name, test.filename, test.line)
		return
	}

	got, errored, runErr := gogen.Run(prog, in)
	if runErr != nil {
		t.Errorf("[gogen %s]%s:%d run: %v", cfg.Name, test.filename, test.line, runErr)
		return
	}

	var errs []error
	// Mirror runExecutionTest's accept/reject logic.
	switch {
	case errored && test.expected:
		errs = append(errs, fmt.Errorf("rejected accepted trace"))
	case !errored && !test.expected:
		errs = append(errs, fmt.Errorf("accepted rejected trace"))
	case !errored && test.expected:
		errs = append(errs, compareGogenOutputs(outputs, got)...)
	}

	for _, err := range errs {
		t.Errorf("[gogen %s]%s:%d %v", cfg.Name, test.filename, test.line, err)
	}
}

// compareGogenOutputs checks each expected output memory against the generated
// program's output of the same name.
func compareGogenOutputs(expected map[string][]vm.Uint, got map[string][]uint64) []error {
	var errs []error
	//
	for name, want := range expected {
		have := got[name]
		//
		if len(have) != len(want) {
			errs = append(errs, fmt.Errorf("output %q: length mismatch (expected %d, got %d)", name, len(want), len(have)))
			continue
		}
		//
		for i := range want {
			if want[i].Uint64() != have[i] {
				errs = append(errs, fmt.Errorf("output %q[%d]: expected %d, got %d", name, i, want[i].Uint64(), have[i]))
				break
			}
		}
	}
	//
	return errs
}

// toUint64Map converts decoded word inputs into the plain uint64 form the
// generated program consumes over JSON, reporting whether all words fit.
func toUint64Map(m map[string][]vm.Uint) (map[string][]uint64, bool) {
	out := make(map[string][]uint64, len(m))
	//
	for name, words := range m {
		values := make([]uint64, len(words))
		for i, w := range words {
			if !w.FitsWithin(64) {
				return nil, false
			}

			values[i] = w.Uint64()
		}
		//
		out[name] = values
	}
	//
	return out, true
}
