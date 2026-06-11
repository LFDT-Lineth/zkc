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
package gogen_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

var update = flag.Bool("update", false, "rewrite the golden files from current generator output")

// TestGolden pins the exact generated source for a handful of small fixtures:
// the leanness regression suite.  Any change to the emitted shape shows up as
// a reviewable golden diff (regenerate with `go test -run TestGolden -update`).
//
// "Golden file" is the conventional Go-testing term for a checked-in file
// holding a test's expected output, compared byte-for-byte and regenerated
// via an -update flag (the pattern used throughout the Go standard library);
// the .golden extension just avoids the toolchain treating the expected
// output as a Go source file.
func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"tutorial", tutorialSrc},    // arithmetic + memory + store checks
		{"destructure", destructSrc}, // multi-register store (StoreAcross)
		{"branch", branchSrc},        // SKIP_IF / SKIP control flow
		{"call", callSrc},            // CALL / RETURN width checks
		{"carry", carrySrc},          // 128-bit pair accumulation
		{"divmod", divModSrc},        // INT_DIV / INT_REM
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The importable-package artefact (no JSON harness): the shape a
			// `zkc generate` consumer would review.
			src, err := vm.GenerateGo(compileUint(t, tc.src, false), vm.GoGenConfig{Package: "golden"})
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}

			path := filepath.Join("testdata", "golden", tc.name+".go.golden")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}

				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file (run `go test -run TestGolden -update`): %v", err)
			}

			if string(want) != src {
				t.Errorf("generated source differs from %s (run `go test -run TestGolden -update`"+
					" and review the diff)\n--- got ---\n%s", path, src)
			}
		})
	}
}
