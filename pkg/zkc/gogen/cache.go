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
// executables, caching each distinct source so it is built exactly once.
//
// The cache is keyed by the sha256 of the source: concurrent callers for the
// same source share a single `go build`, and with a persistent directory a
// binary built for one request is reused by later ones (e.g. a prover
// compiling programs received over the wire, across proof requests — and
// across restarts, since an existing binary is reused without rebuilding).
// There is no eviction: distinct sources accumulate until the directory is
// removed, which is the caller's policy decision, not this package's.
package gogen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Cache builds gogen sources into executables, at most once per distinct
// source.  The zero value is not usable; construct with NewCache.
type Cache struct {
	// dir is the root under which per-source build directories live; created
	// lazily ("" → a process-lifetime temporary directory).
	dirOnce sync.Once
	dir     string
	dirErr  error
	// builds single-flights concurrent requests for the same source, keyed by
	// its sha256 (sources can be large; the digest also names the build dir).
	mu     sync.Mutex
	builds map[string]*build
}

type build struct {
	once sync.Once
	path string
	err  error
}

// NewCache returns a cache rooted at dir.  An empty dir means a temporary
// directory created on first use (per-process cache); a fixed dir makes the
// cache persistent — binaries found there are reused without rebuilding.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir, builds: map[string]*build{}}
}

// Build returns the path to an executable for the given generated source
// (which must be a package main, i.e. produced with GoGenConfig.Package ==
// "main"), compiling it if this cache has not seen the source before.
func (c *Cache) Build(src string) (string, error) {
	c.dirOnce.Do(func() {
		if c.dir == "" {
			c.dir, c.dirErr = os.MkdirTemp("", "zkc-gogen-")
		}
	})

	if c.dirErr != nil {
		return "", c.dirErr
	}

	sum := sha256.Sum256([]byte(src))
	key := hex.EncodeToString(sum[:])

	c.mu.Lock()

	b := c.builds[key]
	if b == nil {
		b = &build{}
		c.builds[key] = b
	}

	c.mu.Unlock()

	b.once.Do(func() { b.path, b.err = buildOnce(filepath.Join(c.dir, key), src) })

	return b.path, b.err
}

// buildOnce compiles src in the given content-addressed directory.  A binary
// already present (a previous process with the same persistent root) is
// reused as-is.
func buildOnce(dir, src string) (string, error) {
	prog := filepath.Join(dir, "prog")
	// Persistent-cache hit: the binary is content-addressed by its source.
	if _, err := os.Stat(prog); err == nil {
		return prog, nil
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
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

// defaultCache backs the package-level Build: one shared per-process cache.
var defaultCache = NewCache("")

// Build compiles src using a process-wide cache rooted in a temporary
// directory.  Callers wanting persistence across processes use NewCache with
// a fixed directory instead.
func Build(src string) (string, error) {
	return defaultCache.Build(src)
}
