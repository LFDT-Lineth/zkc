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

// Package gogen compiles the Go source produced by vm.GenerateGo into
// executables.
package gogen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Build compiles src into an executable and returns its path.
func Build(src string) (string, error) {
	dir, err := os.MkdirTemp("", "zkc-gogen-")
	if err != nil {
		return "", err
	}

	return buildInDir(dir, src)
}

func buildInDir(dir, src string) (string, error) {
	prog := filepath.Join(dir, "prog")

	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command(goBin, "build", "-o", prog, ".")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}

	return prog, nil
}
