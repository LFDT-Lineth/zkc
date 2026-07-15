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
package zkc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// moduleStats captures the summary information gathered for a single module of
// the generated AIR schema.  Which fields are populated depends on the module
// kind: static modules report only their cell count; native modules report only
// their register count; all other modules report the full breakdown.
type moduleStats struct {
	name string
	// kind is one of "static", "native" or "" (a regular module).
	kind string
	// cells is the number of cells in a static reference table (rows ×
	// registers).  Only meaningful for static modules.
	cells uint
	// preSplit maps each register bitwidth (as declared in the bytecode program,
	// before register splitting) to the number of registers of that width.
	preSplit map[uint]uint
	// preNative is the number of native (field) registers, which have no
	// bitwidth and so are not counted in any preSplit width bucket.
	preNative uint
	// postRegs is the number of registers after splitting (i.e. in the AIR
	// schema).  For native modules this is the reported register count.
	postRegs uint
	// degrees maps each vanishing-constraint degree to the number of constraints
	// of that degree.
	degrees map[uint]uint
	// lookups is the number of lookup constraints.
	lookups uint
	// complexity is a cost measure: the number of constraints weighted by the
	// square of their degree (Σ degree²).
	complexity uint
}

// PrintCompileStats prints summary statistics about the modules of a generated
// AIR schema, one row per module.  Register widths (pre-splitting) and
// constraint degrees are gathered from the pre-split bytecode program (ir) and
// the post-split AIR schema respectively.
func PrintCompileStats[F field.Element[F]](air schema.AnySchema[F], ir vm.Program[vm.Uint]) {
	var (
		// Pre-split register histograms, keyed by module name.
		preSplit = preSplitRegisters(ir)
		stats    []moduleStats
	)
	// Gather per-module statistics from the AIR schema.
	for iter := air.Modules(); iter.HasNext(); {
		stats = append(stats, summariseAirModule(iter.Next(), preSplit))
	}
	//
	printAirModuleStats(stats)
}

// preSplitInfo captures the pre-splitting register breakdown for a single
// module of the bytecode program.
type preSplitInfo struct {
	// widths maps register bitwidth to the number of registers of that width.
	widths map[uint]uint
	// native is the number of native (field) registers, which have no bitwidth.
	native uint
}

// preSplitRegisters builds, for each module in the bytecode program, a histogram
// mapping register bitwidth to the number of registers of that width, plus a
// separate count of native (bitwidth-less) registers.
func preSplitRegisters(ir vm.Program[vm.Uint]) map[string]preSplitInfo {
	var info = make(map[string]preSplitInfo)
	//
	for _, m := range ir.Modules() {
		var entry = preSplitInfo{widths: make(map[uint]uint)}
		//
		for _, r := range m.Registers() {
			if bw := r.Bitwidth(); bw.HasValue() {
				entry.widths[bw.Unwrap()]++
			} else {
				entry.native++
			}
		}
		//
		info[m.Name()] = entry
	}
	//
	return info
}

// summariseAirModule gathers the summary statistics for a single AIR module.
func summariseAirModule[F field.Element[F]](mod schema.Module[F],
	preSplit map[string]preSplitInfo) moduleStats {
	//
	var stats = moduleStats{
		name:     mod.Name().String(),
		preSplit: make(map[uint]uint),
		degrees:  make(map[uint]uint),
	}
	//
	switch {
	case mod.IsStatic():
		// Static reference table: report only the number of cells.
		stats.kind = "static"
		stats.cells = uint(len(mod.StaticContents())) * mod.Width()
	case mod.IsNative():
		// Native circuit: report only the number of registers.
		stats.kind = "native"
		stats.postRegs = mod.Width()
	default:
		// Regular module: report the full breakdown.
		info := preSplit[mod.Name().Name]
		stats.preSplit = info.widths
		stats.preNative = info.native
		stats.postRegs = mod.Width()
		//
		for iter := mod.Constraints(); iter.HasNext(); {
			switch c := iter.Next().(type) {
			case air.VanishingConstraint[F]:
				degree := c.Complexity()
				stats.degrees[degree]++
				stats.complexity += degree * degree
			case air.LookupConstraint[F]:
				stats.lookups++
			}
		}
	}
	//
	return stats
}

