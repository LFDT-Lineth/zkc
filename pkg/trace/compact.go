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
	"fmt"
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
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
	columns []array.MutArray[F]
}

// NewCompactModule constructs a module with the given name, descriptors and rows.
func NewCompactModule[F field.Element[F]](descriptor ModuleDescriptor, data ...array.MutArray[F]) *CompactModule[F] {
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

// InitCompactModule constructs a new empty module with appropriately allocated
// (but zero-height) columns for each descriptor.
func InitCompactModule[F field.Element[F]](descriptor ModuleDescriptor) *CompactModule[F] {
	var (
		width   = descriptor.Width()
		columns = make([]array.MutArray[F], width)
	)
	//
	for rid := range descriptor.Columns {
		var bitwidth = descriptor.Columns[rid].Bitwidth.UnwrapOr(math.MaxUint)
		// Allocate compact representation
		columns[rid] = array.Alloc[F](bitwidth, 0)
	}
	//
	return &CompactModule[F]{0, descriptor, columns}
}

// Initialise implementation for ModuleBuilder interface.  This constructs a new
// empty module; the receiver is not used.
func (p *CompactModule[F]) Initialise(descriptor ModuleDescriptor) *CompactModule[F] {
	return InitCompactModule[F](descriptor)
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
func (p *CompactModule[F]) Expand(col uint, data array.MutArray[F]) {
	if p.columns[col] != nil {
		panic("cannot expand non-empty column")
	} else if p.untouched() {
		// This module has no columns assigned yet (e.g. it is an entirely
		// computed module, such as a lookup table, which has no natural
		// presence in the original trace).  In this case, its recorded height
		// is just a placeholder rather than an established fact, so the first
		// column expanded determines the real height.
		p.height = data.Len()
	} else if data.Len() != p.height {
		panic(fmt.Sprintf("invalid column height (%d vs %d)", data.Len(), p.height))
	}
	//
	p.columns[col] = data
}

// untouched determines whether or not any column in this module has been
// assigned data yet.
func (p *CompactModule[F]) untouched() bool {
	for _, col := range p.columns {
		if col != nil {
			return false
		}
	}
	//
	return true
}

// Join a given module into this by appending all rows of each column onto the
// corresponding column in this module.
func (p *CompactModule[F]) Join(m Module[F]) {
	if p.Width() != m.Width() {
		panic(fmt.Sprintf("cannot join mismatched modules '%s' (%d columns) vs '%s' (%d columns)",
			p.descriptor.Name, p.Width(), m.Descriptor().Name, m.Width()))
	}
	//
	for i := range p.Width() {
		array.AppendOnto(p.columns[i], m.Column(i))
	}
	// Increment height
	p.height += m.Height()
}

// Clone this module, such that mutating the clone (or the original)
// afterwards has no effect on the other.
func (p *CompactModule[F]) Clone() *CompactModule[F] {
	columns := make([]array.MutArray[F], len(p.columns))
	//
	for i, col := range p.columns {
		if col != nil {
			columns[i] = col.Clone()
		}
	}
	//
	return &CompactModule[F]{p.height, p.descriptor, columns}
}

// Name returns the name of this module.
func (p *CompactModule[F]) Name() string {
	return p.descriptor.Name
}

// Descriptor returns the descriptor of this module.
func (p *CompactModule[F]) Descriptor() ModuleDescriptor {
	return p.descriptor
}

// Column returns the data for the column at the given index.
func (p *CompactModule[F]) Column(index uint) array.Array[F] {
	return p.columns[index]
}

// MutColumn returns mutable access to the data for the column at the given
// index.
func (p *CompactModule[F]) MutColumn(index uint) array.MutArray[F] {
	return p.columns[index]
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p *CompactModule[F]) Height() uint {
	return p.height
}

// Pad returns a copy of this module with the given amount of front/back
// padding added.  The receiver itself is left unmodified.
func (p *CompactModule[F]) Pad(front, back uint) *CompactModule[F] {
	var (
		zero    F
		columns = make([]array.MutArray[F], len(p.columns))
	)
	//
	for i, col := range p.columns {
		if col != nil {
			columns[i] = col.Pad(front, back, zero)
		}
	}
	//
	return &CompactModule[F]{p.height + front + back, p.descriptor, columns}
}

// Width returns the number of columns in this module.
func (p *CompactModule[F]) Width() uint {
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
