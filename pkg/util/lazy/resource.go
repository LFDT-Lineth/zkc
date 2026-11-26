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
package lazy

import (
	"errors"
	"fmt"
	"sync"
)

// ErrResourceManagerClosed is the sentinel whose address matches any
// *ResourceManagerClosed error, regardless of the manager's handle.
var ErrResourceManagerClosed = ResourceManagerClosed{}

var errGenerationAborted = errors.New("resource generation did not complete")

// ResourceManagerClosed reports that a [ResourceManager] has stopped accepting
// requests. Its message identifies the manager by the handle supplied to
// [NewResourceManager]. Use errors.Is(err, &lazy.ErrResourceManagerClosed) to
// recognize this error even when wrapped.
type ResourceManagerClosed struct {
	message string
}

// NewErrResourceManagerClosed returns a closed-manager error for the given handle.
func NewErrResourceManagerClosed(handle string) *ResourceManagerClosed {
	return &ResourceManagerClosed{
		fmt.Sprintf("%s: resource manager closed", handle),
	}
}

// Error implementation for error interface.
func (e *ResourceManagerClosed) Error() string {
	return e.message
}

// Is matches the shared closed-manager sentinel rather than the handle.
func (e *ResourceManagerClosed) Is(target error) bool {
	return target == &ErrResourceManagerClosed
}

// A computation is the (possibly still in-flight) outcome of generating the
// resource for a given key, and is shared by every caller requesting that key.
// The caller which created the computation is its owner: it alone generates the
// resource, writes the outcome and then closes the computation.  Everybody else
// must wait until the computation is closed before accessing the resource.
type computation[T any] struct {
	// Closed by the owner once binary and err have been written.
	done chan struct{}
	// Generated resource (only valid when err is nil).
	resource T
	// Generation failure (if any).
	err error
}

// ResourceManager provides a means of managing generated resources which eventually
// should be released.  Each resource is identified by a key which is used to
// generate the resource.  Upon the first attempt to get the resource associated
// with a given key, the resource is generated from scratch.  Upon subsequent
// attempts, the cached value is reused.  Concurrent requests for the same key
// are handled gracefully by allowing one of the requests to generate the
// resource, with the remainder picking up the cached value.  Finally, resources
// are released when Close is called.
type ResourceManager[K comparable, V any] struct {
	handle string
	//
	database map[K]*computation[V]
	// Set by Close, after which no more claims are accepted.
	closed bool
	// Protects database and closed (but not the computations it holds).
	mu sync.Mutex
	// generate function
	generate func(K) (V, error)
	// release function
	release func(V) error
	// Shares cleanup completion and its result across all callers of Close.
	closeOnce func() (uint, error)
}

// NewResourceManager constructs a fresh resource manager which uses the given
// generate and release functions.
func NewResourceManager[K comparable, V any](handle string, generate func(K) (V, error), release func(V) error,
) *ResourceManager[K, V] {
	p := &ResourceManager[K, V]{
		handle:   handle,
		database: make(map[K]*computation[V]),
		generate: generate,
		release:  release,
	}
	p.closeOnce = sync.OnceValues(p.closeAndReleaseAll)

	return p
}

// Get the resource associated with a given key, either by generating it from
// scratch or reusing a previously cached result.  This returns a flag which
// signals whether a cached resource was used, or not.
// If the generator panics, the panic propagates to its caller. Other callers
// for the same key receive a cached error, and Close does not release the resource.
func (p *ResourceManager[K, V]) Get(key K) (resource V, cached bool, err error) {
	var empty V
	// Claim rechecks closure in case Close ran during generation.
	entry, owner, err := p.claimComputation(key)
	//
	if err != nil {
		return empty, false, err
	}
	// Check whether somebody else is (or was) responsible for generating this.
	if !owner {
		// Yes, so wait for them to finish (if they haven't already).
		<-entry.done
		//
		return entry.resource, true, entry.err
	}
	// No, so generate resource ourself.  Waiters are released upon leaving this
	// function, even if the computation panics.
	defer close(entry.done)
	// Set initial error value so that, if generate panics, the entry will be
	// closed with this error.
	entry.err = errGenerationAborted
	entry.resource, entry.err = p.generate(key)
	//
	return entry.resource, false, entry.err
}

// Close prevents new claims, waits for in-flight computations, and releases
// their resources. Concurrent and subsequent calls wait for the same cleanup
// and return the same release count and error.
func (p *ResourceManager[K, V]) Close() (uint, error) {
	return p.closeOnce()
}

// IsClosed checks whether this resource manager is closed (or not).
func (p *ResourceManager[K, F]) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.closed
}

// claimComputation returns the computation for a given key, creating it if there is none.
// In the latter case this returns true to signal that the caller is now its
// owner and, hence, responsible for completing it.  No computation can be
// claimed once the executor is closed.
func (p *ResourceManager[K, V]) claimComputation(key K) (*computation[V], bool, error) {
	// Take lock
	p.mu.Lock()
	// Release lock upon leaving function
	defer func() { p.mu.Unlock() }()
	// Check whether or not executor still accepting claims.
	if p.closed {
		return nil, false, NewErrResourceManagerClosed(p.handle)
	}
	// Check whether or not this computation already exists.
	if entry, exists := p.database[key]; exists {
		// Yes, return it.
		return entry, false, nil
	}
	// No, so register it before anybody else does.
	entry := &computation[V]{done: make(chan struct{})}
	p.database[key] = entry
	//
	return entry, true, nil
}

func (p *ResourceManager[K, V]) closeAndReleaseAll() (uint, error) {
	var (
		errs      []error
		entries   = p.closeAndDetachAll()
		nReleased uint
	)
	//
	for _, entry := range entries {
		// Wait for computation to finish (if it hasn't already).
		<-entry.done
		// Failed computation hold no resources.
		if entry.err == nil {
			// Release resources
			errs = append(errs, p.release(entry.resource))
			//
			nReleased++
		}
	}
	// Return any errors arising
	return nReleased, errors.Join(errs...)
}

// closeAndDetachAll closes the executor, then removes every computation from
// the database and returns them.  Doing both under the lock ensures every
// computation ever claimed is amongst those returned by some call to this.  The
// lock is not held whilst the caller waits on them.
func (p *ResourceManager[K, V]) closeAndDetachAll() []*computation[V] {
	var entries []*computation[V]
	// Take lock
	p.mu.Lock()
	// Release lock upon leaving function
	defer func() { p.mu.Unlock() }()
	// Refuse any further claims
	p.closed = true
	//
	for _, v := range p.database {
		entries = append(entries, v)
	}
	// Reset database
	clear(p.database)
	//
	return entries
}
