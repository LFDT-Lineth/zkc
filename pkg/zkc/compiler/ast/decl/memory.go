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
package decl

import (
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/expr"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// MemoryKind determines the type of a given memory (i.e. random access, read
// only, etc).
type MemoryKind uint8

const (
	// PUBLIC_STATIC_MEMORY represents a (public) static read-only memory.  That
	// is a ROM which never changes across all executions of a given machine.
	PUBLIC_STATIC_MEMORY = 0
	// PRIVATE_STATIC_MEMORY represents a (private) static read-only memory.  That
	// is a ROM which never changes across all executions of a given machine.
	PRIVATE_STATIC_MEMORY = 1
	// PUBLIC_READ_ONLY_MEMORY represents a (public) read-only memory which can
	// change between different executions of a given machine.
	PUBLIC_READ_ONLY_MEMORY = 2
	// PRIVATE_READ_ONLY_MEMORY represents a (private) read-only memory which
	// can change between different executions of a given machine.
	PRIVATE_READ_ONLY_MEMORY = 3
	// PUBLIC_WRITE_ONCE_MEMORY represents a (public) write-only memory which can only be
	// written once.
	PUBLIC_WRITE_ONCE_MEMORY = 4
	// PRIVATE_WRITE_ONCE_MEMORY represents a (private) write-only memory which
	// can only be written once.
	PRIVATE_WRITE_ONCE_MEMORY = 5
	// RANDOM_ACCESS_MEMORY represents the ubiquitous form of memory which
	// supports arbitrary reads / writes.  Observe that RAM is always private.
	RANDOM_ACCESS_MEMORY = 6
)

// ResolvedMemory represents a memory whose external identifiers are otherwise resolved.
// As such, it should not be possible that such a declaration refers to unknown
// (or otherwise incorrect) external components.
type ResolvedMemory = Memory[symbol.Resolved]

// UnresolvedMemory represents a memory which contains string identifiers
// for external (i.e. unlinked) components.  As such, its possible that such a
// memory may fail with an error at link time due to an unresolvable
// reference to an external component (e.g. function, RAM, ROM, etc).
type UnresolvedMemory = Memory[symbol.Unresolved]

// Memory represents a declaration of some form of memory, such as random
// access, read only, etc.
type Memory[S symbol.Symbol[S]] struct {
	// Name given to this memory variable
	name string
	// Annotations associated with this declaration.
	annotations []string
	// Kind of memory (i.e. read-only, random access, etc)
	Kind MemoryKind
	// Address bus for memory (where, for random access, the first line always
	// denotes the index type used).
	Address []variable.Descriptor[S]
	// Data bus for memory.
	Data []variable.Descriptor[S]
	// Contents (for static memory only). Each element is a compile-time constant
	// expression.
	Contents []expr.Expr[S]
}

// NewMemory constructs a new memory.
func NewMemory[S symbol.Symbol[S]](name string, kind MemoryKind, address []variable.Descriptor[S],
	data []variable.Descriptor[S], contents []expr.Expr[S]) *Memory[S] {
	// sanity checks
	if contents != nil && kind != PUBLIC_STATIC_MEMORY && kind != PRIVATE_STATIC_MEMORY {
		panic("invalid non-static memory")
	} else if contents == nil && (kind == PUBLIC_STATIC_MEMORY || kind == PRIVATE_STATIC_MEMORY) {
		panic("invalid static memory")
	}
	//
	return &Memory[S]{name: name, Kind: kind, Address: address, Data: data, Contents: contents}
}

// NewRandomAccessMemory constructs a new random access memory.
func NewRandomAccessMemory[S symbol.Symbol[S]](name string, address []variable.Descriptor[S],
	data []variable.Descriptor[S]) *Memory[S] {
	//
	return &Memory[S]{name: name, Kind: RANDOM_ACCESS_MEMORY, Address: address, Data: data}
}

// NewReadOnlyMemory constructs a new read-only access memory.
func NewReadOnlyMemory[S symbol.Symbol[S]](public bool, name string, address []variable.Descriptor[S],
	data []variable.Descriptor[S]) *Memory[S] {
	if public {
		return &Memory[S]{name: name, Kind: PUBLIC_READ_ONLY_MEMORY, Address: address, Data: data}
	}
	//
	return &Memory[S]{name: name, Kind: PRIVATE_READ_ONLY_MEMORY, Address: address, Data: data}
}

// NewWriteOnceMemory constructs a new write-once memory.
func NewWriteOnceMemory[S symbol.Symbol[S]](public bool, name string, address []variable.Descriptor[S],
	data []variable.Descriptor[S]) *Memory[S] {
	if public {
		return &Memory[S]{name: name, Kind: PUBLIC_WRITE_ONCE_MEMORY, Address: address, Data: data}
	}
	//
	return &Memory[S]{name: name, Kind: PRIVATE_WRITE_ONCE_MEMORY, Address: address, Data: data}
}

// NewStaticMemory constructs a new static memory.
func NewStaticMemory[S symbol.Symbol[S]](public bool, name string, address []variable.Descriptor[S],
	data []variable.Descriptor[S], contents []expr.Expr[S]) *Memory[S] {
	//
	if public {
		return &Memory[S]{name: name, Kind: PUBLIC_STATIC_MEMORY, Address: address, Data: data, Contents: contents}
	}
	//
	return &Memory[S]{name: name, Kind: PRIVATE_STATIC_MEMORY, Address: address, Data: data, Contents: contents}
}

// Arity implementation for Declaration interface
func (p *Memory[S]) Arity() (nInputs, nOutputs uint) {
	return uint(len(p.Address)), uint(len(p.Data))
}

// Annotations implementation for Declaration interface
func (p *Memory[S]) Annotations() []string {
	return p.annotations
}

// SetAnnotations implementation for Declaration interface
func (p *Memory[S]) SetAnnotations(annotations []string) {
	p.annotations = annotations
}

// Name implementation for Declaration interface
func (p *Memory[S]) Name() string {
	return p.name
}

// Externs implementation for Declaration interface
func (p *Memory[S]) Externs() []S {
	return nil
}

// IsInput determines whether or not this is an input memory
func (p *Memory[S]) IsInput() bool {
	return p.Kind == PRIVATE_READ_ONLY_MEMORY || p.Kind == PUBLIC_READ_ONLY_MEMORY
}

// IsOutput determines whether or not this is an output memory
func (p *Memory[S]) IsOutput() bool {
	return p.Kind == PRIVATE_WRITE_ONCE_MEMORY || p.Kind == PUBLIC_WRITE_ONCE_MEMORY
}

// IsStatic determines whether or not this is a static input memory
func (p *Memory[S]) IsStatic() bool {
	return p.Kind == PRIVATE_STATIC_MEMORY || p.Kind == PUBLIC_STATIC_MEMORY
}

// IsReadable checks whether this memory can be read or not.
func (p *Memory[S]) IsReadable() bool {
	switch p.Kind {
	case PRIVATE_READ_ONLY_MEMORY, PUBLIC_READ_ONLY_MEMORY:
		return true
	case PRIVATE_STATIC_MEMORY, PUBLIC_STATIC_MEMORY:
		return true
	case RANDOM_ACCESS_MEMORY:
		return true
	default:
		return false
	}
}

// IsWriteable checks whether this memory can be written or not.
func (p *Memory[S]) IsWriteable() bool {
	switch p.Kind {
	case PRIVATE_WRITE_ONCE_MEMORY, PUBLIC_WRITE_ONCE_MEMORY:
		return true
	case RANDOM_ACCESS_MEMORY:
		return true
	default:
		return false
	}
}