// statsColumn describes a single column of the statistics table, including its
// two-row header and the per-module data cells.
type statsColumn struct {
	// group is the meta-header label spanning this column and its neighbours
	// (e.g. "Registers"); empty for standalone columns.
	group string
	// top is the header shown on the first row for a standalone column (e.g.
	// "Total (post splitting)"); empty for grouped columns.
	top string
	// sub is the header shown on the second row (e.g. "u16"); empty for
	// standalone columns whose label lives on the first row.
	sub string
	// cells holds the rendered contents, one per module row.
	cells []string
	// leftAlign left-justifies the column (used for the module name).
	leftAlign bool
	// width is the computed content width (excluding padding/separators).
	width uint
}

// printAirModuleStats renders the gathered per-module statistics as a single
// table with one row per module.  The register and constraint columns are
// grouped under "Registers" and "Constraints" meta-headers spanning their
// sub-columns; the remaining columns carry their label on the first header row.
func printAirModuleStats(stats []moduleStats) {
	var (
		// Union of register widths and constraint degrees across all modules,
		// used to determine the (dynamic) set of sub-columns.
		widths  = collectKeys(stats, func(m moduleStats) map[uint]uint { return m.preSplit })
		degrees = collectKeys(stats, func(m moduleStats) map[uint]uint { return m.degrees })
		cols    []*statsColumn
	)
	// Module name (left-aligned).
	cols = append(cols, dataColumn(stats, "", "", "Module", true,
		func(m moduleStats) string { return m.name }))
	// Registers (pre-splitting), one sub-column per width, then native registers.
	for _, w := range widths {
		cols = append(cols, regularColumn(stats, "Registers", fmt.Sprintf("u%d", w),
			func(m moduleStats) uint { return m.preSplit[w] }))
	}
	//
	cols = append(cols, regularColumn(stats, "Registers", "native",
		func(m moduleStats) uint { return m.preNative }))
	// Total registers post-splitting (regular and native modules).
	cols = append(cols, dataColumn(stats, "", "Total", "(post splitting)", false,
		func(m moduleStats) string {
			if m.kind == "" || m.kind == "native" {
				return count(m.postRegs)
			}
			//
			return ""
		}))
	// Constraints by degree, one sub-column per degree.
	for _, d := range degrees {
		cols = append(cols, regularColumn(stats, "Constraints", fmt.Sprintf("d%d", d),
			func(m moduleStats) uint { return m.degrees[d] }))
	}
	// Lookups, complexity (regular modules only).
	cols = append(cols, dataColumn(stats, "", "lookups", "", false,
		func(m moduleStats) string {
			if m.kind == "" {
				return count(m.lookups)
			}
			//
			return ""
		}))
	cols = append(cols, dataColumn(stats, "", "complexity", "(= sum d^2)", false,
		func(m moduleStats) string {
			if m.kind == "" {
				return count(m.complexity)
			}
			//
			return ""
		}))
	// Cells (static modules only).
	cols = append(cols, dataColumn(stats, "", "cells", "(static tables)", false,
		func(m moduleStats) string {
			if m.kind == "static" {
				return count(m.cells)
			}
			//
			return ""
		}))
	//
	renderStatsTable(cols, len(stats))
}

// regularColumn builds a grouped data column whose cells hold a (possibly zero)
// count derived from each module, blank for static/native modules.
func regularColumn(stats []moduleStats, group, sub string, get func(moduleStats) uint) *statsColumn {
	return dataColumn(stats, group, "", sub, false, func(m moduleStats) string {
		if m.kind != "" {
			return ""
		}
		//
		return count(get(m))
	})
}

// dataColumn builds a column, rendering each module's cell via get and computing
// the column's content width from its headers and cells.
func dataColumn(stats []moduleStats, group, top, sub string, leftAlign bool,
	get func(moduleStats) string) *statsColumn {
	//
	var col = statsColumn{group: group, top: top, sub: sub, leftAlign: leftAlign}
	// Standalone columns carry their (wider) label on the first row.
	col.width = max(uint(len([]rune(top))), uint(len([]rune(sub))))
	//
	for _, m := range stats {
		cell := get(m)
		col.cells = append(col.cells, cell)
		col.width = max(col.width, uint(len([]rune(cell))))
	}
	//
	return &col
}

