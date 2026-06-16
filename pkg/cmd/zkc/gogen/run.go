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
package gogen

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

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
func Run(prog string, in map[string][]byte, stderr io.Writer) (map[string][]byte, bool, error) {
	inJSON, err := MarshalInputs(in)
	if err != nil {
		return nil, false, err
	}

	return RunRaw(prog, inJSON, stderr)
}

// MarshalInputs renders input memories as the JSON the generated harness reads:
// one "0x…" hex string per memory.  Benchmarks pre-marshal once so the timed
// loop measures the executor, not the marshalling.
func MarshalInputs(in map[string][]byte) ([]byte, error) {
	raw := make(map[string]string, len(in))
	for name, bytes := range in {
		raw[name] = "0x" + hex.EncodeToString(bytes)
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
func RunRaw(prog string, inJSON []byte, stderr io.Writer) (map[string][]byte, bool, error) {
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

	out := make(map[string][]byte, len(raw))

	for name, s := range raw {
		bytes, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return nil, false, fmt.Errorf("decoding generated output %q: %v", name, err)
		}

		out[name] = bytes
	}

	return out, false, nil
}
