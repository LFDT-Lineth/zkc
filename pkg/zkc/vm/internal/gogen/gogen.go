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
package gogen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ErrMachineFailure is used to indicate when a gogen binary leads to a machine
// failure, as opposed to any other kind of internal error.
var ErrMachineFailure = errors.New("machine failure")

// Compile compiles a ZkC program into a GoGen program which can be
// executed, or an error indicating some kind of compilation failure.
func Compile(program descriptor.Program[word.Uint]) (Binary, error) {
	// Generate native Go source for the word machine.
	var (
		src, err = Generate(program, Config{})
		dir      string
	)
	//
	if err != nil {
		return Binary{}, err
	}
	// Construct temporary directory in which to build
	dir, err = os.MkdirTemp("", "zkcgen")
	//
	if err != nil {
		return Binary{}, err
	}
	// Determine program path
	programPath := filepath.Join(dir, "prog")
	// Construct file holding contents of generated program.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, err
	}
	// Write out go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, err
	}
	//
	cmd := exec.Command("go", "build", "-o", programPath, ".")
	cmd.Dir = dir
	//
	if out, err := cmd.CombinedOutput(); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	//
	return Binary{programPath}, nil
}

// ============================================================================
// GoGenProgram
// ============================================================================

// Binary provides a handle for a (compiled) GoGen program.
type Binary struct {
	// Path to the compiled executable
	path string
}

// Free any resources used by this handle, such as a physical executable stored
// on the file system.
func (p *Binary) Free() error {
	if p.path != "" {
		// Remove temp directory
		return os.RemoveAll(filepath.Dir(p.path))
	}
	//
	return nil
}

// Run executes the compiled gogen binaryon the given input memories, returning
// the output memories, along with any errors arising.
//
// Inputs and outputs are raw memory bytes keyed by memory name, exactly as
// vm.BootAndExecute consumes/produces them: the generated harness shares the
// JSON-hex encoding `zkc exec` accepts.
//
// The generated binary's debug/printf output and any fail message are written
// to stderr; callers pass the sink they want those statements forwarded to
// (os.Stderr for `exec --gogen`, io.Discard for differential tests, which
// compare outputs rather than messages).
func (p *Binary) Run(in map[string]bit.Array, stderr io.Writer) (map[string]bit.Array, error) {
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

// UnmarshalOutputs is the opposite of MarshalInputs --- it converts the JSON
// output from a machine execution back into a more structured form.
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
		stdout bytes.Buffer
		cmd    = exec.Command(path)
	)
	//
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	//
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, ErrMachineFailure
		}
		//
		return nil, err
	}
	//
	return stdout.Bytes(), nil
}
