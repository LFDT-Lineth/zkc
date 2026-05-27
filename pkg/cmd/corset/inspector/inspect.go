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
package inspector

import (
	"fmt"
	"regexp"
	"time"

	"github.com/consensys/go-corset/pkg/cmd/corset/view"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/termio"
	"github.com/consensys/go-corset/pkg/util/termio/widget"
)

// ==================================================================
// Inspector
// ==================================================================

// DEFAULT_MODE sets the default command bar, and allows the user to navigate
// the trace.
const DEFAULT_MODE = 0

// NUMERIC_INPUT_MODE is where the user is entering a numberic value (e.g. to
// specify the row for a goto command).
const NUMERIC_INPUT_MODE = 1

// TEXT_INPUT_MODE is where the user is entering a text value (e.g. for a column
// filter).
const TEXT_INPUT_MODE = 2

// STATUS_MODE means the commandbar is notifying the user with a message for a
// short period of time.
const STATUS_MODE = 3

// Inspector provides the necessary package
type Inspector struct {
	width  uint
	height uint
	//
	term *termio.Terminal
	// Module states
	modules []ModuleState
	// true means show all modules
	visibility bool
	// Widgets
	tabs      *widget.Tabs[uint]
	table     *widget.Table
	cmdBar    *widget.TextLine
	statusBar *widget.TextLine
	statusClk uint
	// The stack of "modes" in which the inspector is operating.  The root modes
	// is the first in the stack.  When this is terminated, then the inspector
	// closes.
	modes []Mode
}

// Mode identifies a mode in which the inspector is operating.  The
// default mode is for navigating the trace, but other modes are available for
// receiving input from the user or displaying error messages, etc.
type Mode interface {
	// Activate is called when this mode becomes active.  This happens when the
	// mode is first entered, but can also happen subsequently when a child mode
	// exits and results in this mode being reactivated.
	Activate(*Inspector)
	// Clock is called on every clock tick.  This gives the mode an opportunity
	// to do something if it wishes to.
	Clock(*Inspector)
	// KeyPressed in the inspector and received by this mode.
	KeyPressed(*Inspector, uint16) bool
}

// NewInspector constructs a new inspector on given terminal.
func NewInspector(term *termio.Terminal, trace view.TraceView) *Inspector {
	var (
		states     = make([]ModuleState, 0)
		numVisible = 0
	)
	// Sort into alpabetical order
	ntrace := trace.Sort(func(l, r schema.ModuleId) int {
		var (
			lName = trace.Module(l).Data().Name()
			rName = trace.Module(r).Data().Name()
		)
		//
		return lName.Cmp(rName)
	})
	// Filter out non-public modules
	ntrace = ntrace.Filter(view.NewTraceFilter(func(mid schema.ModuleId) view.ModuleFilter {
		ith := trace.Module(mid)
		if ith.Data().Name().Name != "" {
			if ith.Data().IsPublic() {
				numVisible++
			}
			//
			return view.DefaultModuleFilter()
		}
		//
		return nil
	}))
	//
	for i := range ntrace.Width() {
		states = append(states, newModuleState(ntrace.Module(i)))
	}
	// Sort states
	//
	tabs, table, cmdbar, statusbar := initInspectorWidgets(term, states, numVisible == 0)
	//
	inspector := &Inspector{0, 0, term, states, numVisible == 0, tabs, table, cmdbar, statusbar, 0, nil}
	table.SetSource(inspector)
	// Put the inspector into default mode.
	inspector.EnterMode(&NavigationMode{})
	//
	if numVisible == 0 {
		inspector.statusClk = 10
		inspector.statusBar.AddLeft(
			termio.NewColouredText("no public modules found; showing private modules.", termio.TERM_RED))
	}
	//
	return inspector
}

// Clock the inspector
func (p *Inspector) Clock() error {
	dirty := false
	mode := len(p.modes) - 1
	nWidth, nHeight := p.term.GetSize()
	// Pass on clock
	p.modes[mode].Clock(p)
	//
	if p.statusClk != 0 {
		p.statusClk = p.statusClk - 1
		// Clear status when clock expired
		if p.statusClk == 0 {
			p.statusClk = 0
			p.statusBar.Clear()
			// Force render
			dirty = true
		}
	}
	// Only force rerender if dimensions have changed.
	if dirty || nWidth != p.width || nHeight != p.height {
		// Update cached dimensions
		p.width, p.height = nWidth, nHeight
		// Render
		return p.Render()
	}
	//
	return nil
}

// Render the inspector to the given terminal
func (p *Inspector) Render() error {
	return p.term.Render()
}

// Close the inspector.
func (p *Inspector) Close() error {
	return p.term.Restore()
}

