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

// Module describes a module within the trace.  Every module is composed of some
// number of rows, and has a specific width.
type Module[T any] interface {
	fmt.Stringer
	// Append a given row onto this module.  This will panic if the length of
	// this row does not match the width of this module.
	Append(...T)
	// Module name.
	Name() string
	// Descriptor returns the registers describing the columns of this module.
	Descriptor() iter.Iterator[Register]
	// RegisterAt returns the register at the given index.
	RegisterAt(uint) Register
	// Access a given row in this module.
	Row(uint) Row[T]
	// Returns the number of columns in this module.
	Width() uint
	// Returns the height of this module.
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
	Initialise(string, []Register, ...[]T) M
}

// Register describes an individual register in a row-major trace module.
type Register struct {
	// Register name.
	Name string
	// Register bitwidth.  If this is none, then this represents a "native
	// register" (i.e. one backed by field elements).
	Bitwidth util.Option[uint]
}

// NewRegister constructs a new trace register with the given name and bitwidth.
func NewRegister(name string, bitwidth util.Option[uint]) Register {
	return Register{name, bitwidth}
}

// Row describes an individual row of data within a trace table.
type Row[T any] interface {
	// Get the value in a given column of this row.
	Get(column uint) T
	// Returns the number of columns in this row.
	Width() uint
}

var rtraceBinaryMagic = []byte{'r', 't', 'r', 'a', 'c', 'e', 0, 1}
