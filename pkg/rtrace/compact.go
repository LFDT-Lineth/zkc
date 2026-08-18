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
	// Recorded height of module
	height uint
	// Holds the descriptor for this module.
	descriptor ModuleDescriptor
	// Holds the complete set of columns in this module, with one for each
	// descriptor.
	columns []narray.MutArray[F]
}

// NewCompactModule constructs a module with the given name, descriptors and rows.
func NewCompactModule[F field.Element[F]](descriptor ModuleDescriptor, data ...narray.MutArray[F]) *CompactModule[F] {
	var height uint
	// Sanity check
	if uint(len(data)) != descriptor.Width() {
		panic(fmt.Sprintf("incorrect number of data columns for module '%s' (%d vs %d)",
			descriptor.Name, len(data), descriptor.Width()))
	}
	// Determine maximum height
	for _, col := range data {
		if col != nil {
			height = max(height, col.Len())
		}
	}
	// Check matching heights
	for i, col := range data {
		if col != nil && col.Len() != height {
			panic(fmt.Sprintf("column %s has mismatched height (%d vs %d)",
				descriptor.Columns[i].Name, col.Len(), height))
		}
	}
	//
	return &CompactModule[F]{height, descriptor, data}
}

// Initialise implementation for Module interface.
func (p *CompactModule[F]) Initialise(descriptor ModuleDescriptor) *CompactModule[F] {
	var (
		width   = descriptor.Width()
		columns = make([]narray.MutArray[F], width)
	)
	//
	for rid := range descriptor.Columns {
		var bitwidth = descriptor.Columns[rid].Bitwidth.UnwrapOr(math.MaxUint)
		// Allocate compact representation
		columns[rid] = narray.Alloc[F](bitwidth, 0)
	}
	//
	return &CompactModule[F]{0, descriptor, columns}
}

// Append implementation for Module interface.
func (p *CompactModule[F]) Append(row ...F) {
	if len(row) != len(p.descriptor.Columns) {
		panic("mismatched row data")
	}
	// Append element for each row
	for i, v := range row {
		p.columns[i].Append(v)
	}
	// Increment height
	p.height++
}

// Expand a given column in this module
func (p *CompactModule[F]) Expand(col uint, data narray.MutArray[F]) {
	if p.columns[col] != nil {
		panic("cannot expand non-empty column")
	} else if data.Len() != p.height {
		panic(fmt.Sprintf("invalid column height (%d vs %d)", data.Len(), p.height))
	}
	//
	p.columns[col] = data
}

// Join a given module into this by appending all rows of each column onto the
// corresponding column in this module.
func (p *CompactModule[T]) Join(m Module[T]) {
	if p.Width() != m.Width() {
		panic(fmt.Sprintf("cannot join mismatched modules '%s' (%d columns) vs '%s' (%d columns)",
			p.descriptor.Name, p.Width(), m.Descriptor().Name, m.Width()))
	}
	//
	for i := range p.Width() {
		narray.AppendOnto(p.columns[i], m.Column(i))
	}
	// Increment height
	p.height += m.Height()
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
func (p *CompactModule[F]) Column(index uint) narray.Array[F] {
	return p.columns[index]
}

// MutColumn returns mutable access to the data for the column at the given
// index.
func (p *CompactModule[F]) MutColumn(index uint) narray.MutArray[F] {
	return p.columns[index]
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p *CompactModule[F]) Height() uint {
	return p.height
}

// Pad this module by a given amount of front/back padding
func (p *CompactModule[F]) Pad(front, back uint) {
	var zero F
	//
	for i := range p.columns {
		if p.columns[i] != nil {
			p.columns[i].Pad(front, back, zero)
		}
	}
	// Increment height
	p.height += front + back
}

// Width returns the number of columns in this module.
func (p *CompactModule[T]) Width() uint {
	return uint(len(p.descriptor.Columns))
}

func (p *CompactModule[F]) String() string {
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
