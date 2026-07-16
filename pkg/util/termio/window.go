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
	"strings"
)

// Window provides an abstraction over an interactive terminal session.  This
// is fairly simplistic at this stage, and supports layout of widges in a
// vertical direction only.
type Window interface {
	// Add a new widget to this window.  Widgets will be laid out vertically in
	// the order they are added.
	Add(w Widget)
	// Render this window to the terminal.
	Render() error
}

// Canvas represents a surface on which widgets can draw.
type Canvas interface {
	// Get the dimensions of this canvas.
	GetDimensions() (uint, uint)
	// Write a chunk to the canvas.
	Write(x, y uint, text FormattedText)
}

// Widget is an abstract entity which can be displayed upon a terminal window.
type Widget interface {
	// Get height of this widget, where MaxUint indicates widget expands to take
	// as much as it can.
	GetHeight() uint
	// Render this widget on the given canvas.
	Render(canvas Canvas)
}

// FormattedText represents, as the name suggests, a chunk of formatted text.
type FormattedText struct {
	// Format to apply to this text (optional)
	format *AnsiEscape
	// Text represents the contents
	text []rune
}

// NewText constructs a new (unformatted) chunk of text.
func NewText(text string) FormattedText {
	return FormattedText{nil, []rune(text)}
}

// NewFormattedText constructs a new chunk of text with a given format.
func NewFormattedText(text string, format AnsiEscape) FormattedText {
	return FormattedText{&format, []rune(text)}
}

// NewColouredText constructs a new (coloured) chunk of text.
func NewColouredText(text string, colour uint) FormattedText {
	escape := NewAnsiEscape().FgColour(colour)
	return FormattedText{&escape, []rune(text)}
}

// Len returns the number of characters [runes] in this chunk of formatted text.
// Observe that this does not include characters arising from the formatting
// escapes.
func (p *FormattedText) Len() uint {
	return uint(len(p.text))
}

// ClearFormat clears any formatting for this chunk of text.
func (p *FormattedText) ClearFormat() {
	p.format = nil
}

// Format sets the format for this chunk of text.
func (p *FormattedText) Format(format AnsiEscape) {
	p.format = &format
}

// Clip removes text from the start and end.
func (p *FormattedText) Clip(start uint, end uint) FormattedText {
	var (
		text []rune
		len  = p.Len()
	)
	// clip text entirely
	if start >= len {
		text = []rune{}
	} else if end >= len {
		text = p.text[start:]
	} else {
		text = p.text[start:end]
	}
	//
	return FormattedText{p.format, text}
}

// Pad ensures the text is at least a given width, by prepending spaces
// (right-justifying the content).
func (p *FormattedText) Pad(width uint) FormattedText {
	var text = p.text
	// Pad only if necessary
	if uint(len(text)) < width {
		text = []rune(fmt.Sprintf("%*s", width, string(text)))
	}
	//
	return FormattedText{p.format, text}
}

// PadLeft ensures the text is at least a given width, by appending spaces
// (left-justifying the content).
func (p *FormattedText) PadLeft(width uint) FormattedText {
	var text = p.text
	// Pad only if necessary
	if uint(len(text)) < width {
		text = []rune(fmt.Sprintf("%-*s", width, string(text)))
	}
	//
	return FormattedText{p.format, text}
}

// Center ensures the text is at least a given width, by padding with spaces on
// both sides (centring the content).
func (p *FormattedText) Center(width uint) FormattedText {
	var n = p.Len()
	// Pad only if necessary
	if n >= width {
		return FormattedText{p.format, p.text}
	}
	//
	left := (width - n) / 2
	right := width - n - left
	text := strings.Repeat(" ", int(left)) + string(p.text) + strings.Repeat(" ", int(right))
	//
	return FormattedText{p.format, []rune(text)}
}

// Bytes returns an ANSI-formatted byte representing of this chunk.
func (p *FormattedText) Bytes() []byte {
	// Append bytes
	if p.format != nil {
		// Apply formatting
		bytes := []byte(p.format.Build())
		// Add content
		bytes = append(bytes, []byte(string(p.text))...)
		// Reset formatting
		escape := ResetAnsiEscape().Build()
		//
		return append(bytes, []byte(escape)...)
	}
	// no formatting
	return []byte(string(p.text))
}
