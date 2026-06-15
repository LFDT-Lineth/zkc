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
	"encoding/json"
	"fmt"
	"os/exec"
)

// Run executes a built gogen program on the given inputs, returning the
// decoded outputs, whether the program reported an execution error (a
// rejected trace — the reference machine's error path), and any
// harness-level error (everything else).
func Run(prog string, in map[string][]uint64) (map[string][]uint64, bool, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return nil, false, err
	}

	return RunRaw(prog, inJSON)
}

// RunRaw is Run with the inputs already marshalled to JSON — benchmarks
// pre-marshal once so the timed loop measures the executor, not
// json.Marshal of the inputs.
//
// The protocol matches the generated main harness: JSON inputs on stdin,
// JSON outputs on stdout, exit code 1 for an execution error.
func RunRaw(prog string, inJSON []byte) (map[string][]uint64, bool, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command(prog)
	cmd.Stdin = bytes.NewReader(inJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, true, nil
		}

		return nil, false, fmt.Errorf("running generated program: %v\n%s", err, stderr.String())
	}

	var out map[string][]uint64
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, false, fmt.Errorf("decoding generated output %q: %v", stdout.String(), err)
	}

	return out, false, nil
}
