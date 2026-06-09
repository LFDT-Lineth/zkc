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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// runGogenExecutionTest cross-checks the generated-Go ("native") executor against
// a test case: it generates Go from the u64 word machine, compiles it once
// (cached), runs it on the test inputs as a subprocess, and verifies the outputs
// and accept/reject verdict match — exactly as runExecutionTest does for a Core.
//
// Programs the generator cannot yet handle (division, field ops, wide registers,
// …) are skipped rather than failed: gogen coverage is opt-in and grows over time.
// Note: runGogenExecutionTest shares its *testing.T with the whole CheckValid run
// (there is no enclosing t.Run), so it must never call t.Skip / t.Fatal — doing so
// would abort the surrounding bytecode and constraint checks.  Unsupported programs
// are logged and skipped; only genuine mismatches use t.Errorf.
func runGogenExecutionTest(t *testing.T, wm *vm.WordMachine[vm.Uint64], test TestCase, cfg vm.WordConfig) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		// No Go toolchain: silently skip this optional cross-check.
		return
	}
	// Generate native Go for this machine.
	src, err := vm.WordToGoSource(wm)
	if err != nil {
		t.Logf("[gogen %s]%s:%d unsupported, skipping: %v", cfg.Name, test.filename, test.line, err)
		return
	}
	// Compile once (binaries are cached by source across test cases / vectors).
	prog, err := gogenBinary(goBin, src)
	if err != nil {
		t.Errorf("[gogen %s]%s:%d build: %v", cfg.Name, test.filename, test.line, err)
		return
	}
	// Decode inputs and expected outputs (in word units, one uint64 per row word).
	inputs, outputs := decodeInputsOutputs(t, wm, test.data)

	got, errored, runErr := runGogenProgram(prog, toUint64Map(inputs))
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
func compareGogenOutputs(expected map[string][]vm.Uint64, got map[string][]uint64) []error {
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
// generated program consumes over JSON.
func toUint64Map(m map[string][]vm.Uint64) map[string][]uint64 {
	out := make(map[string][]uint64, len(m))
	//
	for name, words := range m {
		values := make([]uint64, len(words))
		for i, w := range words {
			values[i] = w.Uint64()
		}
		//
		out[name] = values
	}
	//
	return out
}

// runGogenProgram runs the compiled program with JSON inputs on stdin, returning
// the decoded outputs, whether it reported an execution error (exit 1 — the
// reference error path), and any harness-level error (other failures).
func runGogenProgram(prog string, in map[string][]uint64) (map[string][]uint64, bool, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return nil, false, err
	}
	//
	var stdout, stderr bytes.Buffer
	//
	cmd := exec.Command(prog)
	cmd.Stdin = bytes.NewReader(inJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	//
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, true, nil
		}
		//
		return nil, false, fmt.Errorf("running generated program: %v\n%s", err, stderr.String())
	}
	//
	var out map[string][]uint64
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, false, fmt.Errorf("decoding generated output %q: %v", stdout.String(), err)
	}
	//
	return out, false, nil
}

// GogenBuild generates+compiles (once, cached by source) a binary for the given
// generated Go source, returning its path.  Exposed for benchmarks that drive the
// generated executor directly (e.g. the three-way keccak comparison).
func GogenBuild(src string) (string, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}

	return gogenBinary(goBin, src)
}

// GogenRun runs a compiled gogen binary on the given inputs, returning its outputs,
// whether it reported an execution error, and any harness-level error.
func GogenRun(prog string, in map[string][]uint64) (map[string][]uint64, bool, error) {
	return runGogenProgram(prog, in)
}

// ---------------------------------------------------------------------------
// Build cache: each distinct generated source is compiled exactly once.
// ---------------------------------------------------------------------------

type gogenBuild struct {
	once sync.Once
	path string
	err  error
}

var (
	gogenCacheMu sync.Mutex
	gogenCache   = map[string]*gogenBuild{}
	gogenDirOnce sync.Once
	gogenDir     string
	gogenDirErr  error
)

// gogenBinary returns the path to a compiled binary for src, building it (once)
// on first request.  Subsequent calls for the same source return the cached path,
// so repeated test cases / input vectors do not re-invoke the Go compiler.
func gogenBinary(goBin, src string) (string, error) {
	gogenCacheMu.Lock()

	b := gogenCache[src]
	if b == nil {
		b = &gogenBuild{}
		gogenCache[src] = b
	}
	gogenCacheMu.Unlock()

	b.once.Do(func() { b.path, b.err = buildGogen(goBin, src) })

	return b.path, b.err
}

func buildGogen(goBin, src string) (string, error) {
	base, err := gogenBaseDir()
	if err != nil {
		return "", err
	}
	//
	sum := sha256.Sum256([]byte(src))
	dir := filepath.Join(base, hex.EncodeToString(sum[:8]))
	//
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	//
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return "", err
	}
	//
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		return "", err
	}
	//
	prog := filepath.Join(dir, "prog")
	cmd := exec.Command(goBin, "build", "-o", prog, ".")
	cmd.Dir = dir
	//
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}
	//
	return prog, nil
}

// gogenBaseDir creates (once) a shared temporary directory for generated programs.
func gogenBaseDir() (string, error) {
	gogenDirOnce.Do(func() { gogenDir, gogenDirErr = os.MkdirTemp("", "zkc-gogen-") })
	return gogenDir, gogenDirErr
}
