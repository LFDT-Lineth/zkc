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
	"sort"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/word"
)

// ArrayTrace provides an implementation of Trace which stores columns as an
// array.
type ArrayTrace[W word.Word[W]] struct {
	// Internal memory pool
	builder array.Builder[W]
	// Holds the height of each module in this trace.  The index of each
	// module in this array uniquely identifies it, and is referred to as the
	// "module index".
	modules []ArrayModule[W]
}

// NewArrayTrace constructs a trace from a given set of indexed modules and columns.
func NewArrayTrace[W word.Word[W]](builder array.Builder[W], modules []ArrayModule[W]) *ArrayTrace[W] {
	return &ArrayTrace[W]{builder, modules}
}

// Column accesses a given column directly via a reference.
func (p *ArrayTrace[W]) Column(cref ColumnRef) Column[W] {
	return p.modules[cref.Module()].Column(cref.Column().index)
}

// HasModule determines whether this trace has a module with the given name and,
// if so, what its module index is.
func (p *ArrayTrace[W]) HasModule(module ModuleName) (uint, bool) {
	// Linea scan through list of modules
	for mid, mod := range p.modules {
		if mod.name == module {
			return uint(mid), true
		}
	}
	//
	return math.MaxUint, false
}

// Module returns a specific module in this trace.
func (p *ArrayTrace[W]) Module(module uint) Module[W] {
	return p.modules[module]
}

// RawModule returns a specific module in this trace.
func (p *ArrayTrace[W]) RawModule(module uint) *ArrayModule[W] {
	return &p.modules[module]
}

// Modules returns an iterator over the modules in this trace.
func (p *ArrayTrace[W]) Modules() iter.Iterator[Module[W]] {
	arr := iter.NewArrayIterator(p.modules)
	return iter.NewCastIterator[ArrayModule[W], Module[W]](arr)
}

// Width returns number of columns in this trace.
func (p *ArrayTrace[W]) Width() uint {
	return uint(len(p.modules))
}

// Pad prepends (front) and appends (back) a given module with a given number of
// padding rows.
func (p *ArrayTrace[W]) Pad(module uint, front uint, back uint) {
	p.modules[module].Pad(front, back)
}

// Builder implementation for Trace interface.
func (p *ArrayTrace[W]) Builder() array.Builder[W] {
	return p.builder
}

