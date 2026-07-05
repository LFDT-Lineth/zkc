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
package util

import "strings"

// Range identifies a contiguous span of rows, from Start up to and including
// Finish, during which its column is considered active.  The endpoints may be
// given in either order.
type Range struct {
	Start  uint
	Finish uint
}

// bounds returns this range's endpoints in ascending order.
func (p Range) bounds() (uint, uint) {
	if p.Start > p.Finish {
		return p.Finish, p.Start
	}
	//
	return p.Start, p.Finish
}

// Contains determines whether a given row falls within this range (inclusive of
// both endpoints).
func (p Range) Contains(row uint) bool {
	lo, hi := p.bounds()
	//
	return lo <= row && row <= hi
}

// Overlaps determines whether this range shares any row with the other.  Both
// endpoints are treated as inclusive, so ranges which merely touch (share a
// single endpoint row) are also considered to overlap.
func (p Range) Overlaps(other Range) bool {
	plo, phi := p.bounds()
	olo, ohi := other.bounds()
	//
	return plo <= ohi && olo <= phi
}

// FlowGraph renders a set of vertical columns down a grid of rows.  Each range
// occupies its own column (in the order added) and is "active" on the rows
// within its span.  The intention is to visualise, at a glance, which rows each
// range covers.
type FlowGraph struct {
	// ranges holds one entry per column, in left-to-right order.
	ranges []Range
}

// NewFlowGraph constructs a FlowGraph from the given ranges, one column per
// range in the order provided.
func NewFlowGraph(ranges ...Range) FlowGraph {
	return FlowGraph{ranges}
}

// Add appends a new range (column) spanning rows start..finish (inclusive).
func (p *FlowGraph) Add(start, finish uint) {
	p.ranges = append(p.ranges, Range{start, finish})
}

// glyphs returns the two-column box-drawing rendering of this range at the
// given row.  The right column carries the vertical line and its corners; the
// left column carries the horizontal lead-in (at the source) and the arrow
// head (at the target), both pointing left toward the instruction.  A range is
// rendered as an arrow running from its Start (source) to its Finish (target):
//
//	─┐       source (Start), when the target lies below
//	 │       span in between
//	◄┘       target (Finish), when the target lies below
//
// Two spaces are returned for rows lying outside the range.
func (p Range) glyphs(row uint) string {
	switch {
	case !p.Contains(row):
		return "  "
	case row == p.Finish && p.Finish >= p.Start:
		// Target below the source: arrive from above, turn left into the
		// instruction (arrow head + up-and-left corner).
		return "◄┘"
	case row == p.Finish:
		// Target above the source: arrive from below, turn left into the
		// instruction (arrow head + down-and-left corner).
		return "◄┐"
	case row == p.Start && p.Finish > p.Start:
		// Source above the target: horizontal lead-in then head down.
		return "─┐"
	case row == p.Start:
		// Source below the target: horizontal lead-in then head up.
		return "─┘"
	default:
		return " │"
	}
}

// flowColumn holds a set of mutually non-overlapping ranges which can therefore
// share a single rendered column.
type flowColumn struct {
	ranges []Range
}

// overlaps determines whether the given range overlaps any range already
// allocated to this column.
func (p *flowColumn) overlaps(r Range) bool {
	for _, existing := range p.ranges {
		if existing.Overlaps(r) {
			return true
		}
	}
	//
	return false
}

// add allocates a range to this column (the caller having ensured it does not
// overlap any range already present).
func (p *flowColumn) add(r Range) {
	p.ranges = append(p.ranges, r)
}

// glyphs returns this column's rendering at the given row: the glyphs of
// whichever contained range is active there (at most one, since the column's
// ranges are non-overlapping), or two spaces when none is active.
func (p *flowColumn) glyphs(row uint) string {
	for _, r := range p.ranges {
		if r.Contains(row) {
			return r.glyphs(row)
		}
	}
	//
	return "  "
}

// allocate packs the ranges into columns of mutually non-overlapping ranges: it
// sweeps through the ranges (in the order added), greedily placing each into
// the first column it does not overlap, and creating a new column when none
// will accommodate it.
func (p *FlowGraph) allocate() []flowColumn {
	var columns []flowColumn
	//
	for _, r := range p.ranges {
		placed := false
		// Try to place the range in the first available column.
		for i := range columns {
			if !columns[i].overlaps(r) {
				columns[i].add(r)

				placed = true
				//
				break
			}
		}
		// Otherwise, start a new column for it.
		if !placed {
			columns = append(columns, flowColumn{ranges: []Range{r}})
		}
	}
	//
	return columns
}

// Render produces n strings, one per row.  Ranges are packed into columns such
// that non-overlapping ranges share a column; each column contributes two
// adjacent characters (in left-to-right order), and adjacent columns are
// separated by a space.  A range draws a vertical arrow spanning it: a
// horizontal lead-in at the source row, and an arrow head turning left toward
// the instruction at the target row.  Rows outside every range in a column
// render as blank there.
func (p *FlowGraph) Render(n uint) []string {
	var (
		columns = p.allocate()
		rows    = make([]string, n)
	)
	//
	for row := range n {
		cols := make([]string, len(columns))
		//
		for i := range columns {
			cols[i] = columns[i].glyphs(row)
		}
		//
		rows[row] = strings.Join(cols, " ")
	}
	//
	return rows
}
