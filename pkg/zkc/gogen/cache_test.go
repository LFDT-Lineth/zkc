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
	"os"
	"os/exec"
	"sync"
	"testing"
)

const trivialMain = "package main\n\nfunc main() {}\n"

func requireGo(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
}

// TestCacheBuildOnce checks concurrent Builds of one source share a single
// compilation and return the same binary.
func TestCacheBuildOnce(t *testing.T) {
	requireGo(t)

	c := NewCache(t.TempDir())

	var (
		wg    sync.WaitGroup
		paths [4]string
		errs  [4]error
	)

	for i := range paths {
		wg.Add(1)

		go func() {
			defer wg.Done()

			paths[i], errs[i] = c.Build(trivialMain)
		}()
	}

	wg.Wait()

	for i := range paths {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}

		if paths[i] != paths[0] {
			t.Fatalf("distinct paths for one source: %q vs %q", paths[i], paths[0])
		}
	}

	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatal(err)
	}
}

// TestCachePersistent checks a fresh Cache over the same directory reuses the
// existing binary instead of rebuilding (cross-process / cross-restart reuse).
func TestCachePersistent(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()

	first, err := NewCache(dir).Build(trivialMain)
	if err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewCache(dir).Build(trivialMain)
	if err != nil {
		t.Fatal(err)
	}

	if second != first {
		t.Fatalf("path changed across caches: %q vs %q", second, first)
	}

	after, err := os.Stat(second)
	if err != nil {
		t.Fatal(err)
	}

	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("binary was rebuilt despite persistent cache hit")
	}
}
