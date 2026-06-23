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
package view

import (
	"math"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
)

// TraceFilter is used to filter a given TraceView to focus on key aspects of the
// view, as required for the task at hand.
type TraceFilter interface {
	// Determine filter for columns in the given module; if nil is returned,
	// then module is ignored.
	Module(sc.ModuleId) ModuleFilter
}

// ModuleFilter is used to focus on a subset of columns in a given module.
type ModuleFilter interface {
	// Determine filter for cells of this column; if nil is returned, then
	// column is ignored.
	Column(SourceColumn) bool
	// Window specifies a specific viewport to use.
	Range() (start, end uint)
}

// ============================================================================
// Default Filter
// ============================================================================

type defaultFilter struct{}

// DefaultTraceFilter constructs a default filter which filters nothing.
func DefaultTraceFilter() TraceFilter {
	return &defaultFilter{}
}

// DefaultModuleFilter constructs a default column filter which filters nothing.
func DefaultModuleFilter() ModuleFilter {
	return &defaultFilter{}
}

func (p *defaultFilter) Module(sc.ModuleId) ModuleFilter {
	return p
}

func (p *defaultFilter) Column(SourceColumn) bool {
	return true
}

func (p *defaultFilter) Range() (start, end uint) {
	return 0, math.MaxUint
}

// ============================================================================
// Trace Filter
// ============================================================================

// NewTraceFilter constructs a filter from a given predicate.
func NewTraceFilter(fn func(sc.ModuleId) ModuleFilter) TraceFilter {
	return &traceFilter{fn}
}

type traceFilter struct {
	fn func(sc.ModuleId) ModuleFilter
}

func (p *traceFilter) Module(mid sc.ModuleId) ModuleFilter {
	if f := p.fn(mid); f != nil {
		return f
	}
	//
	return nil
}

// ============================================================================
// Module Filter
// ============================================================================

// NewModuleFilter constructs a filter from a given predicate.
func NewModuleFilter(start, end uint, filter func(SourceColumn) bool) ModuleFilter {
	return &moduleFilter{start, end, filter}
}

type moduleFilter struct {
	start, end uint
	filter     func(SourceColumn) bool
}

// Range implementation for ModuleFilter interface
func (p *moduleFilter) Range() (uint, uint) {
	return p.start, p.end
}

// Column implementation for ModuleFilter interface
func (p *moduleFilter) Column(col SourceColumn) bool {
	return p.filter(col)
}

// ============================================================================
// Cell Filter
// ============================================================================

// FilterForCells returns a filter which focuses specifically on the given
// cells.
func FilterForCells(cells CellRefSet, padding uint) TraceFilter {
	var filter = traceCellFilter{padding, nil}
	//
	for iter := cells.Iter(); iter.HasNext(); {
		filter.addCell(iter.Next())
	}
	//
	return &filter
}

type traceCellFilter struct {
	padding uint
	modules []moduleCellFilter
}

type moduleCellFilter struct {
	padding    uint
	columns    []bit.Set
	start, end uint
}

func (p *traceCellFilter) Module(mid sc.ModuleId) ModuleFilter {
	var n = uint(len(p.modules))
	//
	if n <= mid || len(p.modules[mid].columns) == 0 {
		return nil
	}
	//
	return &p.modules[mid]
}

func (p *traceCellFilter) addCell(cell tr.CellRef) {
	var (
		m   = cell.Column.Module()
		n   = cell.Column.Register().Unwrap()
		mod moduleCellFilter
	)
	// First ensure enough modules
	if uint(len(p.modules)) <= m {
		q := make([]moduleCellFilter, m+1)
		copy(q, p.modules)
		p.modules = q
	}
	//
	mod = p.modules[m]
	// Second ensure enough columns
	if uint(len(mod.columns)) <= n {
		q := make([]bit.Set, n+1)
		copy(q, mod.columns)
		mod.padding = p.padding
		mod.columns = q
		mod.start = math.MaxUint
		mod.end = 0
	}
	// Finally, add the cell
	mod.columns[n].Insert(uint(cell.Row))
	mod.start = min(mod.start, uint(cell.Row))
	mod.end = max(mod.end, uint(cell.Row)+1)
	//
	p.modules[m] = mod
}

func (p *moduleCellFilter) Column(col SourceColumn) bool {
	// Look to see whether any limb is in this column, or not.
	for _, lid := range col.Limbs {
		if uint(len(p.columns)) > lid.Unwrap() {
			var bits bit.Set = p.columns[lid.Unwrap()]
			//
			if bits.Count() != 0 {
				return true
			}
		}
	}
	//
	return false
}

func (p *moduleCellFilter) Range() (start, end uint) {
	start, end = p.start, p.end
	// apply padding to start
	if start >= p.padding {
		start -= p.padding
	} else {
		start = 0
	}
	// done
	return start, end + p.padding
}
