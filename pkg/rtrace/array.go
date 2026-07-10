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
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Array provides an implementation of Trace which stores rows as an array.
type Array[T any] struct {
	// Holds the set of modules in this trace.  The index of each module in this
	// array uniquely identifies it, and is referred to as the "module index".
	modules []ArrayModule[T]
}

var _ Trace[any] = (*Array[any])(nil)

// NewArray constructs a row-major trace from a given set of modules.
func NewArray[T any](modules []ArrayModule[T]) *Array[T] {
	return &Array[T]{modules}
}

// HasModule determines whether this trace has a module with the given name and,
// if so, what its module index is.
func (p *Array[T]) HasModule(name string) (uint, bool) {
	for mid, mod := range p.modules {
		if mod.name == name {
			return uint(mid), true
		}
	}
	//
	return math.MaxUint, false
}

// Module returns a specific module in this trace.
func (p *Array[T]) Module(module uint) Module[T] {
	return p.modules[module]
}

// RawModule returns a specific module in this trace.
func (p *Array[T]) RawModule(module uint) *ArrayModule[T] {
	return &p.modules[module]
}

// Modules returns an iterator over the modules in this trace.
func (p *Array[T]) Modules() iter.Iterator[Module[T]] {
	arr := iter.NewArrayIterator(p.modules)
	return iter.NewCastIterator[ArrayModule[T], Module[T]](arr)
}

// Width returns the number of modules in this trace.
func (p *Array[T]) Width() uint {
	return uint(len(p.modules))
}

func (p *Array[T]) String() string {
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

// ----------------------------------------------------------------------------

// ArrayModule describes an individual module within a row-major trace.
type ArrayModule[T any] struct {
	// Holds the name of this module.
	name string
	// Holds the register descriptor for this module.
	descriptor []register
	// Holds the flattened limb descriptor for this module.
	limbs []limb
	// Holds the complete set of rows in this module.
	rows []ArrayRow[T]
}

var _ Module[any] = ArrayModule[any]{}

// NewArrayModule constructs a module with the given name, descriptor and rows.
// This assigns final module-wide limb identifiers and register identifiers to
// the descriptor.
func NewArrayModule[T any](name string, descriptor []Register, rows ...[]T) ArrayModule[T] {
	var (
		registers = make([]register, len(descriptor))
		limbs     []limb
		rowData   = make([]ArrayRow[T], len(rows))
	)
	//
	for rid := range descriptor {
		reg, regLimbs := newRegisterFromDescriptor(descriptor[rid], uint(rid), uint(len(limbs)))
		registers[rid] = reg

		limbs = append(limbs, regLimbs...)
	}
	//
	width := uint(len(limbs))
	//
	for i, row := range rows {
		rowData[i] = NewArrayRow(row)
		//
		if uint(len(row)) != width {
			panic(fmt.Sprintf("invalid row width (have %d, expected %d)", len(row), width))
		}
	}
	//
	return ArrayModule[T]{name, registers, limbs, rowData}
}

// Name returns the name of this module.
func (p ArrayModule[T]) Name() string {
	return p.name
}

// Descriptor returns the registers describing this module.
func (p ArrayModule[T]) Descriptor() iter.Iterator[Register] {
	arr := iter.NewArrayIterator(p.descriptor)
	return iter.NewCastIterator[register, Register](arr)
}

// Limbs returns the limbs describing this module.
func (p ArrayModule[T]) Limbs() iter.Iterator[Limb] {
	arr := iter.NewArrayIterator(p.limbs)
	return iter.NewCastIterator[limb, Limb](arr)
}

// LimbAt returns the limb with the given index.
func (p ArrayModule[T]) LimbAt(index uint) Limb {
	return p.limbs[index]
}

// Row returns a specific row within this trace module.
func (p ArrayModule[T]) Row(id uint) Row[T] {
	return p.rows[id]
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p ArrayModule[T]) Height() uint {
	return uint(len(p.rows))
}

// Width returns the number of columns in this module.
func (p ArrayModule[T]) Width() uint {
	return uint(len(p.limbs))
}

func (p ArrayModule[T]) String() string {
	var id strings.Builder
	//
	if p.name == "" {
		id.WriteString("empty")
	} else {
		id.WriteString(p.name)
	}
	//
	id.WriteString("={")
	//
	for i, r := range p.rows {
		if i != 0 {
			id.WriteString(", ")
		}
		//
		id.WriteString(r.String())
	}
	//
	id.WriteString("}")
	//
	return id.String()
}

// ArrayRow describes an individual row of data within a row-major trace module.
type ArrayRow[T any] struct {
	// Holds the raw data making up this row.
	data []T
}

var _ Row[any] = ArrayRow[any]{}

// NewArrayRow constructs a row from the given data.
func NewArrayRow[T any](data []T) ArrayRow[T] {
	return ArrayRow[T]{data}
}

// Get returns the value at a given column in this row.
func (p ArrayRow[T]) Get(column uint) T {
	return p.data[column]
}

// Width returns the number of columns in this row.
func (p ArrayRow[T]) Width() uint {
	return uint(len(p.data))
}

func (p ArrayRow[T]) String() string {
	var id strings.Builder
	//
	id.WriteString("{")
	//
	for i, v := range p.data {
		if i != 0 {
			id.WriteString(",")
		}
		//
		fmt.Fprintf(&id, "%v", v)
	}
	//
	id.WriteString("}")
	//
	return id.String()
}
