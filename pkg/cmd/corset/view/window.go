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

// Window provides a "view port" into the trace data.
type Window struct {
	startCol, endCol uint
	startRow         uint
	rows             []SourceColumnId
}

// NewWindow constructs a new window
func NewWindow(width uint, rows []SourceColumnId) Window {
	return Window{0, width, 0, rows}
}

// Goto shifts the starting point of this window to a given offset.
func (p Window) Goto(col, row uint) Window {
	col = min(p.endCol, col)
	row = min(uint(len(p.rows)), row)
	//
	return Window{col, p.endCol, row, p.rows}
}

// Offset returns the x,y offset of this window.
func (p Window) Offset() (x uint, y uint) {
	return p.startCol, p.startRow
}

// Row returns the source column represented by the given row in this window.
func (p Window) Row(index uint) SourceColumnId {
	return p.rows[index+p.startRow]
}

// Rows returns the set of all active rows in this window.
func (p Window) Rows() []SourceColumnId {
	return p.rows
}

// Dimensions returns the width and height this window.
func (p Window) Dimensions() (width uint, height uint) {
	return p.endCol - p.startCol, uint(len(p.rows)) - p.startRow
}
