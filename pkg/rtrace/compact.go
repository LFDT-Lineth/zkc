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
	"slices"
	"strings"

	ctrace "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/narray"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// CompactModule describes an individual module within a trace, and represents
// each register within that module using a column with an appropriate (compact)
// encoding.
type CompactModule[F field.Element[F]] struct {
	// Holds the name of this module.
	name string
	// Holds the register registers for this module.
	registers []Register
	// Holds the complete set of columns in this module, with one for each
	// declared limb.
	columns []narray.MutArray[F]
}

// NewCompactModule constructs a module with the given name, descriptor and rows.
// This assigns final module-wide limb identifiers and register identifiers to
// the descriptor.
func NewCompactModule[F field.Element[F]](name string, descriptor []Register, rows ...[]F) *CompactModule[F] {
	var (
		registers = slices.Clone(descriptor)
		height    = uint(len(rows))
		width     = uint(len(descriptor))
		columns   = make([]narray.MutArray[F], width)
	)
	//
	for rid := range descriptor {
		var bitwidth = descriptor[rid].Bitwidth.UnwrapOr(math.MaxUint)
		// Allocate compact representation
		columns[rid] = allocArray[F](bitwidth, height)
	}
	// Initialise given rows
	for i, row := range rows {
		if uint(len(row)) != width {
			panic(fmt.Sprintf("invalid row width (have %d, expected %d)", len(row), width))
		}
		//
		for j, w := range row {
			columns[j].Set(uint(i), w)
		}
	}
	//
	return &CompactModule[F]{name, registers, columns}
}

// Initialise implementation for Module interface.
func (p *CompactModule[T]) Initialise(name string, registers []Register, rows ...[]T) *CompactModule[T] {
	return NewCompactModule(name, registers, rows...)
}

// Append implementation for Module interface.
func (p *CompactModule[T]) Append(row ...T) {
	if len(row) != len(p.registers) {
		panic("mismatched row data")
	}
	// Append element for each row
	for i, v := range row {
		p.columns[i].Append(v)
	}
}

// Name returns the name of this module.
func (p *CompactModule[T]) Name() string {
	return p.name
}

// Descriptor returns the registers describing this module.
func (p *CompactModule[T]) Descriptor() iter.Iterator[Register] {
	return iter.NewArrayIterator(p.registers)
}

// RegisterAt returns the limb with the given index.
func (p *CompactModule[T]) RegisterAt(index uint) Register {
	return p.registers[index]
}

// Row returns a specific row within this trace module.
func (p *CompactModule[T]) Row(id uint) Row[T] {
	return &compactRow[T]{p.columns, id}
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p *CompactModule[T]) Height() uint {
	if len(p.columns) == 0 {
		return 0
	}
	//
	return p.columns[0].Len()
}

// Width returns the number of columns in this module.
func (p *CompactModule[T]) Width() uint {
	return uint(len(p.registers))
}

func (p *CompactModule[T]) String() string {
	var id strings.Builder
	//
	if p.name == "" {
		id.WriteString("∅")
	} else {
		id.WriteString(p.name)
	}

	id.WriteString("={")
	//
	for i, c := range p.columns {
		if i != 0 {
			id.WriteString(", ")
		}
		//
		id.WriteString(c.String())
	}
	//
	id.WriteString("}")
	// Done
	return id.String()
}

// ToLtModule implementation for Module interface.
func (p *CompactModule[T]) ToLtModule() lt.Module[T] {
	var (
		name    = ctrace.ModuleName{Name: p.name, Multiplier: 1}
		columns = make([]lt.Column[T], len(p.columns))
	)
	//
	for i, col := range p.columns {
		var (
			data = col.ToLegacy()
			name = p.registers[i].Name
		)
		//
		columns[i] = lt.NewColumn(name, data)
	}
	//
	return lt.NewModule(name, columns)
}

type compactRow[T any] struct {
	columns []narray.MutArray[T]
	// row index
	row uint
}

// Get the value in a given column of this row.
func (p compactRow[T]) Get(column uint) T {
	return p.columns[column].Get(p.row)
}

// Returns the number of columns in this row.
func (p compactRow[T]) Width() uint {
	return uint(len(p.columns))
}

func allocArray[F field.Element[F]](bitwidth uint, height uint) narray.MutArray[F] {
	var (
		zero  F
		width = zero.Modulus().BitLen()
	)
	// Construct column
	switch {
	case bitwidth == 0:
		return narray.NewConstantArray(height, 0, zero)
	case bitwidth == 1:
		var x = narray.NewBitArray[F](height)
		return &x
	case bitwidth <= 8 && width >= 8:
		var x = narray.NewSmallArray[uint8, F](height, bitwidth)
		return &x
	case bitwidth <= 16 && width >= 16:
		var x = narray.NewSmallArray[uint16, F](height, bitwidth)
		return &x
	case bitwidth <= 32 && width >= 32:
		var x = narray.NewSmallArray[uint32, F](height, bitwidth)
		return &x
	case bitwidth <= 64 && width >= 64:
		var x = narray.NewSmallArray[uint64, F](height, bitwidth)
		return &x
	default:
		return narray.NewStaticArray[F](height, bitwidth)
	}
}