func (p *ArrayTrace[W]) String() string {
	// Use string builder to try and make this vaguely efficient.
	var id strings.Builder

	id.WriteString("{")
	// Write each module in turn
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

// ArrayModule describes an individual module within a trace.
type ArrayModule[W word.Word[W]] struct {
	// Holds the name of this module
	name ModuleName
	// Holds the height of all columns within this module.
	height uint
	// Number of keys which can be used for searching in this module.  Cannot
	// exceed the number of columns, but it can be zero.
	numKeys uint
	// Holds the complete set of columns in this trace.  The index of each
	// column in this array uniquely identifies it, and is referred to as the
	// "column index".
	columns []ArrayColumn[W]
}

// NewArrayModule constructs a module with the given name and an (as yet)
// unspecified height.
func NewArrayModule[W word.Word[W]](name ModuleName, keys uint, columns []ArrayColumn[W]) ArrayModule[W] {
	var (
		height uint = 0
		first       = true
	)
	//
	for _, c := range columns {
		if first && c.data != nil {
			height = c.Height()
			first = false
		} else if c.data != nil && height != c.Height() {
			// NOTE: we ignore nil columns and assume they are computed columns
			// which are yet to be filled.
			panic(fmt.Sprintf("invalid column height (have %d, expected %d)", c.Height(), height))
		}
	}
	// Sanity check height is a multiple of the length multiplier
	if name.Multiplier == 0 {
		panic(fmt.Sprintf("invalid length multiplier (%d)", name.Multiplier))
	} else if height%name.Multiplier != 0 {
		panic(fmt.Sprintf("invalid module height (have %d, expected multiple of %d)", height, name.Multiplier))
	}
	//
	return ArrayModule[W]{name, height, keys, columns}
}

// Name returns the name of this module.
func (p ArrayModule[W]) Name() ModuleName {
	return p.name
}

// Column returns a specific column within this trace module.
func (p ArrayModule[W]) Column(id uint) Column[W] {
	return &p.columns[id]
}

// ColumnOf returns a specific column within this trace module.
func (p ArrayModule[W]) ColumnOf(name string) Column[W] {
	for _, c := range p.columns {
		if c.name == name {
			return &c
		}
	}
	//
	panic(fmt.Sprintf("unknown column \"%s\"", name))
}

// FindLast implementation for the trace.Module interface.
func (p ArrayModule[W]) FindLast(keys ...W) uint {
	if uint(len(keys)) != p.numKeys {
		panic(fmt.Sprintf("incorrect number of keys provided (was %d, expected %d)", len(keys), p.numKeys))
	}
	// Search for given keys
	index := sort.Search(int(p.height), func(row int) bool {
		return rowLessThanEquals(keys, uint(row), p.columns)
	})
	// Sanity check result
	if uint(index) < p.height && rowEquals(uint(index), p.columns, keys) {
		// Shift along to last index
		return findLastFrom(uint(index), p.height, p.columns, keys)
	}
	//
	return math.MaxUint
}

// Keys implementation for the trace.Module interface.
func (p ArrayModule[W]) Keys() uint {
	return p.numKeys
}

// Height returns the height of this module, meaning the number of assigned
// rows.
func (p ArrayModule[W]) Height() uint {
	return p.height
}

// Width returns the number of columns in this module.
func (p ArrayModule[W]) Width() uint {
	return uint(len(p.columns))
}

// FillColumn sets the data and padding for the given column.  This will panic
// if the data is already set.  Also, if the module height is updated then this
// returns true to signal a height recalculation is required.
func (p *ArrayModule[W]) FillColumn(cid uint, data array.MutArray[W]) bool {
	// Find column to fill
	col := &p.columns[cid]
	// Sanity check this column has not already been filled.
	if p.height == math.MaxUint {
		// Initialise column height
		p.height = data.Len()
	} else if data.Len() != p.Height() {
		// The height of this module maybe changing as a result of this
		// operation.  Therefore, we temporarily set it to an invalid value
		// under the expectation at the height will be subsequently recalculated.
		p.height = math.MaxUint
	}
	// Fill the column
	col.data = data
	//
	return p.height == math.MaxUint
}

// Resize the height of this module on the assumption it has been reset whilst
// filling a column (as above).  This will panic if either: the module height
// was not previously reset; or, if the column heights are inconsistent.
func (p *ArrayModule[W]) Resize() {
	var (
		multiplier      = p.name.Multiplier
		nsize      uint = math.MaxUint
		first      bool = true
	)
	//
	for i := 0; i != len(p.columns); i++ {
		data := p.columns[i].Data()
		//
		if data == nil {
			// skip
		} else if first {
			nsize = data.Len()
			first = false
		} else if nsize != data.Len() {
			panic(fmt.Sprintf("incompatible column heights (%d vs %d)", nsize, data.Len()))
		}
	}
	// Sanity check height is a multiple of the length multiplier
	if nsize%multiplier != 0 {
		panic(fmt.Sprintf("invalid module height (have %d, expected multiple of %d)", nsize, multiplier))
	}
	// Done
	p.height = nsize
}

// Pad prepends (front) and appends (back) all columns in this module with a
// given number of padding rows.
func (p *ArrayModule[W]) Pad(front uint, back uint) {
	var (
		multiplier = p.name.Multiplier
	)
	//
	if front%multiplier != 0 {
		panic(fmt.Sprintf("invalid front padding (have %d, expected multiple of %d)", front, multiplier))
	} else if back%multiplier != 0 {
		panic(fmt.Sprintf("invalid back padding (have %d, expected multiple of %d)", back, multiplier))
	}
	// Update height accordingly
	p.height += front + back
	// Padd each column contained within this module.
	for i := 0; i < len(p.columns); i++ {
		p.columns[i].pad(front, back)
	}
}

func (p *ArrayModule[W]) String() string {
	var (
		id   strings.Builder
		name = p.name.String()
	)
	//
	if name == "" {
		id.WriteString("∅")
	} else {
		id.WriteString(name)
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

// ----------------------------------------------------------------------------

// ArrayColumn describes an individual column of data within a trace table.
type ArrayColumn[T any] struct {
	// Holds the name of this column
	name string
	// Holds the raw data making up this column
	data array.MutArray[T]
	// Value to be used when padding this column
	padding T
}

// NewArrayColumn constructs a with the give name, data and padding.  The given
// data is permitted to be nil, and this is used to signal a computed column.
func NewArrayColumn[W fmt.Stringer](name string, data array.MutArray[W], padding W) ArrayColumn[W] {
	return ArrayColumn[W]{name, data, padding}
}

// Name returns the name of the given column.
func (p *ArrayColumn[W]) Name() string {
	return p.name
}

// Height determines the height of this column.
func (p *ArrayColumn[W]) Height() uint {
	return p.data.Len()
}

// Padding returns the value which will be used for padding this column.
func (p *ArrayColumn[W]) Padding() W {
	return p.padding
}

// Data provides access to the underlying data of this column
func (p *ArrayColumn[W]) Data() array.Array[W] {
	return p.data
}

// Get the value at a given row in this column.  If the row is
// out-of-bounds, then the column's padding value is returned instead.
// Thus, this function always succeeds.
func (p *ArrayColumn[W]) Get(row int) W {
	if row < 0 || uint(row) >= p.data.Len() {
		// out-of-bounds access
		return p.padding
	}
	// in-bounds access
	return p.data.Get(uint(row))
}

func (p *ArrayColumn[W]) String() string {
	var id strings.Builder
	// Write column name
	id.WriteString(p.name)
	//
	if p.data == nil {
		id.WriteString("=⊥")
	} else {
		id.WriteString("={")
		// Print out each element
		for j := uint(0); j < p.Height(); j++ {
			jth := p.Get(int(j))

			if j != 0 {
				id.WriteString(",")
			}

			fmt.Fprintf(&id, "%v", jth)
		}
		//
		id.WriteString("}")
	}
	//
	return id.String()
}

func (p *ArrayColumn[W]) pad(front uint, back uint) {
	if p.data != nil {
		// Pad front of array
		p.data = p.data.Pad(front, back, p.padding)
	}
}

func findLastFrom[W word.Word[W]](row, height uint, columns []ArrayColumn[W], keys []W) uint {
	for (row+1) < height && rowEquals(row+1, columns, keys) {
		row = row + 1
	}
	//
	return row
}

func rowLessThanEquals[W word.Word[W]](keys []W, row uint, columns []ArrayColumn[W]) bool {
	var n = len(keys)
	// NOTE: since register limbs are split with the least significant having
	// the lowest index, they are sorted in a right-to-left fashion by
	// io.Executor.  This ensures that, after register splitting, all instances
	// remain in sorted order.  Therefore, we have to reflect this here.
	for k := n; k > 0; {
		k--
		//
		kth := columns[k].data.Get(row)
		//
		if c := keys[k].Cmp(kth); c > 0 {
			// miss
			return false
		} else if c < 0 {
			// hit
			return true
		}
	}
	//
	return true
}

func rowEquals[W word.Word[W]](row uint, columns []ArrayColumn[W], keys []W) bool {
	var n = len(keys)
	//
	for k := range n {
		kth := columns[k].data.Get(row)
		if !kth.Equals(keys[k]) {
			// miss
			return false
		}
	}
	//
	return true
}
