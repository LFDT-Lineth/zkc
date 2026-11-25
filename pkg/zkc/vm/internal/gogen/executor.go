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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/lazy"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
	log "github.com/sirupsen/logrus"
)

// ============================================================================
// GoGenExecutor
// ============================================================================

// Executor provides a means of reusing compiled binaries across different calls
// to Compile().  Concurrent requests for the same program / configuration pair
// share a single compilation, whilst those for different ones compile in
// parallel.  Finally, resources are released when Close is called.
type Executor struct {
	manager *lazy.ResourceManager[string, Binary]
}

// NewExecutor constructs a new gogen executor.
func NewExecutor() *Executor {
	return &Executor{
		manager: lazy.NewResourceManager("gogen", compile, release),
	}
}

// Handle implementation for Closeable interface.
func (p *Executor) Handle() string {
	return "gogen"
}

// Close implementable for Closeable interface.  This releases all cached
// results and prevents anything more from being compiled.  This waits for any
// compilations still in flight, so that their binaries are released as well.
func (p *Executor) Close() error {
	var n, err = p.manager.Close()
	//
	log.Info("gogen: released ", n, " cached binaries")
	//
	return err
}

// Compile compiles a ZkC program into a GoGen program which can be executed, or
// an error indicating some kind of compilation failure.  This requires a Go
// toolchain on the PATH, and fails if none is found.  Furthermore, the outcome
// of that compilation (whether a binary or a failure) is cached such that it is
// reused on subsequent calls.  Once the executor is closed, this always fails
// with a [lazy.ResourceManagerClosed] error.
func (p *Executor) Compile(program descriptor.Program[word.Uint], config Config) (Binary, error) {
	var stats = util.NewPerfStats()
	// Check whether manager is closed already, or not.
	if p.manager.IsClosed() {
		return Binary{}, lazy.NewErrResourceManagerClosed("gogen")
	}
	// Generate unformatted source program as the resource key.
	key, err := generateRaw(program, config)
	// Sanity check
	if err != nil {
		return Binary{}, err
	}
	// Get the given resource from the manager, potentially compiling it from
	// scratch as necessary.
	binary, cached, err := p.manager.Get(key)
	//
	if err != nil {
		return Binary{}, err
	} else if cached {
		log.Info("gogen: reusing cached binary ", binary.path)
	} else {
		log.Info("gogen: compiled gogen binary ", binary.path, " ", stats.String())
	}
	//
	return binary, nil
}

func compile(key string) (binary Binary, err error) {
	stats := util.NewPerfStats()
	// Format source before building it
	if source, fmtErr := formatSource(key); fmtErr != nil {
		err = fmtErr
	} else {
		binary, err = Build(source)
	}
	// Log stats
	log.Info("Compiled gogen binary ", binary.path, " ", stats.String())
	// Done
	return binary, err
}

func release(b Binary) error {
	return b.Free()
}
