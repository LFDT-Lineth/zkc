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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/gogen"
)

// GoGenCompile compiles a ZkC program into a GoGen program which can be
// executed, or an error indicating some kind of compilation failure.
func GoGenCompile(program Program[Uint]) (GoGenProgram, error) {
	// Generate native Go source for the word machine.
	var (
		src, err = gogen.Generate(program, GoGenConfig{})
		dir      string
	)
	//
	if err != nil {
		return GoGenProgram{}, err
	}
	// Construct temporary directory in which to build
	dir, err = os.MkdirTemp("", "zkcgen")
	//
	if err != nil {
		return GoGenProgram{}, err
	}
	// Determine program path
	programPath := filepath.Join(dir, "prog")
	// Construct file holding contents of generated program.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		os.RemoveAll(dir)
		return GoGenProgram{}, err
	}
	// Write out go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		os.RemoveAll(dir)
		return GoGenProgram{}, err
	}
	//
	cmd := exec.Command("go", "build", "-o", programPath, ".")
	cmd.Dir = dir
	//
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return GoGenProgram{}, fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	//
	return GoGenProgram{programPath}, nil
}

// ============================================================================
// GoGenProgram
// ============================================================================

// GoGenProgram provides a handle for a GoGen program.
type GoGenProgram struct {
	// Path to the compiled executable
	path string
}

// Free any resources used by this handle, such as a physical executable stored
// on the file system.
func (p *GoGenProgram) Free() error {
	dir := filepath.Dir(p.path)
	// Remove temp directory
	return os.RemoveAll(dir)
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
func (p *GoGenProgram) Run(in map[string]bit.Array, stderr io.Writer) (map[string]bit.Array, error) {
	var (
		// Convert inputs into JSON
		input, err = MarshalInputs(in)
	)
	//
	if err != nil {
		return nil, err
	}
	//
	output, err := runRaw(p.path, input, stderr)
	//
	if err != nil {
		return nil, err
	}
	//
	return UnmarshalOutputs(output)
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

func UnmarshalOutputs(output []byte) (map[string]bit.Array, error) {
	var raw map[string]string
	//
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("decoding generated output %q: %v", string(output), err)
	}

	out := make(map[string]bit.Array, len(raw))

	for name, s := range raw {
		bytes, err := bit.ParseArray(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return nil, fmt.Errorf("decoding generated output %q: %v", name, err)
		}

		out[name] = bytes
	}

	return out, nil
}

func runRaw(path string, input []byte, stderr io.Writer) ([]byte, error) {
	var (
		stdout, captured bytes.Buffer
		cmd              = exec.Command(path)
	)
	//
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(stderr, &captured)
	//
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running generated program: %v\n%s", err, captured.String())
	}
	//
	return stdout.Bytes(), nil
}
