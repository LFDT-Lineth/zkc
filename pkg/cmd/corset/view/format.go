// Copyright Consensys Incorporated
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
	"fmt"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
)

// TraceFormatting provides a top-level notion of formatting
type TraceFormatting[F field.Element[F]] interface {
	Module(ModuleData[F]) ModuleFormatting
}

// ModuleFormatting provides a generic mechanism for applying formatting to
// modules being refendered.
type ModuleFormatting interface {
	// ColumnTitle returns optional formating for a given column title.
	ColumnTitle(uint, string) termio.FormattedText
	// ColumnTitle returns optional formating for a given row title.
	RowTitle(SourceColumnId, string) termio.FormattedText
	// Cell returns optional formating for a given cells in the trace.
	Cell(SourceColumn, uint, string) termio.FormattedText
}

// DefaultFormatter returns a default formatting
func DefaultFormatter[F field.Element[F]]() TraceFormatting[F] {
	return &defaultFormatter[F]{}
}

type defaultFormatter[F field.Element[F]] struct{}

// ColumnTitle implementation for Formatting interface
func (p *defaultFormatter[F]) ColumnTitle(_ uint, text string) termio.FormattedText {
	return termio.NewText(text)
}

// ColumnTitle implementation for Formatting interface
func (p *defaultFormatter[F]) Module(ModuleData[F]) ModuleFormatting {
	return p
}

// ColumnTitle implementation for Formatting interface
func (p *defaultFormatter[F]) RowTitle(_ register.Id, text string) termio.FormattedText {
	return termio.NewText(text)
}

// Cell implementation for Formatting interface
func (p *defaultFormatter[F]) Cell(_ SourceColumn, _ uint, text string) termio.FormattedText {
	return termio.NewText(text)
}

// ============================================================================
// Cell Formatting
// ============================================================================

// NewCellFormatter constructs a new cell formatter
func NewCellFormatter[F field.Element[F]](cells CellRefSet, ansiEscapes bool) TraceFormatting[F] {
	return &cellFormatter[F]{nil, cells, ansiEscapes}
}

type cellFormatter[F field.Element[F]] struct {
	data        ModuleData[F]
	cells       CellRefSet
	ansiEscapes bool
}

// ColumnTitle implementation for Formatting interface
func (p *cellFormatter[F]) ColumnTitle(_ uint, text string) termio.FormattedText {
	if p.ansiEscapes {
		return termio.NewFormattedText(text, termio.NewAnsiEscape().FgColour(termio.TERM_WHITE))
	}
	//
	return termio.NewText(text)
}

// ColumnTitle implementation for ModuleFormatting interface
func (p *cellFormatter[F]) RowTitle(_ SourceColumnId, text string) termio.FormattedText {
	if p.ansiEscapes {
		return termio.NewFormattedText(text, termio.NewAnsiEscape().FgColour(termio.TERM_WHITE))
	}
	//
	return termio.NewText(text)
}

// Module implementation for TraceFormatting interface
func (p *cellFormatter[F]) Module(mod ModuleData[F]) ModuleFormatting {
	var q = *p
	//
	q.data = mod
	//
	return &q
}

// Cell implementation for ModuleFormatting interface
func (p *cellFormatter[F]) Cell(col SourceColumn, row uint, text string) termio.FormattedText {
	// Check each limb in turn
	if !p.data.IsActive(col, row) {
		if p.ansiEscapes {
			return termio.NewColouredText(text, termio.TERM_BLACK)
		}
		//
		return termio.NewText("")
	} else if containsCell(p.data.Id(), col, row, p.cells) {
		if p.ansiEscapes {
			return termio.NewFormattedText(text, termio.NewAnsiEscape().FgColour(termio.TERM_RED))
		}
		//
		return termio.NewText(fmt.Sprintf("*%s", text))
	}
	//
	return termio.NewText(text)
}

func containsCell(mid sc.ModuleId, col SourceColumn, row uint, cells CellRefSet) bool {
	// Check each limb in turn
	for _, lid := range col.Limbs {
		// Consrtuct column reference
		limbRef := tr.NewColumnRef(mid, lid)
		// Construct cellRef reference
		cellRef := tr.NewCellRef(limbRef, int(row))
		// if any limb involved, entire column involved.
		if cells.Contains(cellRef) {
			return true
		}
	}
	//
	return false
}
