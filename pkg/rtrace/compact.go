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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/narray"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// CompactModule describes an individual module within a trace, and represents
// each column within that module using an appropriate (compact) encoding.
type CompactModule[F field.Element[F]] struct {
	// Holds the descriptor for this module.
	descriptor ModuleDescriptor
	// Holds the complete set of columns in this module, with one for each
	// descriptor.
	columns []narray.MutArray[F]
}

// NewCompactModule constructs a module with the given name, descriptors and rows.
func NewCompactModule[F field.Element[F]](descriptor ModuleDescriptor, rows ...[]F) *CompactModule[F] {
	var (
		height  = uint(len(rows))
		width   = uint(len(descriptor.Columns))
		columns = make([]narray.MutArray[F], width)
	)
	//
	for rid := range descriptor.Columns {
		var bitwidth = descriptor.Columns[rid].Bitwidth.UnwrapOr(math.MaxUint)
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
	return &CompactModule[F]{descriptor, columns}
}

// Initialise implementation for Module interface.
func (p *CompactModule[T]) Initialise(descriptor ModuleDescriptor, rows ...[]T) *CompactModule[T] {
	return NewCompactModule(descriptor, rows...)
}

// Append implementation for Module interface.
func (p *CompactModule[T]) Append(row ...T) {
	if len(row) != len(p.descriptor.Columns) {
		panic("mismatched row data")
	}
	// Append element for each row
	for i, v := range row {
		p.columns[i].Append(v)
	}
}

// Join a given module into this by appending all rows of each column onto the
// corresponding column in this module.
func (p *CompactModule[T]) Join(m Module[T]) {
	if p.Width() != m.Width() {
		panic("cannot join mismatched modules")
	}
	//
	for i := range p.Width() {
		narray.AppendOnto(p.columns[i], m.Column(i))
	}
}

// Name returns the name of this module.
func (p *CompactModule[T]) Name() string {
	return p.descriptor.Name
}

// Descriptor returns the descriptor of this module.
func (p *CompactModule[T]) Descriptor() ModuleDescriptor {
	return p.descriptor
}

// Column returns the data for the column at the given index.
func (p *CompactModule[T]) Column(index uint) narray.Array[T] {
	return p.columns[index]
}

// MutColumn returns mutable access to the data for the column at the given
// index.
func (p *CompactModule[T]) MutColumn(index uint) narray.MutArray[T] {
	return p.columns[index]
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
	return uint(len(p.descriptor.Columns))
}

func (p *CompactModule[T]) String() string {
	var id strings.Builder
	//
	if p.descriptor.Name == "" {
		id.WriteString("∅")
	} else {
		id.WriteString(p.descriptor.Name)
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
		return narray.NewBitArray[F](height)
	case bitwidth <= 8 && width >= 8:
		return narray.NewSmallArray[uint8, F](height, bitwidth)
	case bitwidth <= 16 && width >= 16:
		return narray.NewSmallArray[uint16, F](height, bitwidth)
	case bitwidth <= 32 && width >= 32:
		return narray.NewSmallArray[uint32, F](height, bitwidth)
	case bitwidth <= 64 && width >= 64:
		return narray.NewSmallArray[uint64, F](height, bitwidth)
	default:
		return narray.NewStaticArray[F](height, bitwidth)
	}
}
