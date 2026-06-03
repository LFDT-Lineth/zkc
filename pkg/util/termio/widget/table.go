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
package widget

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
)

// TableSource is an abstraction used by a table to determine what values to put in each cell.
type TableSource interface {
	// Width returns the width of a given column.
	ColumnWidth(col uint) uint
	// Get the width and height of this table
	Dimensions() (uint, uint)
	// Get content of given cell in table.
	CellAt(col, row uint) termio.FormattedText
}

// Table is a grid of cells of varying width.
type Table struct {
	source TableSource
}

// NewTable constructs a new table with a given source.
func NewTable(source TableSource) *Table {
	return &Table{source}
}

// GetHeight of this widget, where MaxUint indicates widget expands to take as
// much as it can.
func (p *Table) GetHeight() uint {
	return math.MaxUint
}

// SetSource sets the table source.
func (p *Table) SetSource(source TableSource) {
	p.source = source
}

// Render this widget on the given canvas.
func (p *Table) Render(canvas termio.Canvas) {
	// Determine canvas dimensions
	width, height := canvas.GetDimensions()
	//
	xpos := uint(0)
	//
	for col := uint(0); xpos < width; col++ {
		colWidth := p.source.ColumnWidth(col)
		//
		for row := uint(0); row < height; row++ {
			cell := p.source.CellAt(col, row)
			cell = cell.Clip(0, colWidth)
			canvas.Write(xpos, row, cell)
		}
		//
		xpos += colWidth + 1
	}
}

// Print the table.
func (p *Table) Print() {
	width, height := p.source.Dimensions()
	//
	for row := range height {
		for col := range width {
			var (
				jth       = p.source.CellAt(col, row)
				jth_width = p.source.ColumnWidth(col)
				text      string
			)
			// Clip anything longer than given width
			jth = jth.Clip(0, jth_width)
			// Pad out anything shorter than given width
			jth = jth.Pad(jth_width)
			// Print
			text = string(jth.Bytes())
			//
			fmt.Printf(" %s |", text)
		}

		fmt.Println()
	}
}
