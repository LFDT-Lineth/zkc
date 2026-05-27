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
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/termio"
	"github.com/consensys/go-corset/pkg/util/termio/widget"
)

// ModuleConfig encapsulates various configuration settings for a ModuleView.
type ModuleConfig interface {
	// CellWidth returns the current maximum width of any cell in the table.
	CellWidth() uint
	// TitleWidth returns the current maximum width of any title in the table.
	TitleWidth() uint
	// SetCellWidth sets the maximum width of any cells in the table.
	SetCellWidth(cellWidth uint)
	// SetTitleWidth set the maximum width of any title in the table.
	SetTitleWidth(titleWidth uint)
}

// ModuleView abstracts an underlying trace module.  For example, it manages the
// way that column data is displayed (e.g. in hex or in decimal, etc), whether
// or not register limbs are shown, and provides a mechanism for querying
// whether a given cell is "active" or not.  Specifically, cells in a
// perspective are not active when that perspective is not active.
type ModuleView interface {
	widget.TableSource
	// Config returns config settings for this module.
	Config() ModuleConfig
	// Data abstracts the raw data of the underlying module.
	Data() ModuleData
	// Filter this view to produce a more focused view.
	Filter(ModuleFilter) ModuleView
	// Return offset position within module
	Offset() (uint, uint)
	// Set offset position within module
	Goto(uint, uint)
	// Window returns the active window for this module.
	Window() Window
}

// ============================================================================
// View Implementation
// ============================================================================

type moduleView[F field.Element[F]] struct {
	// width of all cells / titles
	cellWidth, titleWidth uint
	// Limbs indicates whether or not to show the raw limbs, or the combined
	// source-level register.
	limbs bool
	// Formatting for this module
	formatting ModuleFormatting
	// viewport within module data
	window Window
	// Data provides the raw underlying data which can be shared between
	// multiple views.
	data *moduleData[F]
}

// Offset returns the current offset position within module
func (p *moduleView[F]) Config() ModuleConfig {
	return p
}

// Data returns an abtract view of the data for given register
func (p *moduleView[F]) Data() ModuleData {
	return p.data
}

// Filter columns in this module
func (p *moduleView[F]) Filter(filter ModuleFilter) ModuleView {
	var q = *p
	//
	q.window = p.data.Filter(filter)
	//
	return &q
}

// Offset returns the current offset position within module
func (p *moduleView[F]) Offset() (uint, uint) {
	return p.window.Offset()
}

// Goto a specific offset within module
func (p *moduleView[F]) Goto(x, y uint) {
	p.window = p.window.Goto(x, y)
}

// Window returns the active window for this module
func (p *moduleView[F]) Window() Window {
	return p.window
}

// ============================================================================
// ModuleConfig
// ============================================================================

// CellWidth implementation for ModuleComfig interface
func (p *moduleView[F]) CellWidth() uint {
	return p.cellWidth
}

// SetCellWidth implementation for ModuleConfig interface
func (p *moduleView[F]) SetCellWidth(cellWidth uint) {
	p.cellWidth = cellWidth
}

// CellWidth implementation for ModuleComfig interface
func (p *moduleView[F]) TitleWidth() uint {
	return p.titleWidth
}

// SetCellWidth implementation for ModuleComfig interface
func (p *moduleView[F]) SetTitleWidth(titleWidth uint) {
	p.titleWidth = titleWidth
}

// ============================================================================
// TableSource
// ============================================================================

// CellAt returns the contents of a specific cell in this table.
func (p *moduleView[F]) CellAt(col uint, row uint) termio.FormattedText {
	var (
		formatted termio.FormattedText
		x, _      = p.window.Offset()
	)
	//
	if col == 0 && row == 0 {
		return termio.NewText("")
	} else if col == 0 {
		reg := p.window.Row(row - 1)
		text := clipValue(p.data.RowTitle(reg), p.titleWidth)
		formatted = p.formatting.RowTitle(reg, text)
	} else if row == 0 {
		text := p.data.ColumnTitle(col + x - 1)
		formatted = p.formatting.ColumnTitle(col+x-1, text)
	} else {
		srcCol := p.window.Row(row - 1)
		row = col + x - 1
		//
		text := p.data.CellAt(row, srcCol.Unwrap())
		// Clip the cell value
		text = clipValue(text, p.cellWidth)
		//
		formatted = p.formatting.Cell(p.data.SourceColumn(srcCol), row, text)
	}
	// apply formatting (if applicable)
	return formatted
}

func (p *moduleView[F]) ColumnWidth(col uint) uint {
	var (
		w, _ = p.data.Dimensions()
		x, _ = p.window.Offset()
	)
	//
	if col == 0 {
		width := uint(0)
		//
		for _, row := range p.window.Rows() {
			text := p.data.RowTitle(row)
			width = max(width, uint(len(text)))
		}
		//
		return min(p.titleWidth, width)
	} else if col+x-1 >= w {
		return 0
	}
	//
	col = col + x - 1
	//
	width := uint(len(p.data.ColumnTitle(col)))
	//
	for _, row := range p.window.Rows() {
		text := p.data.CellAt(col, row.Unwrap())
		width = max(width, uint(len(text)))
	}
	//
	return min(p.cellWidth, width)
}

func (p *moduleView[F]) Dimensions() (uint, uint) {
	var width, height = p.window.Dimensions()
	// Account for row / column title
	return width + 1, height + 1
}

func clipValue(str string, maxWidth uint) string {
	runes := []rune(str)
	//
	if maxWidth > 2 && len(runes) > int(maxWidth) {
		runes := runes[:maxWidth]
		runes[maxWidth-1] = '.'
		runes[maxWidth-2] = '.'
		// done
		return string(runes)
	} else if len(runes) > int(maxWidth) {
		return string(runes[:maxWidth])
	}
	// No clipping required
	return str
}
