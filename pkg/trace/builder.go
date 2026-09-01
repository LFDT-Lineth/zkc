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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ModuleBuilder describes an individual module within a trace, and represents
// each column within that module using an appropriate (compact) encoding.
type ModuleBuilder[F field.Element[F]] struct {
	// Recorded height of module
	height uint
	// Holds the descriptor for this module.
	descriptor ModuleDescriptor
	// Holds the complete set of columns in this module, with one for each
	// descriptor.
	columns []array.MutArray[F]
}

// InitModuleBuilder constructs a new empty module with appropriately allocated
// (but zero-height) columns for each descriptor.
func InitModuleBuilder[F field.Element[F]](descriptor ModuleDescriptor) *ModuleBuilder[F] {
	var (
		width   = descriptor.Width()
		columns = make([]array.MutArray[F], width)
	)
	//
	for rid := range descriptor.Columns {
		var bitwidth = descriptor.Columns[rid].Bitwidth.UnwrapOr(math.MaxUint)
		// Allocate compact representation
		columns[rid] = array.Alloc[F](bitwidth)
	}
	//
	return &ModuleBuilder[F]{0, descriptor, columns}
}

// Build constructs a module from this builder.
func (p *ModuleBuilder[F]) Build() Module[F] {
	var columns = make([]array.Array[F], len(p.columns))
	//
	for i, col := range p.columns {
		columns[i] = col.Build()
	}
	//
	return NewModule(p.descriptor, columns...)
}

// Append implementation for Module interface.
func (p *ModuleBuilder[F]) Append(row ...F) {
	if len(row) != len(p.descriptor.Columns) {
		panic("mismatched row data")
	}
	// Append element for each row
	for i, v := range row {
		p.columns[i] = p.columns[i].Append(v)
	}
	// Increment height
	p.height++
}

// Name returns the name of this module.
func (p *ModuleBuilder[F]) Name() string {
	return p.descriptor.Name
}

// Descriptor returns the descriptor of this module.
func (p *ModuleBuilder[F]) Descriptor() ModuleDescriptor {
	return p.descriptor
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p *ModuleBuilder[F]) Height() uint {
	return p.height
}

// Width returns the number of columns in this module.
func (p *ModuleBuilder[F]) Width() uint {
	return uint(len(p.descriptor.Columns))
}

func (p *ModuleBuilder[F]) String() string {
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
		id.WriteString(c.Build().String())
	}
	//
	id.WriteString("}")
	// Done
	return id.String()
}