// renderStatsTable prints the assembled columns: a meta-header row (with grouped
// labels spanning their sub-columns), a sub-header row, a horizontal rule, a
// totals row summing every column, then one row per module.
func renderStatsTable(cols []*statsColumn, nrows int) {
	// Compute the totals row (summing each column over all modules) up front so
	// its width is accounted for when sizing columns.
	totals := totalsRow(cols)
	// Grow member columns so each group's span is wide enough for its label.
	fitGroupLabels(cols)
	// Total rendered width of the table (each column contributes its content
	// width plus a leading space, trailing space and separator).
	var total uint
	for _, c := range cols {
		total += c.width + 3
	}
	//
	rule := strings.Repeat("-", int(total))
	// Top rule.
	fmt.Println(rule)
	// Meta-header row: group labels span their members; standalone labels sit in
	// their own column.
	for i := 0; i < len(cols); {
		if group := cols[i].group; group != "" {
			// Emit the group label centred across the whole span of members.
			j := i
			for j < len(cols) && cols[j].group == group {
				j++
			}
			//
			fmt.Print(" " + center(group, spanWidth(cols[i:j])) + " |")
			i = j
		} else {
			fmt.Print(" " + center(cols[i].top, cols[i].width) + " |")
			i++
		}
	}
	//
	fmt.Println()
	// Sub-header row.
	for _, c := range cols {
		fmt.Print(" " + align(c.sub, c.width, c.leftAlign) + " |")
	}
	//
	fmt.Println()
	// Horizontal rule.
	fmt.Println(rule)
	// Totals row, followed by a rule separating it from the per-module rows.
	for i, c := range cols {
		fmt.Print(" " + align(totals[i], c.width, c.leftAlign) + " |")
	}
	//
	fmt.Println()
	fmt.Println(rule)
	// Data rows.
	for r := 0; r < nrows; r++ {
		for _, c := range cols {
			fmt.Print(" " + align(c.cells[r], c.width, c.leftAlign) + " |")
		}
		//
		fmt.Println()
	}
}

// totalsRow computes the totals row, summing the numeric cells of each column
// over all modules.  The first (module-name) column is labelled "Total".  Column
// widths are widened to fit the computed totals.
func totalsRow(cols []*statsColumn) []string {
	var totals = make([]string, len(cols))
	//
	for i, c := range cols {
		if i == 0 {
			totals[i] = "Total"
		} else {
			var sum uint
			//
			for _, cell := range c.cells {
				if n, err := strconv.Atoi(cell); err == nil {
					sum += uint(n)
				}
			}
			//
			totals[i] = count(sum)
		}
		//
		c.width = max(c.width, uint(len([]rune(totals[i]))))
	}
	//
	return totals
}

// fitGroupLabels widens the last member of each group so the group's span is at
// least as wide as its meta-header label.
func fitGroupLabels(cols []*statsColumn) {
	for i := 0; i < len(cols); {
		group := cols[i].group
		if group == "" {
			i++
			continue
		}
		//
		j := i
		for j < len(cols) && cols[j].group == group {
			j++
		}
		//
		if deficit := len([]rune(group)) - int(spanWidth(cols[i:j])); deficit > 0 {
			cols[j-1].width += uint(deficit)
		}
		//
		i = j
	}
}

// spanWidth returns the content width available to a merged header cell spanning
// the given member columns, accounting for the padding and separators that would
// otherwise sit between them.
func spanWidth(members []*statsColumn) uint {
	var width uint
	//
	for _, c := range members {
		width += c.width
	}
	// Each internal boundary contributes two padding spaces plus a separator.
	return width + uint(3*(len(members)-1))
}

// center pads s with spaces so it is centred within width.
func center(s string, width uint) string {
	n := len([]rune(s))
	if uint(n) >= width {
		return s
	}
	//
	left := (int(width) - n) / 2
	right := int(width) - n - left
	//
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// align pads s to width, left- or right-justified.
func align(s string, width uint, left bool) string {
	pad := int(width) - len([]rune(s))
	if pad <= 0 {
		return s
	}
	//
	if left {
		return s + strings.Repeat(" ", pad)
	}
	//
	return strings.Repeat(" ", pad) + s
}

// collectKeys returns the sorted union of the keys of the histogram selected by
// key(m) across all modules.
func collectKeys(stats []moduleStats, key func(moduleStats) map[uint]uint) []uint {
	var seen = make(map[uint]bool)
	//
	for _, m := range stats {
		for k := range key(m) {
			seen[k] = true
		}
	}
	//
	keys := make([]uint, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	//
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	//
	return keys
}

// count renders a count as text, showing an empty cell for a zero count so that
// the populated numbers stand out.
func count(n uint) string {
	if n == 0 {
		return ""
	}
	//
	return fmt.Sprintf("%d", n)
}
