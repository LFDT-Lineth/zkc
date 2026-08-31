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
package trace

import (
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Trace represents a complete (sharded) trace.  That is, an array of shards.
type Trace[F field.Element[F]] []Shard[F]

// Shard describes an immutable set of named modules whose data is organised by
// columns.
type Shard[F field.Element[F]] struct {
	// Holds the set of modules in this trace.  The index of each module in this
	// array uniquely identifies it, and is referred to as the "module index".
	modules []Module[F]
}

// NewShard constructs a new shard from a given set of module traces.
func NewShard[F field.Element[F]](modules []Module[F]) Shard[F] {
	return Shard[F]{modules}
}

// IsEmpty determines whether or not this shard is completely empty.
func (p Shard[F]) IsEmpty() bool {
	return p.modules == nil
}

// HasModule determines whether this trace has a module with the given name and,
// if so, what its module index is.
func (p Shard[F]) HasModule(name string) (uint, bool) {
	for mid, mod := range p.modules {
		if mod.Name() == name {
			return uint(mid), true
		}
	}
	//
	return math.MaxUint, false
}

// Module returns a specific module in this trace.
func (p Shard[F]) Module(module uint) Module[F] {
	return p.modules[module]
}

// RawModule returns a specific (raw) module in this trace.
func (p Shard[F]) RawModule(module uint) Module[F] {
	return p.modules[module]
}

// Modules returns an iterator over the modules in this trace.
func (p Shard[F]) Modules() iter.Iterator[Module[F]] {
	it := iter.NewArrayIterator(p.modules)
	//
	return iter.NewCastIterator[Module[F], Module[F]](it)
}

// Width returns the number of modules in this trace.
func (p Shard[F]) Width() uint {
	return uint(len(p.modules))
}

func (p Shard[F]) String() string {
	var id strings.Builder

	id.WriteString("{")
	//
	for i, m := range p.modules {
		if i != 0 {
			id.WriteString(", ")
		}
		//
		id.WriteString(m.String())
	}
	//
	id.WriteString("}")
	//
	return id.String()
}

// ModuleDescriptor describes an individual module within a trace, including all
// of its columns.
type ModuleDescriptor struct {
	Name string
	// Descriptors for all columns
	Columns []ColumnDescriptor
	// Flag indicating replication (or not).
	Replicated bool
}

// NewModuleDescriptor constructs a straightforward module descriptor (i.e. with
// no additional metadata).
func NewModuleDescriptor(name string, columns []ColumnDescriptor) ModuleDescriptor {
	return ModuleDescriptor{name, columns, false}
}

// Width returns the number of columns in this module.
func (p ModuleDescriptor) Width() uint {
	return uint(len(p.Columns))
}

// WithReplication sets the replication metadata for this module to the given flag.
func (p ModuleDescriptor) WithReplication(flag bool) ModuleDescriptor {
	p.Replicated = flag
	return p
}

// ColumnDescriptor describes an individual column in a trace module.
type ColumnDescriptor struct {
	// Column name.
	Name string
	// Column bitwidth.  If this is none, then this represents a "native column"
	// (i.e. one backed by field elements).
	Bitwidth util.Option[uint]
}

// NewColumnDescriptor constructs a new column descriptor with the given name
// and bitwidth.
func NewColumnDescriptor(name string, bitwidth util.Option[uint]) ColumnDescriptor {
	return ColumnDescriptor{name, bitwidth}
}

var traceBinaryMagic = []byte{'r', 't', 'r', 'a', 'c', 'e', 0, 2}
