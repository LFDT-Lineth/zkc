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
package termio

import (
	"fmt"
	"slices"
	"strings"
)

// FormattedTable is useful for printing tables to the terminal.
type FormattedTable struct {
	// Maximum width of each column.
	widths []uint
	// Table data stored in row-major format.
	rows [][]FormattedText
	// Separator printed after each column (defaults to "|").
	separator string
	// Whether each column is left-justified (defaults to false, i.e.
	// right-justified).
	leftAlign []bool
}

// NewFormattedTable constructs a new table with given dimensions.
func NewFormattedTable(width uint, height uint) *FormattedTable {
	widths := make([]uint, width)
	leftAlign := make([]bool, width)
	rows := make([][]FormattedText, height)
	// Construct the table
	for i := uint(0); i < height; i++ {
		rows[i] = make([]FormattedText, width)
	}

	return &FormattedTable{widths: widths, rows: rows, separator: "|", leftAlign: leftAlign}
}

// SetSeparator sets the string printed after each column (e.g. "|" for a ruled
// table, or "" to drop the vertical lines between columns).
func (p *FormattedTable) SetSeparator(separator string) {
	p.separator = separator
}

// SetLeftAlign makes the given column left-justified (the default is
// right-justified).
func (p *FormattedTable) SetLeftAlign(col uint) {
	p.leftAlign[col] = true
}

// Set the contents of a given cell in this table
func (p *FormattedTable) Set(col uint, row uint, val FormattedText) {
	p.widths[col] = max(p.widths[col], uint(len(val.text)))
	p.rows[row][col] = val
}

// Format the contents of a given cell in this table
func (p *FormattedTable) Format(col uint, row uint, escape AnsiEscape) {
	p.rows[row][col] = FormattedText{&escape, p.rows[row][col].text}
}

// Text returns the unformatted text contents of a given cell in this table
func (p *FormattedTable) Text(col uint, row uint) string {
	return string(p.rows[row][col].text)
}

// Height returns the height of this table.
func (p *FormattedTable) Height() uint {
	return uint(len(p.rows))
}

// Sort the data in this table according to a given table sorted.
func (p *FormattedTable) Sort(start uint, sorter TableSorter) {
	slices.SortStableFunc(p.rows[start:], sorter)
}

// SetRow sets the contents of an entire row in this table
func (p *FormattedTable) SetRow(row uint, vals ...FormattedText) {
	if len(vals) != len(p.widths) {
		panic("incorrect number of columns")
	}
	// Update column widths
	for i := 0; i < len(p.widths); i++ {
		p.widths[i] = max(p.widths[i], uint(len(vals[i].text)))
	}
	// Done
	p.rows[row] = vals
}

// SetMaxWidths puts an upper bound on the width of any column.
func (p *FormattedTable) SetMaxWidths(width uint) {
	for i := uint(0); i < uint(len(p.widths)); i++ {
		p.SetMaxWidth(i, width)
	}
}

// SetMaxWidth puts an upper bound on the width of any column.
func (p *FormattedTable) SetMaxWidth(col uint, width uint) {
	p.widths[col] = min(p.widths[col], width)
}

// Print the table with or without the use of ANSI escapes (e.g. for showing
// colour).  Disabling escapes is useful in environments that don't support
// escapes as, otherwise, you get a lot of visible excape characters being
// printed.
func (p *FormattedTable) Print(escapes bool) {
	//
	for i := range p.rows {
		row := p.rows[i]
		//
		for j, col := range row {
			var (
				jth       = col
				jth_width = p.widths[j]
				text      string
			)
			// Clip anything longer than given width
			jth = jth.Clip(0, jth_width)
			// Pad out anything shorter than given width, justifying according
			// to the column's alignment.
			if p.leftAlign[j] {
				jth = jth.PadLeft(jth_width)
			} else {
				jth = jth.Pad(jth_width)
			}
			// Print colour (if applicable)
			if escapes {
				text = string(jth.Bytes())
			} else {
				text = string(jth.text)
			}
			//
			fmt.Printf(" %s %s", text, p.separator)
		}

		fmt.Println()
	}
}

// ============================================================================
// Table Sorter
// ============================================================================

// TableSorter represents a mechanism for sorting tables in some way.
type TableSorter func([]FormattedText, []FormattedText) int

// NewTableSorter constructs a new table sorter which actually does nothing.
// The goal is then to further refine this as necessary.
func NewTableSorter() TableSorter {
	return func(lhs []FormattedText, rhs []FormattedText) int {
		return 0
	}
}

// Invert the direction of sorting, so that largest values comes first.
func (p TableSorter) Invert() TableSorter {
	return func(lhs []FormattedText, rhs []FormattedText) int {
		cmp := p(lhs, rhs)
		//
		return -cmp
	}
}

// SortColumn adds a sort by the given column to the table sorter.
func (p TableSorter) SortColumn(col uint) TableSorter {
	return func(lhs []FormattedText, rhs []FormattedText) int {
		var l, r string
		// Try parent sort
		if c := p(lhs, rhs); c != 0 {
			return c
		}
		//
		l = string(lhs[col].text)
		r = string(rhs[col].text)
		//
		return strings.Compare(l, r)
	}
}

// SortNumericalColumn adds a sort by the given column to the table sorter.
func (p TableSorter) SortNumericalColumn(col uint) TableSorter {
	return func(lhs []FormattedText, rhs []FormattedText) int {
		var l, r string
		// Try parent sort
		if c := p(lhs, rhs); c != 0 {
			return c
		}
		//
		l = string(lhs[col].text)
		r = string(rhs[col].text)
		//
		if len(l) < len(r) {
			return -1
		} else if len(l) > len(r) {
			return 1
		}
		// Now try this sort
		return strings.Compare(l, r)
	}
}
