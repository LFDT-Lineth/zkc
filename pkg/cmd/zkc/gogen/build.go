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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
