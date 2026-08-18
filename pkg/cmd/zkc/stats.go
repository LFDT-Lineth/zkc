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
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// moduleStats captures the summary information gathered for a single module of
// the generated AIR schema.  Which fields are populated depends on the module
// kind: static modules report only their cell count; native modules report only
// their register count; all other modules report the full breakdown.
type moduleStats struct {
	name string
	// typ is the display module type: "function", "native", one of "RAM"/"ROM"/"WOM"
	// for memories, or "static".
	typ string
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
	// pcMax is the maximum program-counter value for a (non-native) function,
	// i.e. the number of bytecode lines (indices plus the halt value).  Zero for
	// modules without a program counter.
	pcMax uint
	// degrees maps each vanishing-constraint degree to the number of constraints
	// of that degree.
	degrees map[uint]uint
	// lookups is the number of lookup constraints.
	lookups uint
	// complexity is a cost measure: each constraint weighted by the square of
	// its degree times the number of distinct columns it reads
	// (Σ nCols · degree²).
	complexity uint
}

// isStatic reports whether this is a static reference table.
func (m moduleStats) isStatic() bool { return m.typ == "static" }

// isNative reports whether this is a native circuit.
func (m moduleStats) isNative() bool { return m.typ == "native" }

// isRegular reports whether this is a regular module (a function or memory),
// i.e. neither static nor native.  Regular modules report the full breakdown.
func (m moduleStats) isRegular() bool { return !m.isStatic() && !m.isNative() }

// bucket groups a range of keys (register widths or constraint degrees) into a
// single column of the stats table.  The range is inclusive.
type bucket struct {
	label  string
	lo, hi uint
}

// registerBuckets defines the (fixed) width buckets used for the pre-split
// register columns.  The first bucket's lo of 0 folds the trivial 0-bit
// (constant) registers into "u1".
var registerBuckets = []bucket{
	{"u1", 0, 1},
	{"u2-u16", 2, 16},
	{"u17-u32", 17, 32},
	{"u33-u64", 33, 64},
	{"u65+", 65, ^uint(0)},
}

// degreeBuckets defines the (fixed) degree buckets used for the constraint
// columns; degrees of 8 or more are aggregated into "d8+".
var degreeBuckets = []bucket{
	{"d1", 0, 1},
	{"d2", 2, 2},
	{"d3", 3, 3},
	{"d4", 4, 4},
	{"d5", 5, 5},
	{"d6", 6, 6},
	{"d7", 7, 7},
	{"d8+", 8, ^uint(0)},
}

// closeFinalBucket returns a copy of the given buckets in which the final,
// open-ended bucket's label is closed off with the actual maximum key present
// (e.g. "u65-u80" or "d8-d12").  When nothing reaches the final bucket its
// original ("+") label is kept.
func closeFinalBucket(buckets []bucket, prefix string, maxKey uint) []bucket {
	out := append([]bucket(nil), buckets...)
	last := &out[len(out)-1]
	//
	if maxKey >= last.lo {
		last.label = fmt.Sprintf("%s%d-%s%d", prefix, last.lo, prefix, maxKey)
	}
	//
	return out
}

// maxKey returns the largest key found across the histograms selected by sel
// over all modules (0 if none).
func maxKey(stats []moduleStats, sel func(moduleStats) map[uint]uint) uint {
	var m uint
	//
	for _, s := range stats {
		for k := range sel(s) {
			m = max(m, k)
		}
	}
	//
	return m
}

// bucketCount sums the counts in hist whose key falls within bucket b.
func bucketCount(hist map[uint]uint, b bucket) uint {
	var n uint
	//
	for k, c := range hist {
		if k >= b.lo && k <= b.hi {
			n += c
		}
	}
	//
	return n
}

// PrintCompileStats prints summary statistics about the modules of a generated
// AIR schema, one row per module.  Register widths (pre-splitting) and
// constraint degrees are gathered from the pre-split bytecode program (ir) and
// the post-split AIR schema respectively.  The order argument determines how the
// modules are ordered (see orderModules).
func PrintCompileStats[F field.Element[F], W vm.Word[W]](air schema.AnySchema[F], ir vm.Program[W], order string) {
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
	orderModules(stats, order)
	printAirModuleStats(stats)
}

// ValidStatsOrder reports whether order is a recognised --stats ordering key.
func ValidStatsOrder(order string) bool {
	switch order {
	case "name", "total", "complexity", "lookups":
		return true
	default:
		return false
	}
}