// CurrentModule returns the currently selected module
func (p *Inspector) CurrentModule() *ModuleState {
	if p.tabs.HasSelected() {
		mid := p.tabs.Selected()
		//
		return &p.modules[mid]
	}
	//
	return &ModuleState{view: &emptyModuleView{}}
}

// EnterMode pushes a new mode onto the mode stack.
func (p *Inspector) EnterMode(mode Mode) {
	// Append mode to stack of active modes
	p.modes = append(p.modes, mode)
	// Activate mode
	mode.Activate(p)
}

// KeyPressed allows the inspector to react to a key being pressed by the user.
func (p *Inspector) KeyPressed(key uint16) bool {
	var n = len(p.modes) - 1
	//
	if p.modes[n].KeyPressed(p, key) {
		p.modes = p.modes[0:n]
		//
		if n > 0 {
			// Reactivate mode
			p.modes[n-1].Activate(p)
		}
	}
	// Exit when the mode stack is empty.
	return len(p.modes) == 0
}

// SetStatus puts a message on the status bar.  Messages remain visible for some
// number of clock cycles.
func (p *Inspector) SetStatus(msg termio.FormattedText) {
	p.statusBar.Clear()
	p.statusBar.AddLeft(msg)
	p.statusClk = 5
}

// change cell width in current module
func (p *Inspector) changeCellWidth(direction bool) {
	// Action change
	width := p.CurrentModule().view.Config().CellWidth()
	//
	if direction {
		width++
	} else if width > 2 {
		width--
	}
	//
	p.CurrentModule().view.Config().SetCellWidth(width)
	//
	p.SetStatus(termio.NewColouredText(fmt.Sprintf("Cell width now %d", width), termio.TERM_GREEN))
}

// change cell width in current module
func (p *Inspector) changeTitleWidth(direction bool) {
	// Action change
	width := p.CurrentModule().view.Config().TitleWidth()
	//
	if direction {
		width++
	} else if width > 2 {
		width--
	}
	//
	p.CurrentModule().view.Config().SetTitleWidth(width)
	//
	p.SetStatus(termio.NewColouredText(fmt.Sprintf("Title width now %d", width), termio.TERM_GREEN))
}

// Actions goto row mode
func (p *Inspector) gotoRow(row uint) termio.FormattedText {
	// Action change
	row = p.CurrentModule().gotoRow(row)
	//
	return termio.NewColouredText(fmt.Sprintf("At row %d", row), termio.TERM_GREEN)
}

// filter columns based on a regex
func (p *Inspector) filterColumns(regex *regexp.Regexp) termio.FormattedText {
	row, _ := p.CurrentModule().view.Offset()
	filter := p.CurrentModule().columnFilter
	filter.Regex = regex
	p.CurrentModule().applyColumnFilter(filter, true)
	// Success
	return p.gotoRow(row)
}

func (p *Inspector) clearColumnFilter() bool {
	row, _ := p.CurrentModule().view.Offset()
	filter := p.CurrentModule().columnFilter
	filter.Regex = nil
	p.CurrentModule().applyColumnFilter(filter, false)
	p.gotoRow(row)
	// Success
	return true
}

func (p *Inspector) toggleColumnFilter() bool {
	var (
		row, _ = p.CurrentModule().view.Offset()
		filter = p.CurrentModule().columnFilter
		msg    string
	)
	// Implement toggle semantics
	switch {
	case !filter.Computed && filter.UserDefined:
		filter.Computed = true
		filter.UserDefined = false
		msg = "Showing computed columns only"
	case filter.Computed && !filter.UserDefined:
		filter.UserDefined = true
		msg = "Showing all columns"
	case filter.Computed && filter.UserDefined:
		filter.Computed = false
		msg = "Showing non-computed columns only"
	}
	//
	p.CurrentModule().applyColumnFilter(filter, false)
	p.CurrentModule().gotoRow(row)
	p.SetStatus(termio.NewColouredText(msg, termio.TERM_GREEN))
	// Success
	return true
}

func (p *Inspector) nextScanResult(forwards bool) {
	var (
		current = p.CurrentModule()
		msg     termio.FormattedText
		col, _  = p.CurrentModule().view.Offset()
	)
	//
	if current.lastQuery == nil {
		msg = termio.NewColouredText("no previous scan", termio.TERM_RED)
	} else if forwards {
		// Dig out the last query
		msg = p.CurrentModule().matchQuery(col+1, forwards, current.lastQuery)
	} else {
		msg = p.CurrentModule().matchQuery(col-1, forwards, current.lastQuery)
	}
	//
	p.SetStatus(msg)
}

