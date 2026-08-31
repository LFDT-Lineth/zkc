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
	// Number of columns each cell spans (row-major, defaults to 1).  A span
	// greater than one merges the cell with the following columns, centring its
	// content across them (see SetSpan).  Allocated lazily.
	spans [][]uint
	// Rows before which a full-width horizontal rule should be printed.  A key of
	// len(rows) draws a trailing rule after the final row (see SetRule).
	rules map[uint]bool
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

	return &FormattedTable{widths: widths, rows: rows, separator: "|", leftAlign: leftAlign,
		rules: make(map[uint]bool)}
}

// SetSpan sets a cell which spans the given number of columns, centring its
// content across them.  Unlike Set, a spanning cell does not widen the
// individual column it starts in; the label is expected to fit within the
// combined width of the spanned columns.
func (p *FormattedTable) SetSpan(col uint, row uint, span uint, val FormattedText) {
	if p.spans == nil {
		p.spans = make([][]uint, len(p.rows))
	}
	//
	if p.spans[row] == nil {
		p.spans[row] = make([]uint, len(p.widths))
	}
	//
	p.spans[row][col] = span
	p.rows[row][col] = val
}

// SetRule requests a full-width horizontal rule be printed immediately before
// the given row.  Passing Height() draws a trailing rule after the final row.
func (p *FormattedTable) SetRule(beforeRow uint) {
	p.rules[beforeRow] = true
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

// PrintedWidth returns the total width (in characters) of this table as rendered
// by Print.  This is useful when something of matching width needs to be drawn
// alongside the table (e.g. a horizontal rule).  Each column contributes its
// width plus a leading space, a trailing space and the separator.
func (p *FormattedTable) PrintedWidth() uint {
	var width uint
	//
	for _, w := range p.widths {
		width += w + 2 + uint(len(p.separator))
	}
	//
	return width
}

// Print the table with or without the use of ANSI escapes (e.g. for showing
// colour).  Disabling escapes is useful in environments that don't support
// escapes as, otherwise, you get a lot of visible excape characters being
// printed.
func (p *FormattedTable) Print(escapes bool) {
	//
	for i := range p.rows {
		// Print a horizontal rule before this row, if requested.
		if p.rules[uint(i)] {
			fmt.Println(strings.Repeat("-", int(p.PrintedWidth())))
		}
		//
		p.printRow(i, escapes)
	}
	// Print a trailing rule after the final row, if requested.
	if p.rules[uint(len(p.rows))] {
		fmt.Println(strings.Repeat("-", int(p.PrintedWidth())))
	}
}

// printRow prints a single row, honouring column spans and alignment.
func (p *FormattedTable) printRow(row int, escapes bool) {
	cells := p.rows[row]
	//
	for j := 0; j < len(cells); {
		var (
			span  = p.spanAt(row, j)
			width = p.widths[j]
			jth   = cells[j]
		)
		// Spanning cell: centre across the combined width of the columns.
		if span > 1 {
			width = p.spanWidth(j, span)
		}
		//
		jth = jth.Clip(0, width)
		// Justify according to the cell's span / column alignment.
		switch {
		case span > 1:
			jth = jth.Center(width)
		case p.leftAlign[j]:
			jth = jth.PadLeft(width)
		default:
			jth = jth.Pad(width)
		}
		// Print colour (if applicable)
		var text string
		if escapes {
			text = string(jth.Bytes())
		} else {
			text = string(jth.text)
		}
		//
		fmt.Printf(" %s %s", text, p.separator)
		//
		j += int(span)
	}
	//
	fmt.Println()
}

// spanAt returns the number of columns the cell at (row, col) spans (1 if
// unset).
func (p *FormattedTable) spanAt(row int, col int) uint {
	if p.spans == nil || p.spans[row] == nil || p.spans[row][col] == 0 {
		return 1
	}
	//
	return p.spans[row][col]
}

// spanWidth returns the content width available to a cell spanning span columns
// starting at col, accounting for the padding and separators that would
// otherwise sit between them.
func (p *FormattedTable) spanWidth(col int, span uint) uint {
	var width uint
	//
	for k := 0; k < int(span); k++ {
		width += p.widths[col+k]
	}
	// Each internal boundary contributes two padding spaces plus a separator.
	return width + (span-1)*(2+uint(len(p.separator)))
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
		// Try parent sort
		if c := p(lhs, rhs); c != 0 {
			return c
		}
		//
		var (
			lv = string(lhs[col].text)
			rv = string(rhs[col].text)
		)
		//
		l := parseNumericColumn(lv)
		r := parseNumericColumn(rv)
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

func parseNumericColumn(text string) string {
	var (
		gtext, giga = strings.CutSuffix(text, "G")
		mtext, mega = strings.CutSuffix(text, "M")
		ktext, kilo = strings.CutSuffix(text, "K")
	)
	// Account for "human-readable" forms.
	switch {
	case giga:
		return fmt.Sprintf("%s000000000", gtext)
	case mega:
		return fmt.Sprintf("%s000000", mtext)
	case kilo:
		return fmt.Sprintf("%s000", ktext)
	default:
		return text
	}
}
