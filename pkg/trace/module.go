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
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Module describes a module within the trace.  Every module is a collection of
// zero or more data columns with the same height.  The width of a module is the
// number of such columns it contains.  Every column in the module has a
// "descriptor" which provides metadata about the columns, such as its name and
// declared bitwidth, etc.
type Module[F field.Element[F]] struct {
	// Recorded height of module
	height uint
	// Holds the descriptor for this module.
	descriptor ModuleDescriptor
	// Holds the complete set of columns in this module, with one for each
	// descriptor.
	columns []array.Array[F]
}

// NewModule constructs a module with the given name, descriptors and rows.
func NewModule[F field.Element[F]](descriptor ModuleDescriptor, data ...array.Array[F]) Module[F] {
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
	return Module[F]{height, descriptor, data}
}

// Expand a given column in this module
func (p Module[F]) Expand(col uint, data array.Array[F]) Module[F] {
	var (
		height  = p.height
		columns = slices.Clone(p.columns)
	)
	// Sanity check
	if columns[col] != nil {
		panic("cannot expand non-empty column")
	} else if p.untouched() {
		// This module has no columns assigned yet (e.g. it is an entirely
		// computed module, such as a lookup table, which has no natural
		// presence in the original trace).  In this case, its recorded height
		// is just a placeholder rather than an established fact, so the first
		// column expanded determines the real height.
		height = data.Len()
	} else if data.Len() != p.height {
		panic(fmt.Sprintf("invalid column height (%d vs %d)", data.Len(), p.height))
	}
	//
	columns[col] = data
	// Done
	return Module[F]{height, p.descriptor, columns}
}

// untouched determines whether or not any column in this module has been
// assigned data yet.
func (p Module[F]) untouched() bool {
	for _, col := range p.columns {
		if col != nil {
			return false
		}
	}
	//
	return true
}

// Name returns the name of this module.
func (p Module[F]) Name() string {
	return p.descriptor.Name
}

// Descriptor returns the descriptor of this module.
func (p Module[F]) Descriptor() ModuleDescriptor {
	return p.descriptor
}

// Column returns the data for the column at the given index.
func (p Module[F]) Column(index uint) array.Array[F] {
	return p.columns[index]
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p Module[F]) Height() uint {
	return p.height
}

// Pad returns a copy of this module with the given amount of front/back
// padding added.  The receiver itself is left unmodified.
func (p Module[F]) Pad(front uint) Module[F] {
	var columns = make([]array.Array[F], len(p.columns))
	//
	for i, col := range p.columns {
		if col != nil {
			columns[i] = col.Pad(front)
		}
	}
	//
	return Module[F]{p.height + front, p.descriptor, columns}
}

// Width returns the number of columns in this module.
func (p Module[F]) Width() uint {
	return uint(len(p.descriptor.Columns))
}

func (p Module[F]) String() string {
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
