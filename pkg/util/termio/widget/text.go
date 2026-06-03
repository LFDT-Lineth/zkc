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

import "github.com/LFDT-Lineth/zkc/pkg/util/termio"

// TextLine displays a line of formatted text.
type TextLine struct {
	// Contents of this line
	leftContents  []termio.FormattedText
	rightContents []termio.FormattedText
}

// NewText constructs a new text widget which is initially empty.
func NewText() *TextLine {
	return &TextLine{nil, nil}
}

// GetHeight of this widget, where MaxUint indicates widget expands to take as
// much as it can.
func (p *TextLine) GetHeight() uint {
	return 1
}

// Clear contents of this text line.
func (p *TextLine) Clear() {
	p.leftContents = nil
	p.rightContents = nil
}

// AddLeft adds a new left-aligned chunk of formatted text.
func (p *TextLine) AddLeft(txt termio.FormattedText) {
	p.leftContents = append(p.leftContents, txt)
}

// AddRight adds a new right-aligned chunk of formatted text.
func (p *TextLine) AddRight(txt termio.FormattedText) {
	p.rightContents = append(p.rightContents, txt)
}

// Render the tabs widget to a given canvas.
func (p *TextLine) Render(canvas termio.Canvas) {
	var (
		width, _ = canvas.GetDimensions()
		xpos     = uint(0)
	)
	// Render left-aligned contents
	for _, txt := range p.leftContents {
		canvas.Write(xpos, 0, txt)
		xpos += txt.Len() + 0
	}
	//
	xpos = width
	// Render right-aligned contents
	for _, txt := range p.rightContents {
		xpos -= txt.Len() + 0
		canvas.Write(xpos, 0, txt)
	}
}
