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
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var traceCmd = &cobra.Command{
	Use:   "trace [flags] input.json file1.zkc file2.zkc ...",
	Short: "Trace a zkc program.",
	Long: `Trace a zkc program to produce a set of outputs from a given set of inputs.

This is equivalent to "execute" but always generates a trace: it does not
support fast mode or checkpointing.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, traceCmds)
	},
}

// Available instances
var traceCmds = []FieldAgnosticCmd{
	{field.GF_251, runTraceCmd[gf251.Element]},
	{field.GF_8209, runTraceCmd[gf8209.Element]},
	{field.KOALABEAR_16, runTraceCmd[koalabear.Element]},
	{field.BLS12_377, runTraceCmd[bls12_377.Element]},
}

// Permitted flag combinations
var traceFlags FlagChecks

func runTraceCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build = GetBuildConfig[F](cmd, field)
		// outputFile file for trace
		outputFile = GetString(cmd, "output")
		// check constraints
		check = GetFlag(cmd, "check")
		// show trace statistics
		stats = GetFlag(cmd, "stats")
		// open trace in the interactive inspector
		inspect = GetFlag(cmd, "inspect")
		// extract sharding config
		sharding = GetString(cmd, "sharding")
		//
		trace   trace.Trace[F]
		outputs map[string][]byte
	)
	// Sanity permitted flag combinations
	checkFlags(cmd, traceFlags)
	// Tracing always generates a trace, so fast mode is not supported.
	if GetFlag(cmd, "fast") {
		fmt.Println("error: \"trace\" does not support fast mode (use \"execute\" instead)")
		os.Exit(1)
	}
	// Configure tracing
	traceConfig := vm.DEFAULT_TRACE_CONFIG.
		WithPadding(build.padding).
		WithBatchSize(GetUint(cmd, "batch")).
		WithParallelism(!GetFlag(cmd, "sequential"))
		//
	if sharding != "" {
		traceConfig = traceConfig.WithSharding(parseShardingConfig(sharding))
	}
	// Build artifacts (compiles source files or loads a prebuilt binary).
	_, binfile := Build[F](build, args[1:]...)
	// =====================================================
	// Trace
	// =====================================================
	// Parse and filter input file
	input := filterInputs(binfile.RawProgram(), ParseInputFile(args[0]))
	// Always trace (no fast mode).  The raw (row-major) trace is retained for
	// statistics, since it carries the original register/limb structure.
	outputs, trace, errors := binfile.Trace(input, traceConfig)
	// =====================================================
	// Generate output
	// =====================================================
	// Write outputs
	for name, bytes := range outputs {
		fmt.Printf("%s = 0x%s\n", name, hex.EncodeToString(bytes))
	}
	// print trace statistics (if requested).  Only meaningful when a trace was
	// actually generated (i.e. no execution errors).
	if stats && len(errors) == 0 {
		printTraceStats(trace)
		printModuleStats(trace)
	}
	// write out trace (if requested)
	if outputFile != "" {
		// Write out trace file
		WriteTraceFile(outputFile, trace)
	}
	// =====================================================
	// Check Constraints
	// =====================================================
	if check && trace != nil {
		checkConstraints(binfile, trace, traceConfig)
	}
	// =====================================================
	// Inspect
	// =====================================================
	// Open the generated trace in the interactive inspector (if requested).  This
	// takes over the terminal, so it runs last, after any stdout output above.
	if inspect && trace != nil {
		// Real ZkC functions are public; synthetic modules (e.g. range-check
		// tables) are private (hidden by default in the inspector).
		errors = corset.InspectTrace(binfile.LimbsMap(), trace, publicModule, false, 32, 128)
	}
	// =====================================================
	// Report Execution Failures
	// =====================================================
	if len(errors) > 0 {
		// Log errors
		for _, e := range errors {
			log.Error(fmt.Sprintf("%s", e))
		}
		//
		os.Exit(4)
	}
}

//nolint:errcheck
func init() {
	rootCmd.AddCommand(traceCmd)
	traceCmd.Flags().StringP("output", "o", "", "specify output file for writing trace")
	traceCmd.Flags().String("sharding", "", "specify sharding strategy")
	traceCmd.Flags().BoolP("check", "c", false, "check generated trace against constraints")
	traceCmd.Flags().Bool("stats", false, "show overall stats for the generated trace")
	traceCmd.Flags().Bool("sequential", false, "force sequential tracing")
	traceCmd.Flags().BoolP("inspect", "i", false, "open the generated trace in the interactive inspector")
	traceCmd.PersistentFlags().UintP("batch", "b", 1024, "specify batch size for constraint checking")
}

func parseShardingConfig(spec string) vm.ShardingStrategy {
	var (
		colon = strings.LastIndex(spec, ":")
	)
	//
	if colon < 0 {
		fmt.Printf("invalid sharding spec \"%s\" (expected \"<function>:<n>\")\n", spec)
		os.Exit(1)
	}
	//
	fn, istr := spec[:colon], spec[colon+1:]
	n, err := strconv.ParseUint(istr, 10, 64)
	//
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	//
	return vm.NewShardingStrategy(fn, n)
}

// publicModule reports whether a module is publicly visible in the inspector.
// Real ZkC functions and memories are public; synthetic modules generated during
// compilation (e.g. range-check tables such as "$range_u16") are private, so the
// inspector hides them by default.  Synthetic modules are named with a "$" sigil,
// which cannot appear in user-written identifiers (see the ZkC lexer), making the
// prefix a reliable marker.  Note the AIR schema does not set the IsSynthetic
// flag for these modules, so the name is the only discriminator available.
func publicModule(name module.Name) bool {
	return !strings.HasPrefix(name, "$")
}

// Column bit-width buckets reported by printTraceStats, matching those shown by
// the corset "trace --stats" command.
var traceStatBuckets = []struct{ lo, hi uint }{{1, 8}, {9, 16}, {17, 32}, {33, 128}, {129, 256}}

const (
	oneK = 1000
	oneM = oneK * oneK
	oneG = oneM * oneK
)

// printTraceStats prints overall statistics for a raw (row-major) trace, much
// like the corset "trace --stats" command.  It reports the total number of
// traced cells (both human-readable and raw) plus a breakdown of columns by
// bit-width.  Columns backed by field elements (i.e. native registers, which
// have no fixed bit-width) are reported separately.
func printTraceStats[F field.Element[F]](rtr trace.Trace[F]) {
	var (
		cells  uint
		counts = make([]uint, len(traceStatBuckets))
		native uint
	)
	// Tally cells and per-column bit-widths across all modules.
	for mid := range rtr.Width() {
		mod := rtr.Module(mid)
		cells += mod.Width() * mod.Height()
		//
		for _, reg := range mod.Descriptor().Columns {
			bitwidth := reg.Bitwidth
			// Native (field-element) limbs have no fixed bit-width.
			if bitwidth.IsEmpty() {
				native++
				continue
			}
			// Otherwise, place the limb in its matching bit-width bucket.
			for i, b := range traceStatBuckets {
				if w := bitwidth.Unwrap(); w >= b.lo && w <= b.hi {
					counts[i]++
					break
				}
			}
		}
	}
	// Assemble the stats table.
	rows := [][2]string{
		{"Cells", humanCount(cells)},
		{"Cells (raw)", fmt.Sprintf("%d", cells)},
	}
	//
	for i, b := range traceStatBuckets {
		rows = append(rows, [2]string{fmt.Sprintf("Columns (%d..%d bits)", b.lo, b.hi), fmt.Sprintf("%d", counts[i])})
	}
	//
	if native > 0 {
		rows = append(rows, [2]string{"Columns (native)", fmt.Sprintf("%d", native)})
	}
	// Render it.
	tbl := termio.NewFormattedTable(2, uint(len(rows)))
	//
	for i, row := range rows {
		tbl.SetRow(uint(i), termio.NewText(row[0]), termio.NewText(row[1]))
	}
	//
	tbl.SetMaxWidths(64)
	tbl.Print(AnsiEscapes)
}

// humanCount formats a (potentially large) count using K/M/G suffixes, matching
// the corset trace command's cell-count formatting.
func humanCount(total uint) string {
	switch {
	case total > oneG:
		return fmt.Sprintf("%.01fG", float64(total)/oneG)
	case total > oneM:
		return fmt.Sprintf("%.01fM", float64(total)/oneM)
	case total > oneK:
		return fmt.Sprintf("%.01fK", float64(total)/oneK)
	default:
		return fmt.Sprintf("%d", total)
	}
}

// Per-module summary column titles, matching those shown by the corset "trace
// --modules" command.
var moduleStatTitles = []string{"columns", "lines", "bitwidth", "cells", "nonzero", "bytes"}

// printModuleStats prints a per-module summary for a raw (row-major) trace, much
// like the corset trace command's module listing.  For each module it reports
// the column count, line (row) count, total bit-width, total cells, non-zero
// cells and total bytes.  Native (field-element) limbs, which have no fixed
// bit-width, are excluded from the bit-width and byte totals.
func printModuleStats[F field.Element[F]](rtr trace.Trace[F]) {
	var (
		n   = rtr.Width()
		tbl = termio.NewFormattedTable(uint(len(moduleStatTitles))+1, n+1)
	)
	// Set column titles (leaving the top-left cell blank, as corset does).
	for i, title := range moduleStatTitles {
		tbl.Set(uint(i)+1, 0, termio.NewText(title))
	}
	// Compute a summary row for each module.
	for mid := range n {
		var (
			mod      = rtr.Module(mid)
			columns  = mod.Width()
			lines    = mod.Height()
			bitwidth uint
			nonzero  uint
			bytes    uint
		)
		// Sum per-limb bit-widths and byte requirements.
		for _, reg := range mod.Descriptor().Columns {
			if bw := reg.Bitwidth; bw.HasValue() {
				w := bw.Unwrap()
				bitwidth += w
				bytes += byteWidth(w) * lines
			}
		}
		// Count non-zero cells.
		for cid := range columns {
			col := mod.Column(cid)
			//
			for rid := range lines {
				if !col.Get(rid).IsZero() {
					nonzero++
				}
			}
		}
		//
		tbl.SetRow(mid+1,
			termio.NewText(mod.Name()),
			termio.NewText(fmt.Sprintf("%d", columns)),
			termio.NewText(fmt.Sprintf("%d", lines)),
			termio.NewText(fmt.Sprintf("%d", bitwidth)),
			termio.NewText(fmt.Sprintf("%d", columns*lines)),
			termio.NewText(fmt.Sprintf("%d", nonzero)),
			termio.NewText(fmt.Sprintf("%d", bytes)),
		)
	}
	//
	tbl.SetMaxWidths(64)
	// Separate the summary stats (above) from the per-module stats (below) with a
	// horizontal rule as wide as the module table.
	fmt.Println(strings.Repeat("-", int(tbl.PrintedWidth())))
	// Sort modules (descending) by cell count, skipping the title row.
	tbl.Sort(1, termio.NewTableSorter().SortNumericalColumn(4).Invert())
	tbl.Print(AnsiEscapes)
}

// byteWidth returns the number of bytes required to hold a value of the given
// bit-width (i.e. the bit-width rounded up to the nearest byte).
func byteWidth(bitwidth uint) uint {
	w := bitwidth / 8
	//
	if bitwidth%8 != 0 {
		w++
	}
	//
	return w
}
