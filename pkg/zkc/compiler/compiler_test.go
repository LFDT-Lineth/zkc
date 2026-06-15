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
package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// TestCompileIncludeDedupAcrossPaths guards against a regression where the same
// physical file, included through two different relative spellings, was parsed
// twice and produced spurious "duplicate declaration" errors.
//
// The layout mirrors the RISC-V interpreter: main.zkc lives in riscv/ and
// includes "memory.zkc" directly; it also pulls in a sibling library file
// (../lib/leaf.zkc) that re-includes the very same memory.zkc via the
// up-and-back path "../riscv/memory.zkc".  filepath.Join cannot collapse that
// spelling to "memory.zkc", so dedup must canonicalise to the absolute path.
func TestCompileIncludeDedupAcrossPaths(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "riscv", "main.zkc"),
		"include \"memory.zkc\"\ninclude \"../lib/leaf.zkc\"\nfn main() {\n  var x:u8 = THE_CONST\n  return\n}\n")
	mustWrite(t, filepath.Join(root, "riscv", "memory.zkc"),
		"const THE_CONST:u8 = 7\n")
	mustWrite(t, filepath.Join(root, "lib", "leaf.zkc"),
		"include \"../riscv/memory.zkc\"\n")

	// Compile with the working directory inside riscv/, so main.zkc is a bare
	// path (dir ".") — exactly the spelling that exposed the bug.
	restore := chdir(t, filepath.Join(root, "riscv"))
	defer restore()

	files, err := source.ReadFiles("main.zkc")
	if err != nil {
		t.Fatalf("read main.zkc: %v", err)
	}

	_, _, errs := Compile(field.KOALABEAR_16, files...)

	for _, e := range errs {
		if strings.Contains(e.Message(), "duplicate declaration") {
			t.Fatalf("memory.zkc parsed twice (include dedup failed): %v", e.Message())
		}
	}

	if len(errs) > 0 {
		t.Fatalf("unexpected compile errors: %v", errs)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Chdir(prev); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}
}