// orderModules reorders stats in place.  Modules are grouped primarily by type,
// in the order function, native, RAM, ROM, WOM, static.  Within each group they
// are ordered by the given key (name|total|complexity|lookups); static tables,
// for which those keys are not meaningful, are always ordered by cell count
// (largest first).
func orderModules(stats []moduleStats, order string) {
	sort.SliceStable(stats, func(i, j int) bool {
		a, b := stats[i], stats[j]
		// Primary: group by type.
		if ra, rb := typeRank(a.typ), typeRank(b.typ); ra != rb {
			return ra < rb
		}
		// Secondary: order within the group.
		if a.typ == "static" {
			return a.cells > b.cells
		}
		//
		return lessByOrder(a, b, order)
	})
}

// typeRank gives the ordering position of each module type: functions first,
// then native modules, then memories (RAM, ROM, WOM), then static tables last.
func typeRank(typ string) int {
	switch typ {
	case "function":
		return 0
	case "native":
		return 1
	case "RAM":
		return 2
	case "ROM":
		return 3
	case "WOM":
		return 4
	case "static":
		return 5
	default:
		return 6
	}
}

// lessByOrder reports whether module a should sort before module b under the
// given ordering key.  Numeric keys sort largest first; unknown keys fall back
// to "total".
func lessByOrder(a, b moduleStats, order string) bool {
	switch order {
	case "name":
		return a.name < b.name
	case "complexity":
		return a.complexity > b.complexity
	case "lookups":
		return a.lookups > b.lookups
	default: // "total"
		return a.postRegs > b.postRegs
	}
}

// preSplitInfo captures the pre-splitting register breakdown for a single
// module of the bytecode program.
type preSplitInfo struct {
	// typ is the module type: "function", "memory", "native" or "static".
	typ string
	// widths maps register bitwidth to the number of registers of that width.
	widths map[uint]uint
	// native is the number of native (field) registers, which have no bitwidth.
	native uint
	// pcMax is the maximum program-counter value for a (non-native) function.
	pcMax uint
}

