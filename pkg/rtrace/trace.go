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
package rtrace

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/narray"
)

// Trace describes a set of named modules whose data is organised by row.
type Trace[T any] interface {
	// Determine whether this trace has a module with the given name and, if so,
	// what its module index is.
	HasModule(name string) (uint, bool)
	// Access a given module in this trace.
	Module(uint) Module[T]
	// Returns an iterator over the contained modules.
	Modules() iter.Iterator[Module[T]]
	// Returns the number of modules in this trace.
	Width() uint
}

// Module describes a module within the trace.  Every module is a collection of
// zero or more data columns with the same height.  The width of a module is the
// number of such columns it contains.  Every column in the module has a
// "descriptor" which provides metadata about the columns, such as its name and
// declared bitwidth, etc.
type Module[T any] interface {
	fmt.Stringer
	// Append a given row onto this module.  This will panic if the length of
	// this row does not match the width of this module.
	Append(...T)
	// Module name.
	Name() string
	// Column returns the data for the column at the given index.
	Column(uint) narray.Array[T]
	// Descriptor returns the descriptor of this module.
	Descriptor() ModuleDescriptor
	// Returns the number of columns in this module.
	Width() uint
	// Returns the height (i.e. number of rows) of this module.
	Height() uint
	// Convert to an lt.Module[T].  This should be considered a destructive
	// operation, so once this is done the given module is finished.
	ToLtModule() lt.Module[T]
}

// ModuleBuilder describes an extended module which can be used for the purposes
// of constructing new modules.
type ModuleBuilder[T any, M any] interface {
	Module[T]
	// Initialise a new module from a given set of rows.
	Initialise(ModuleDescriptor, ...[]T) M
	// MutColumn returns mutable access to the data for the given column.
	MutColumn(uint) narray.MutArray[T]
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

var rtraceBinaryMagic = []byte{'r', 't', 'r', 'a', 'c', 'e', 0, 2}
