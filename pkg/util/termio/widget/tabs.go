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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
)

// Tabs is a simple widget which shows a bunch of titles in a bar, and
// highlights a selected one.
type Tabs[T any] struct {
	tabs     []util.Pair[string, T]
	selected uint
	offset   uint
}

// NewTabs constructs a new tabs widget with the given titles.
func NewTabs[T any](tabs ...util.Pair[string, T]) *Tabs[T] {
	return &Tabs[T]{tabs, 0, 0}
}

// HasSelected determines whether or not there is anything which could be
// selected.
func (p *Tabs[T]) HasSelected() bool {
	return len(p.tabs) > 0
}

// Selected returns the currently selected tab.
func (p *Tabs[T]) Selected() T {
	return p.tabs[p.selected].Right
}

// Select sets the given selected tab.  If the index is greater than the
// available tabs, then it automatically "wraps around".
func (p *Tabs[T]) Select(shift int) {
	if p.HasSelected() {
		var tab = int(p.selected) + shift
		//
		if tab < 0 {
			tab += len(p.tabs)
		}
		//
		p.selected = uint(tab % len(p.tabs))
	}
}

// Render the tabs widget to a given canvas.
func (p *Tabs[T]) Render(canvas termio.Canvas) {
	w, _ := canvas.GetDimensions()
	//
	p.updateOffset(w)
	//
	x := uint(1)
	//
	for i := p.offset; i < uint(len(p.tabs)) && x < w; i++ {
		if i != p.offset {
			// Write out separator
			canvas.Write(x, 0, termio.NewText(" | "))
			x += 3
		}
		// Extract title
		cell := termio.NewText(p.tabs[i].Left)
		// Check for selected
		if i == p.selected {
			cell.Format(termio.UnderlineAnsiEscape())
		}
		// Write out title
		canvas.Write(x, 0, cell)
		x += cell.Len()
	}
}

// GetHeight of this widget, where MaxUint indicates widget expands to take as
// much as it can.
func (p *Tabs[T]) GetHeight() uint {
	return 1
}

func (p *Tabs[T]) updateOffset(width uint) {
	if p.selected < p.offset {
		p.offset = p.selected
	} else {
		var ntabs = uint(len(p.tabs))
		// Keep shifting the offset until the selected tab is visible.
		for p.selected >= p.offset+p.visibleTabCount(width) && p.offset < ntabs {
			p.offset++
		}
	}
}

func (p *Tabs[T]) visibleTabCount(width uint) uint {
	var (
		x = uint(1)
		n = p.offset
	)
	//
	for ; n < uint(len(p.tabs)) && x < width; n++ {
		if n != p.offset {
			x += 3
		}
		// NOTE: this calculation is a little rough.  It doesn't consider
		// clipping, or unicode.
		x += uint(len(p.tabs[n].Left))
	}
	// Account for last tab which may be partially obscured.
	if n < uint(len(p.tabs)) && x != width && n > 0 {
		n--
	}
	//
	return n - p.offset
}
