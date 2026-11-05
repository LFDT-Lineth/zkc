// Copyright Consensys Incorporated
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
package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/gogen"
)

// GoGenConfig controls the shape of the Go source GenerateGo produces.
type GoGenConfig = gogen.Config

// GenerateGo compiles a bytecode program into native Go source: the "fast
// execution mode" alternative to interpreting the program.  Where the bytecode
// interpreter builds a decode-dispatch loop, this emits plain Go the compiler
// can optimise — and it consumes the program over Uint (the same program the
// reference executor interprets), so the generated semantics mirror the
// reference executor, with no register splitting or word narrowing in the way.
//
// The result exposes Run(inputs) (outputs, error); with cfg.Package == "main"
// (the default) a JSON stdin/stdout harness is appended, so the artefact can be
// compiled and run as a subprocess for differential testing and benchmarking.
func GenerateGo(program Program[Uint], cfg GoGenConfig) (string, error) {
	return gogen.Generate(program, cfg)
}

// BootAndExecuteWithGogen executes the word machine by generating native Go,
// compiling it, and running the resulting binary as a subprocess — the same
// path the gogen differential tests take, exposed here as the "--gogen"
// execution mode.
func BootAndExecuteWithGogen(program Program[Uint], input map[string]bit.Array) (map[string]bit.Array, []error) {
	var (
		stats = util.NewPerfStats()
	)
	// Generate native Go source for the word machine.
	src, err := gogen.Generate(program, GoGenConfig{})
	if err != nil {
		return nil, []error{err}
	}
	// Compile the generated source to a temporary executable.
	prog, cleanup, err := Build(src)
	if err != nil {
		return nil, []error{err}
	}
	// Log result
	stats.Log(fmt.Sprintf("compiling binary %s", prog))
	//
	defer cleanup()
	// Run the compiled program, passing only the machine's declared inputs (the
	// generated harness rejects unknown keys).  Forward its debug/printf and
	// fail output to our stderr so those statements are surfaced to the user.
	outputs, errored, err := Run(prog, input, os.Stderr)
	//
	switch {
	case err != nil:
		return nil, []error{err}
	case errored:
		return nil, []error{fmt.Errorf("execution rejected (trace rejected)")}
	}
	//
	return outputs, nil
}

// Build compiles generated gogen source — as produced by vm.GenerateGo with the
// default ("main") package — into an executable, returning the path to the
// compiled binary together with a cleanup function that removes the temporary
// build directory.  The returned path is suitable for passing to Run.
//
// It mirrors the differential-test build harness: a throwaway module directory
// holding the generated main.go, compiled with the local Go toolchain.
func Build(src string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "zkcgen")
	if err != nil {
		return "", nil, err
	}
	//
	cleanup := func() { os.RemoveAll(dir) } //nolint:errcheck
	prog := filepath.Join(dir, "prog")
	//
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	//
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	//
	cmd := exec.Command("go", "build", "-o", prog, ".")
	cmd.Dir = dir
	//
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	//
	return prog, cleanup, nil
}

// Run executes a built gogen program on the given input memories, returning
// the output memories, whether the program reported an execution error (a
// rejected trace — the reference machine's error path), and any harness-level
// error (everything else).
//
// Inputs and outputs are raw memory bytes keyed by memory name, exactly as
// vm.BootAndExecute consumes/produces them: the generated harness shares the
// JSON-hex encoding `zkc exec` accepts.
//
// The generated binary's debug/printf output and any fail message are written
// to stderr; callers pass the sink they want those statements forwarded to
// (os.Stderr for `exec --gogen`, io.Discard for differential tests, which
// compare outputs rather than messages).
func Run(prog string, in map[string]bit.Array, stderr io.Writer) (map[string]bit.Array, bool, error) {
	inJSON, err := MarshalInputs(in)
	if err != nil {
		return nil, false, err
	}

	return RunRaw(prog, inJSON, stderr)
}

// MarshalInputs renders input memories as the JSON the generated harness reads:
// one "0x…" hex string per memory.  Benchmarks pre-marshal once so the timed
// loop measures the executor, not the marshalling.
func MarshalInputs(in map[string]bit.Array) ([]byte, error) {
	raw := make(map[string]string, len(in))
	for name, bits := range in {
		raw[name] = "0x" + bits.String()
	}

	return json.Marshal(raw)
}

// RunRaw is Run with the inputs already marshalled to JSON (see MarshalInputs).
//
// The protocol matches the generated main harness: a JSON object of hex
// strings on stdin, the same on stdout, exit code 1 for an execution error.
//
// The generated binary writes its debug/printf output and any fail message to
// stderr (output memories travel on stdout as JSON, so the two never interfere).
// That stream is forwarded to the caller-supplied stderr sink — so `exec
// --gogen` surfaces those statements as the interpreter paths do — while also
// being captured to enrich any harness-level error.
func RunRaw(prog string, inJSON []byte, stderr io.Writer) (map[string]bit.Array, bool, error) {
	var stdout, captured bytes.Buffer

	cmd := exec.Command(prog)
	cmd.Stdin = bytes.NewReader(inJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(stderr, &captured)

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, true, nil
		}

		return nil, false, fmt.Errorf("running generated program: %v\n%s", err, captured.String())
	}

	var raw map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, false, fmt.Errorf("decoding generated output %q: %v", stdout.String(), err)
	}

	out := make(map[string]bit.Array, len(raw))

	for name, s := range raw {
		bytes, err := bit.ParseArray(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return nil, false, fmt.Errorf("decoding generated output %q: %v", name, err)
		}

		out[name] = bytes
	}

	return out, false, nil
}
