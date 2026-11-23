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
	"errors"
	"sync"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
	log "github.com/sirupsen/logrus"
)

// A compilation key uniquely describes a given compilation event.
// Specifically, this is the program being compiled along with the configuration
// used.
type compilationKey struct {
	// Program which was compiled
	program descriptor.Program[word.Uint]
	// Configuration of program
	config Config
}

// Equals determines whether or not two compilation events are really the same
// (i.e. same program being compiled under the same configuration).
func (p compilationKey) Equals(q compilationKey) bool {
	return p.config.Equals(q.config) && p.program.Equals(q.program)
}

// Hash returns a suitable hash code for a given compilation event.
func (p compilationKey) Hash() uint64 {
	var (
		pHash = p.program.Hash()
		cHash = p.config.Hash()
	)
	//
	return hash.Uint64s(cHash, pHash)
}

// A compilation is the (possibly still in-flight) outcome of compiling a given
// key, and is shared by every caller requesting that key.  The caller which
// created the compilation is its owner: it alone runs the compiler, writes the
// outcome and then closes done.  Everybody else must wait on done before
// reading the outcome.
type compilation struct {
	// Closed by the owner once binary and err have been written.
	done chan struct{}
	// Compiled binary (only valid when err is nil).
	binary Binary
	// Compilation failure (if any).
	err error
}

// ============================================================================
// GoGenExecutor
// ============================================================================

// Executor provides a means of reusing compiled binaries across different calls
// to Compile().  Specifically, it caches the programs being compiled along with
// their configurations and reuses them when possible.  Concurrent requests for
// the same program and configuration share a single compilation, whilst those
// for different ones compile in parallel.  Finally, resources are released when
// ClearAll is called.
type Executor struct {
	database *hash.Map[compilationKey, *compilation]
	// Protects database (but not the compilations it holds, see compilation).
	mu sync.Mutex
}

// NewExecutor constructs a new gogen executor.
func NewExecutor() *Executor {
	return &Executor{
		database: hash.NewMap[compilationKey, *compilation](0),
	}
}

// Compile compiles a ZkC program into a GoGen program which can be executed, or
// an error indicating some kind of compilation failure.  This requires a Go
// toolchain on the PATH, and fails if none is found.  Furthermore, the outcome
// of that compilation (whether a binary or a failure) is cached such that it is
// reused on subsequent calls, until ClearAll is called.
func (p *Executor) Compile(program descriptor.Program[word.Uint], config Config) (Binary, error) {
	var entry, owner = p.claim(compilationKey{program, config})
	// Check whether somebody else is (or was) responsible for compiling this.
	if !owner {
		// Yes, so wait for them to finish (if they haven't already).
		<-entry.done
		//
		if entry.err == nil {
			log.Info("Reusing cached gogen binary ", entry.binary.path)
		}
		//
		return entry.binary, entry.err
	}
	// No, so compile program ourselves.  Waiters are released upon leaving this
	// function, even if the compiler panics.
	defer close(entry.done)
	//
	stats := util.NewPerfStats()
	//
	entry.binary, entry.err = Compile(program, config)
	// Log stats
	log.Info("Compiled gogen binary ", entry.binary.path, " ", stats.String())
	//
	return entry.binary, entry.err
}

// ClearAll releases all cached results and resets the internal cache to an
// empty state.  This waits for any compilations still in flight, so that their
// binaries are released as well.
func (p *Executor) ClearAll() error {
	var (
		errs    []error
		entries = p.detachAll()
	)
	//
	log.Info("Releasing ", len(entries), " cached gogen binaries")
	//
	for _, entry := range entries {
		// Wait for compilation to finish (if it hasn't already).
		<-entry.done
		// Failed compilations hold no resources.
		if entry.err == nil {
			// Release resources
			errs = append(errs, entry.binary.Free())
		}
	}
	// Return any errors arising
	return errors.Join(errs...)
}

// claim returns the compilation for a given key, creating it if there is none.
// In the latter case this returns true to signal that the caller is now its
// owner and, hence, is responsible for completing it.
func (p *Executor) claim(key compilationKey) (*compilation, bool) {
	// Take lock
	p.mu.Lock()
	// Release lock upon leaving function
	defer func() { p.mu.Unlock() }()
	// Check whether or not this compilation already exists.
	if entry, exists := p.database.Get(key); exists {
		// Yes, return it.
		return entry, false
	}
	// No, so register it before anybody else does.
	entry := &compilation{done: make(chan struct{})}
	p.database.Insert(key, entry)
	//
	return entry, true
}

// detachAll removes every compilation from the database and returns them.  The
// lock is not held whilst the caller waits on them.
func (p *Executor) detachAll() []*compilation {
	var entries []*compilation
	// Take lock
	p.mu.Lock()
	// Release lock upon leaving function
	defer func() { p.mu.Unlock() }()
	//
	for iter := p.database.KeyValues(); iter.HasNext(); {
		entries = append(entries, iter.Next().Right)
	}
	// Reset database
	p.database.Clear()
	//
	return entries
}
