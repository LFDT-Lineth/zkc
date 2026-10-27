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
package source

import (
	"fmt"
	"os"
)

// ReadFiles reads a given set of source files, or produces an error.
func ReadFiles(filenames ...string) ([]File, error) {
	files := make([]File, len(filenames))
	//
	for i, n := range filenames {
		bytes, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		//
		files[i] = *NewSourceFile(n, bytes)
	}
	//
	return files, nil
}

// Line provides information about a given line within the original string.
// This includes the line number (counting from 1), and the span of the line
// within the original string.
type Line struct {
	// Original text
	text []rune
	// Span within original text of this line.
	span Span
	// Line number of this line (counting from 1).
	number int
}

// String returns a string representing this line.
func (p *Line) String() string {
	// Extract runes representing line
	runes := p.text[p.span.start:p.span.end]
	// Convert into string
	return string(runes)
}

// Number gets the line number of this line, where the first line in a string
// has line number 1.
func (p *Line) Number() int {
	return p.number
}

// Start returns the starting index of this line in the original string.
func (p *Line) Start() int {
	return p.span.start
}

// Length returns the number of characters in this line.
func (p *Line) Length() int {
	return p.span.Length()
}

// File represents a given source file (typically stored on disk).
type File struct {
	// File name for this source file.
	filename string
	// Contents of this file.
	contents []rune
}

// NewSourceFile constructs a new source file from a given byte array.
func NewSourceFile(filename string, bytes []byte) *File {
	// Convert bytes into runes for easier parsing
	contents := []rune(string(bytes))
	return &File{filename, contents}
}

// Filename returns the filename associated with this source file.
func (s *File) Filename() string {
	return s.filename
}

// Contents returns the contents of this source file.
func (s *File) Contents() []rune {
	return s.contents
}

// SyntaxError constructs a syntax error over a given span of this file with a
// given message.
func (s *File) SyntaxError(span Span, msg string) *SyntaxError {
	return &SyntaxError{s, span, msg}
}

// Lines splits the contents of this file into individual lines.
func (s *File) Lines() []Line {
	var (
		nchars = len(s.contents)
		// Num records the line number, counting from 1.
		num = 1
		// Start records the starting offset of the current line.
		start = 0
		lines []Line
	)
	//
	for i := 0; i < nchars; i++ {
		if s.contents[i] == '\n' {
			// Construct new line
			lines = append(lines, Line{s.contents, Span{start, i}, num})
			// Incremenet line number
			num++
			// Reset start of next line
			start = i + 1
		}
	}
	// Check for non-terminate final line
	if start < nchars {
		// Construct new line
		lines = append(lines, Line{s.contents, Span{start, nchars}, num})
	}
	//
	return lines
}

// FindFirstEnclosingLine determines the first line  in this source file which
// encloses the start of a span.  Observe that, if the position is beyond the
// bounds of the source file then the last physical line is returned.  Also,
// the returned line is not guaranteed to enclose the entire span, as these can
// cross multiple lines.
func (s *File) FindFirstEnclosingLine(span Span) Line {
	var (
		// Index identifies the current position within the original text.
		index = span.start
		// Num records the line number, counting from 1.
		num = 1
		// Start records the starting offset of the current line.
		start = 0
	)
	// Find the line.
	for i := 0; i < len(s.contents); i++ {
		if i == index {
			end := findEndOfLine(index, s.contents)
			return Line{s.contents, Span{start, end}, num}
		} else if s.contents[i] == '\n' {
			num++
			start = i + 1
		}
	}
	//
	return Line{s.contents, Span{start, len(s.contents)}, num}
}

// SyntaxError is a structured error which retains the index into the original
// string where an error occurred, along with an error message.
type SyntaxError struct {
	srcfile *File
	// Byte index into string being parsed where error arose.
	span Span
	// Error message being reported
	msg string
}

// SourceFile returns the underlying source file that this syntax error covers.
func (p *SyntaxError) SourceFile() *File {
	return p.srcfile
}

// Span returns the span of the original text on which this error is reported.
func (p *SyntaxError) Span() Span {
	return p.span
}

// Message returns the message to be reported.
func (p *SyntaxError) Message() string {
	return p.msg
}

// Error implements the error interface.
func (p *SyntaxError) Error() string {
	return fmt.Sprintf("%d:%d:%s", p.span.Start(), p.span.End(), p.Message())
}

// FirstEnclosingLine determines the first line in this source file to which
// this error is associated. Observe that, if the position is beyond the bounds
// of the source file then the last physical line is returned.  Also, the
// returned line is not guaranteed to enclose the entire span, as these can
// cross multiple lines.
func (p *SyntaxError) FirstEnclosingLine() Line {
	return p.srcfile.FindFirstEnclosingLine(p.span)
}

// Find the end of the enclosing line
func findEndOfLine(index int, text []rune) int {
	for i := index; i < len(text); i++ {
		if text[i] == '\n' {
			return i
		}
	}
	// No end in sight!
	return len(text)
}
