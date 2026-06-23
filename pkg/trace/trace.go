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
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Trace describes a set of named columns.  Columns are not required to have the
// same height and can be either "data" columns or "computed" columns.
type Trace[F any] interface {
	// Provides agnostic mechanism for constructing arrays
	Builder() array.Builder[F]
	// Access a given column difrtly via a reference.
	Column(ColumnRef) Column[F]
	// Determine whether this trace has a module with the given name and, if so,
	// what its module index is.
	HasModule(name ModuleName) (uint, bool)
	// Access a given module in this trace.
	Module(ModuleId) Module[F]
	// Returns an iterator over the contained modules
	Modules() iter.Iterator[Module[F]]
	// Returns the number of modules in this trace.
	Width() uint
}

// Module describes a module within the trace.  Every module is composed of some
// number of columns, and has a specific height.
type Module[T any] interface {
	// Module name
	Name() ModuleName
	// Access a given column in this module.
	Column(uint) Column[T]
	// Access a given column by its name.
	ColumnOf(string) Column[T]
	// Keys returns the number n of key columns in this module.  Key columns are
	// always the first n columns in a module.  Such columns have the property
	// that they can be used in conjunction with Find.
	Keys() uint
	// FindLast finds the last row with matching keys.  If no such row exists,
	// this returns math.MaxUint.  Otherwise, it returns the last (i.e. highest)
	// matching row.  Furthermore, if too few or too many keys are provided then
	// this will panic.
	FindLast(...T) uint
	// Returns the number of columns in this module.
	Width() uint
	// Returns the height of this module.
	Height() uint
}

// Column describes an individual column of data within a trace table.
type Column[T any] interface {
	// Holds the name of this column
	Name() string
	// Get the value at a given row in this column.  If the row is
	// out-of-bounds, then the column's padding value is returned instead.
	// Thus, this function always succeeds.
	Get(row int) T
	// Access the underlying data array for this column.  This is useful in
	// situations where we want to clone the entire column, etc.
	Data() array.Array[T]
	// Padding returns the value which will be used for padding this column.
	Padding() T
}

// ModuleName abstracts the notion of a module name, since this is made up of
// two distinct components.
type ModuleName struct {
	// Name of the module (excluding multiplier)
	Name string
	// Multiplier for the module
	Multiplier uint
}

// Cmp two module names
func (p ModuleName) Cmp(q ModuleName) int {
	if c := strings.Compare(p.Name, q.Name); c != 0 {
		return c
	}
	//
	return cmp.Compare(p.Multiplier, q.Multiplier)
}

func (p ModuleName) String() string {
	if p.Multiplier == 1 {
		return p.Name
	}
	//
	return fmt.Sprintf("%s×%d", p.Name, p.Multiplier)
}

// ParseModuleName parses a string formatted as a module name into a ModuleName
// structure.  This will panic if the string is malformed.
func ParseModuleName(name string) ModuleName {
	var (
		splits     = strings.Split(name, "×")
		multiplier uint
	)
	//
	switch {
	case len(splits) == 1:
		multiplier = 1
	case len(splits) == 2:
		tmp, err := strconv.Atoi(splits[1])
		//
		if err != nil {
			panic(err.Error())
		} else if tmp <= 0 {
			panic(fmt.Sprintf("invalid module name \"%s\"", name))
		}
		//
		multiplier = uint(tmp)
	default:
		panic(fmt.Sprintf("invalid module name \"%s\"", name))
	}
	// Done
	return ModuleName{splits[0], multiplier}
}

// Module2String returns a string representation of a module which is primiarily
// useful for debugging.
func Module2String[F fmt.Stringer](module Module[F]) string {
	var builder strings.Builder
	//
	builder.WriteString(module.Name().String())
	builder.WriteString("=>")
	//
	for i := range module.Width() {
		if i != 0 {
			builder.WriteString(";")
		}
		//
		builder.WriteString(Column2String(module.Column(i)))
	}
	//
	return builder.String()
}

// Column2String returns a string representation of a column which is primiarily
// useful for debugging.
func Column2String[F fmt.Stringer](col Column[F]) string {
	var (
		builder strings.Builder
		data    = col.Data()
	)
	//
	builder.WriteString(col.Name())
	builder.WriteString(":")
	//
	if data == nil {
		builder.WriteString("∅")
	} else {
		builder.WriteString("[")
		//
		for i := range data.Len() {
			if i != 0 {
				builder.WriteString(",")
			}
			//
			builder.WriteString(data.Get(i).String())
		}
		//
		builder.WriteString("]")
	}
	//
	return builder.String()
}