// preSplitRegisters builds, for each module in the bytecode program, its type
// and a histogram mapping register bitwidth to the number of registers of that
// width, plus a separate count of native (bitwidth-less) registers.
func preSplitRegisters[W vm.Word[W]](ir vm.Program[W]) map[string]preSplitInfo {
	var info = make(map[string]preSplitInfo)
	//
	for _, m := range ir.Modules() {
		var entry = preSplitInfo{typ: moduleType(m), widths: make(map[uint]uint)}
		// The maximum program-counter value is the number of bytecode lines (line
		// indices plus the one-past-the-end halt value).  Only non-native
		// functions carry a program counter.
		if fn, ok := m.(*vm.Function[W]); ok && !fn.IsNative() {
			entry.pcMax = uint(len(fn.Vectors()))
		}
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

// moduleType classifies a bytecode module: a "function" (possibly "native"), or
// a memory by kind ("static", "ROM" read-only, "WOM" write-once, "RAM"
// read-write).
func moduleType[W vm.Word[W]](m vm.Module[W]) string {
	switch m := m.(type) {
	case *vm.Function[W]:
		if m.IsNative() {
			return "native"
		}
		//
		return "function"
	case *vm.Memory[W]:
		switch {
		case m.IsStatic():
			return "static"
		case m.IsReadOnly():
			return "ROM"
		case m.IsWriteOnly():
			return "WOM"
		default:
			return "RAM"
		}
	default:
		return ""
	}
}

// isGenerated reports whether a module is compiler-generated (rather than
// user-defined).  Generated modules use a "$" name prefix (e.g. "$range_u8").
func isGenerated(name string) bool {
	return strings.HasPrefix(name, "$")
}

// summariseAirModule gathers the summary statistics for a single AIR module.
func summariseAirModule[F field.Element[F]](mod schema.Module[F],
	preSplit map[string]preSplitInfo) moduleStats {
	//
	var stats = moduleStats{
		name:     mod.Name(),
		preSplit: make(map[uint]uint),
		degrees:  make(map[uint]uint),
		pcMax:    preSplit[mod.Name()].pcMax,
	}
	//
	switch {
	case mod.IsStatic():
		// Static reference table: report only the number of cells.
		stats.typ = "static"
		stats.cells = uint(len(mod.StaticContents())) * mod.Width()
	case mod.IsNative():
		// Native circuit: report only the number of registers.
		stats.typ = "native"
		stats.postRegs = mod.Width()
	default:
		// Regular module (function or memory): report the full breakdown.
		info := preSplit[mod.Name()]
		stats.typ = info.typ
		stats.postRegs = mod.Width()
		// Pre-split register widths are only meaningful (and field-independent)
		// for user-defined modules.  Compiler-generated modules (e.g. the
		// recursive $range_* range checkers) have no pre-split form and their
		// register widths depend on the field, so leave those columns blank.
		if !isGenerated(mod.Name()) {
			stats.preSplit = info.widths
			stats.preNative = info.native
		}
		//
		for iter := mod.Constraints(); iter.HasNext(); {
			switch c := iter.Next().(type) {
			case air.VanishingConstraint[F]:
				degree := c.Complexity()
				stats.degrees[degree]++
				stats.complexity += numColumns(c) * degree * degree
			case air.LookupConstraint[F]:
				stats.lookups++
			}
		}
	}
	//
	return stats
}

// numColumns returns the number of distinct columns read by a vanishing
// constraint, where a shifted access counts as a column of its own:
// - A     · (1 - A) = 0 → 1 cols
// - A[-1] · (1 - A) = 0 → 2 cols
// - A     · (1 - B) = 0 → 2 cols
func numColumns[F field.Element[F]](c air.VanishingConstraint[F]) uint {
	return uint(len(*c.Unwrap().Constraint.RequiredCells(0, 0)))
}

// statsColumn describes a single column of the statistics table, including its
// two-row header and the per-module data cells.
type statsColumn struct {
	// group is the meta-header label spanning this column and its neighbours
	// (e.g. "Registers (pre splitting)"); empty for standalone columns.
	group string
	// top is the header shown on the first row for a standalone column (e.g.
	// "complexity"); empty for grouped columns.
	top string
	// sub is the header shown on the second row (e.g. "u16"); empty for
	// standalone columns whose label lives on the first row.
	sub string
	// cells holds the rendered contents, one per module row.
	cells []string
	// leftAlign left-justifies the column (used for the module name).
	leftAlign bool
	// maxAgg makes the per-type totals row report the maximum of this column
	// rather than the sum (used for PC_max).
	maxAgg bool
}

// printAirModuleStats renders the gathered per-module statistics as a single
// table with one row per module.  The register and constraint columns are
// grouped under "Registers" and "Constraints" meta-headers spanning their
// sub-columns; the remaining columns carry their label on the first header row.
func printAirModuleStats(stats []moduleStats) {
	var cols []*statsColumn
	// Give the open-ended final buckets an upper bound reflecting the largest
	// register width / constraint degree actually present.
	maxWidth := maxKey(stats, func(m moduleStats) map[uint]uint { return m.preSplit })
	maxDegree := maxKey(stats, func(m moduleStats) map[uint]uint { return m.degrees })
	regBuckets := closeFinalBucket(registerBuckets, "u", maxWidth)
	degBuckets := closeFinalBucket(degreeBuckets, "d", maxDegree)
	// Module name and type (left-aligned).
	cols = append(cols, dataColumn(stats, "", "", "Module", true,
		func(m moduleStats) string { return m.name }))
	cols = append(cols, dataColumn(stats, "", "", "type", true,
		func(m moduleStats) string { return m.typ }))
	// Maximum program-counter value (functions only); totalled as a max, not sum.
	pcCol := dataColumn(stats, "", "PC_max", "", false,
		func(m moduleStats) string { return count(m.pcMax) })
	pcCol.maxAgg = true
	cols = append(cols, pcCol)
	// Registers (pre-splitting), bucketed by width, then native registers.
	for _, b := range regBuckets {
		b := b
		cols = append(cols, regularColumn(stats, "Registers (pre splitting)", b.label,
			func(m moduleStats) uint { return bucketCount(m.preSplit, b) }))
	}
	//
	cols = append(cols, regularColumn(stats, "Registers (pre splitting)", "native",
		func(m moduleStats) uint { return m.preNative }))
	// Total registers post-splitting, i.e. the number of limbs (regular and
	// native modules).
	cols = append(cols, dataColumn(stats, "", "Post", "Split", false,
		func(m moduleStats) string {
			if !m.isStatic() {
				return count(m.postRegs)
			}
			//
			return ""
		}))
	// Vanishing constraints bucketed by degree.  Note this is specifically the
	// vanishing-constraint breakdown; other AIR constraint kinds (range,
	// permutation, ...) are not counted here (lookups have their own column).
	for _, b := range degBuckets {
		b := b
		cols = append(cols, regularColumn(stats, "Vanishing constraints (by degree)", b.label,
			func(m moduleStats) uint { return bucketCount(m.degrees, b) }))
	}
	// Lookups, complexity (regular modules only).
	cols = append(cols, dataColumn(stats, "", "lookups", "", false,
		func(m moduleStats) string {
			if m.isRegular() {
				return count(m.lookups)
			}
			//
			return ""
		}))
	cols = append(cols, dataColumn(stats, "", "complexity", "(= sum n.d^2)", false,
		func(m moduleStats) string {
			if m.isRegular() {
				return count(m.complexity)
			}
			//
			return ""
		}))
	// Cells (static modules only).
	cols = append(cols, dataColumn(stats, "", "cells", "(static tables)", false,
		func(m moduleStats) string {
			if m.isStatic() {
				return count(m.cells)
			}
			//
			return ""
		}))
	//
	renderStatsTable(cols, stats)
}

// regularColumn builds a grouped data column whose cells hold a (possibly zero)
// count derived from each module, blank for static/native modules.
func regularColumn(stats []moduleStats, group, sub string, get func(moduleStats) uint) *statsColumn {
	return dataColumn(stats, group, "", sub, false, func(m moduleStats) string {
		if !m.isRegular() {
			return ""
		}
		//
		return count(get(m))
	})
}

// dataColumn builds a column, rendering each module's cell via get.
func dataColumn(stats []moduleStats, group, top, sub string, leftAlign bool,
	get func(moduleStats) string) *statsColumn {
	//
	var col = statsColumn{group: group, top: top, sub: sub, leftAlign: leftAlign}
	//
	for _, m := range stats {
		col.cells = append(col.cells, get(m))
	}
	//
	return &col
}

// renderStatsTable renders the assembled columns using a termio table: a
// meta-header row (with grouped labels spanning their sub-columns), a sub-header
// row, one totals row per module type, then one row per module, all delimited by
// horizontal rules.
func renderStatsTable(cols []*statsColumn, stats []moduleStats) {
	var (
		totals = typeTotals(cols, stats)
		ncols  = uint(len(cols))
		nrows  = uint(2 + len(totals) + len(stats))
		tbl    = termio.NewFormattedTable(ncols, nrows)
	)
	// Column alignment.
	for i, c := range cols {
		if c.leftAlign {
			tbl.SetLeftAlign(uint(i))
		}
	}
	// Sub-header row (row 1).
	for i, c := range cols {
		tbl.Set(uint(i), 1, termio.NewText(c.sub))
	}
	// Data region: per-type totals rows, then one row per module.
	row := uint(2)
	//
	for _, totalRow := range totals {
		for i := range cols {
			tbl.Set(uint(i), row, termio.NewText(totalRow[i]))
		}
		//
		row++
	}
	//
	for r := range stats {
		for i, c := range cols {
			tbl.Set(uint(i), row, termio.NewText(c.cells[r]))
		}
		//
		row++
	}
	// Meta-header row (row 0), set last so column widths are already known: group
	// labels span their members; standalone labels sit in their own column.
	for i := 0; i < len(cols); {
		if group := cols[i].group; group != "" {
			j := i
			for j < len(cols) && cols[j].group == group {
				j++
			}
			//
			tbl.SetSpan(uint(i), 0, uint(j-i), termio.NewText(group))
			i = j
		} else {
			tbl.Set(uint(i), 0, termio.NewText(cols[i].top))
			i++
		}
	}
	// Rules: top, after the header rows, after the totals, and at the bottom.
	tbl.SetRule(0)
	tbl.SetRule(2)
	tbl.SetRule(2 + uint(len(totals)))
	tbl.SetRule(nrows)
	//
	tbl.Print(true)
}

// typeTotals computes one totals row per module type present, in type order,
// aggregating the numeric cells of each column over the modules of that type.
// Columns are summed, except those flagged maxAgg (e.g. PC_max) which report the
// maximum.  Each row is labelled "Total" in the module column and carries the
// type in the type column.
func typeTotals(cols []*statsColumn, stats []moduleStats) [][]string {
	var (
		order = []string{"function", "native", "RAM", "ROM", "WOM", "static"}
		rows  [][]string
	)
	//
	for _, typ := range order {
		// Skip types with no modules.
		if !containsType(stats, typ) {
			continue
		}
		//
		row := make([]string, len(cols))
		row[0] = "Total"
		row[1] = typ
		// Aggregate each remaining column over the modules of this type.
		for i := 2; i < len(cols); i++ {
			var agg uint
			//
			for r, m := range stats {
				if m.typ != typ {
					continue
				}
				//
				if n, err := strconv.Atoi(cols[i].cells[r]); err == nil {
					if cols[i].maxAgg {
						agg = max(agg, uint(n))
					} else {
						agg += uint(n)
					}
				}
			}
			//
			row[i] = count(agg)
		}
		//
		rows = append(rows, row)
	}
	//
	return rows
}

// containsType reports whether any module in stats has the given type.
func containsType(stats []moduleStats, typ string) bool {
	for _, m := range stats {
		if m.typ == typ {
			return true
		}
	}
	//
	return false
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
