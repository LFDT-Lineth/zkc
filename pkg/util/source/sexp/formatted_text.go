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
package sexp

import "strings"

// FormattedText provides encapsulates the notion of formatted chunk of text.
type FormattedText struct {
	// Current indent level
	indent int
	// Lines being written
	lines []string
}

func (p *FormattedText) String() string {
	var builder strings.Builder
	//
	for _, l := range p.lines {
		builder.WriteString(l)
		builder.WriteString("\n")
	}
	//
	return builder.String()
}

// Indent increases or decreases the current indent level.
func (p *FormattedText) Indent(delta int) {
	p.indent += delta
}

// NewLine starts a new line
func (p *FormattedText) NewLine() {
	var builder strings.Builder
	// write indent
	for i := 0; i < p.indent; i++ {
		builder.WriteString("   ")
	}
	//
	p.lines = append(p.lines, builder.String())
}

// LineWidth returns the width of the current line.
func (p *FormattedText) LineWidth() uint {
	var n = len(p.lines)
	//
	if n == 0 {
		return 0
	}
	// Width of last line
	return uint(len(p.lines[n-1]))
}

// MaxWidth returns the maximum width of any line in this formatted text block.
func (p *FormattedText) MaxWidth() uint {
	width := 0
	//
	for _, l := range p.lines {
		width = max(width, len(l))
	}
	//
	return uint(width)
}

// WriteString writes a string into the current line of this formatted text
// block.
func (p *FormattedText) WriteString(str string) {
	if len(p.lines) == 0 {
		p.lines = append(p.lines, str)
	} else {
		var builder strings.Builder
		//
		n := len(p.lines) - 1
		// Write current contents
		builder.WriteString(p.lines[n])
		builder.WriteString(str)
		//
		p.lines[n] = builder.String()
	}
}