func (p *Inspector) matchQuery(query *Query) termio.FormattedText {
	var col, _ = p.CurrentModule().view.Offset()
	//
	return p.CurrentModule().matchQuery(col, true, query)
}

func (p *Inspector) toggleModuleVisibility() {
	p.visibility = !p.visibility
	p.tabs = initInspectorTabs(p.visibility, p.modules)
	p.term.Set(0, p.tabs)
}

// ==================================================================
// TableSource
// ==================================================================

// Dimensions implementation for the TableSource interface
func (p *Inspector) Dimensions() (uint, uint) {
	module := p.CurrentModule()
	//
	return module.view.Dimensions()
}

// ColumnWidth gets the width of a given column in the main table of the
// inspector.  Note that columns here are table columns, not trace columns.
func (p *Inspector) ColumnWidth(col uint) uint {
	module := p.CurrentModule()
	//
	return module.view.ColumnWidth(col)
}

// CellAt returns the contents of a given cell in the main table of the
// inspector.
func (p *Inspector) CellAt(col, row uint) termio.FormattedText {
	// Determine currently selected module
	var (
		state         = p.CurrentModule()
		width, height = state.view.Dimensions()
	)
	//
	if col >= width || row >= height {
		return termio.NewText("")
	}
	// Get cell out of module view
	return state.view.CellAt(col, row)
}

// Start provides a read / update / render loop.
func (p *Inspector) Start() []error {
	var errors []error
	// Start clock timer
	clk := time.NewTicker(500 * time.Millisecond)
	//
	go func() {
		for {
			// Receive clock signal
			<-clk.C
			// Force render
			//nolint:errcheck
			p.Clock()
		}
	}()
	//
	for {
		if key, err := p.term.ReadKey(); err != nil {
			errors = append(errors, err)
			break
		} else if exit := p.KeyPressed(key); exit {
			break
		}
		// Rerender window
		if err := p.Render(); err != nil {
			errors = append(errors, err)
			break
		}
	}
	// Attempt to restore terminal state
	if err := p.term.Restore(); err != nil {
		errors = append(errors, err)
	}
	// Done
	return errors
}

// ==================================================================
// Helpers
// ==================================================================

func initInspectorWidgets(term *termio.Terminal, states []ModuleState, showAllModules bool) (tabs *widget.Tabs[uint],
	table *widget.Table, cmdbar *widget.TextLine, statusbar *widget.TextLine) {
	//
	tabs = initInspectorTabs(showAllModules, states)
	table = widget.NewTable(nil)
	cmdbar = widget.NewText()
	statusbar = widget.NewText()
	//
	term.Add(tabs)
	term.Add(widget.NewSeparator("⎯"))
	term.Add(table)
	term.Add(widget.NewSeparator("⎯"))
	term.Add(cmdbar)
	term.Add(statusbar)
	//
	return tabs, table, cmdbar, statusbar
}

func initInspectorTabs(showAllModules bool, states []ModuleState) *widget.Tabs[uint] {
	var tabs []util.Pair[string, module.Id]

	for i, state := range states {
		if state.public || showAllModules {
			name := state.view.Data().Name().String()
			tabs = append(tabs, util.NewPair(name, uint(i)))
		}
	}
	//
	return widget.NewTabs(tabs...)
}

// Empty ModuleView is required for the unusual case that there are no modules
// currently visible.  This can arise if there are no public modules at all.
type emptyModuleView struct {
}

// Config implementation for ModuleView interface.
func (p *emptyModuleView) Config() view.ModuleConfig {
	panic("unsupported operation")
}

// Data implementation for ModuleView interface.
func (p *emptyModuleView) Data() view.ModuleData {
	panic("unsupported operation")
}

// Filter implementation for ModuleView interface.
func (p *emptyModuleView) Filter(view.ModuleFilter) view.ModuleView {
	return p
}

// Offset implementation for ModuleView interface.
func (p *emptyModuleView) Offset() (uint, uint) {
	return 0, 0
}

// Goto implementation for ModuleView interface.
func (p *emptyModuleView) Goto(uint, uint) {

}

// Window implementation for ModuleView interface.
func (p *emptyModuleView) Window() view.Window {
	panic("unsupported operation")
}

// ColumnWidth implementation for TableSource interface.
func (p *emptyModuleView) ColumnWidth(col uint) uint {
	return 0
}

// Dimensions implementation for TableSource interface.
func (p *emptyModuleView) Dimensions() (uint, uint) {
	return 0, 0
}

// CellAt implementation for TableSource interface.
func (p *emptyModuleView) CellAt(col, row uint) termio.FormattedText {
	return termio.NewText("")
}
