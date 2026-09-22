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
	"slices"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/goldilocks"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
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
	{field.GF_251, runTraceCmd[gf251.Element, vm.Uint32]},
	{field.GF_8209, runTraceCmd[gf8209.Element, vm.Uint32]},
	{field.KOALABEAR_16, runTraceCmd[koalabear.Element, vm.Uint32]},
	{field.KOALABEAR_24, runTraceCmd[koalabear.Element, vm.Uint32]},
	{field.GOLDILOCKS_32, runTraceCmd[goldilocks.Element, vm.Uint64]},
	{field.BLS12_377, runTraceCmd[bls12_377.Element, vm.Uint128]},
}

// Permitted flag combinations
var traceFlags FlagChecks

func runTraceCmd[F field.Element[F], W vm.Word[W]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		statsCfg traceStatsConfig[F]
		//
		moduleSummarisers = moduleSummarisers[F]()
		//
		build = GetBuildConfig[F](cmd, field)
		// outputFile file for trace
		outputFile = GetString(cmd, "output")
		// check constraints
		check = GetFlag(cmd, "check")
		// show trace statistics
		stats = GetFlag(cmd, "stats")
		// print entire trace
		showTrace = GetFlag(cmd, "print")
		// open trace in the interactive inspector
		inspect = GetFlag(cmd, "inspect")
		// extract sharding config
		sharding = GetString(cmd, "sharding")
		//
		includes = GetStringArray(cmd, "include")
		// Indicate as to whether trace was fully computed or not.
		computed = false
		//
		tr      trace.LazyTrace[F]
		outputs map[string][]byte
	)
	// Sanity permitted flag combinations
	checkFlags(cmd, traceFlags)
	// Configure stats
	statsCfg.human = !GetFlag(cmd, "raw")
	statsCfg.maxCellWidth = GetUint(cmd, "cell-width")
	statsCfg.summarisers = array.Filter(moduleSummarisers, func(m ModuleSummariser[F]) bool {
		return slices.Contains(includes, m.Name)
	})
	statsCfg.sortedBy = util.Some(GetUint(cmd, "sort"))
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
	_, binfile := Build[F, W](build, args[1:]...)
	// =====================================================
	// Trace
	// =====================================================
	// Parse and filter input file
	input := filterInputs(binfile.RawProgram(), ParseInputFile(args[0]))
	// Always trace (no fast mode).  The raw (row-major) trace is retained for
	// statistics, since it carries the original register/limb structure.
	outputs, maybeTr, errors := binfile.Trace(input, traceConfig)
	//
	if maybeTr.IsEmpty() {
		handleErrors(errors)
	}
	//
	tr = maybeTr.Unwrap().
		// Configure parallelism
		WithParallelism(traceConfig.Parallelism())
	// =====================================================
	// Generate output
	// =====================================================
	// Write outputs
	for name, bytes := range outputs {
		fmt.Printf("%s = 0x%s\n", name, hex.EncodeToString(bytes))
	}
	// print trace statistics (if requested).  Only meaningful when a trace was
	// actually generated (i.e. no execution errors).
	if stats {
		printStats(statsCfg, tr)
		// Record that trace computed
		computed = true
	}
	// print entire trace (if requested).  Unlike the inspector, there is no way
	// to reveal a module which was hidden, so everything carrying data is shown
	// (this excludes, for example, the static range-check tables).
	if showTrace {
		corset.PrintTrace(binfile.LimbsMap(), collectShards(tr), false, 32, 128)
		// Record that trace computed
		computed = true
	}
	// write out trace (if requested)
	if outputFile != "" {
		// Write out trace file
		WriteTraceFile(outputFile, collectShards(tr))
		// Record that trace computed
		computed = true
	}
	// =====================================================
	// Check Constraints
	// =====================================================
	if check {
		checkConstraints(binfile, traceConfig, tr)
		// Record that trace computed
		computed = true
	}
	// =====================================================
	// Inspect
	// =====================================================
	// Open the generated trace in the interactive inspector (if requested).  This
	// takes over the terminal, so it runs last, after any stdout output above.
	if inspect && tr.Len() == 1 {
		shards := collectShards(tr)
		// Real ZkC functions are public; synthetic modules (e.g. range-check
		// tables) are private (hidden by default in the inspector).
		errors = corset.InspectTrace(binfile.LimbsMap(), shards[0], publicModule, false, 32, 128)
		// Record that trace computed
		computed = true
	} else if inspect && tr.Len() > 0 {
		errors = append(errors, fmt.Errorf("cannot inspect multiple trace shards"))
		// Prevent computing trace
		computed = true
	} else if inspect {
		errors = append(errors, fmt.Errorf("cannot inspect zero trace shards"))
		// Prevent computing trace
		computed = true
	}
	// =====================================================
	// Dummy Driver
	// =====================================================
	if !computed {
		var stats = util.NewPerfStats()
		// Dummy driver to ensure trace is fully computed, and any errors are
		// reported.
		errors = tr.Apply(func(id uint, _ trace.Shard[F]) {})
		//
		stats.Log("Trace generation")
	}
	// =====================================================
	// Report Execution Failures
	// =====================================================
	handleErrors(errors)
}

func checkConstraints[F field.Element[F], W vm.Word[W]](binfile *constraints.BinaryFile[F, W],
	cfg vm.TraceConfig, trace trace.LazyTrace[F]) {
	//
	var checkConfig corset.CheckConfig
	// Set sensible defaults (for now)
	checkConfig.Report = true
	checkConfig.ReportCellWidth = 32
	checkConfig.ReportTitleWidth = 40
	checkConfig.ReportPadding = 2
	checkConfig.ReportLimbs = true
	checkConfig.ReportComputed = true
	checkConfig.AnsiEscapes = AnsiEscapes
	// Construct limbs map
	mapping := binfile.LimbsMap()
	// Run the check
	failures, errors := binfile.Check(cfg, trace)
	// Report failures first
	if len(failures) > 0 {
		corset.ReportFailures("AIR", mapping, checkConfig, failures)
	}
	// Report any errors arising
	handleErrors(errors)
}

func collectShards[F field.Element[F]](tr trace.LazyTrace[F]) []trace.Shard[F] {
	var (
		stats = util.NewPerfStats()
		//
		shards = make([]trace.Shard[F], tr.Len())
		// Collect all shards into the array
		errors = tr.Apply(func(id uint, shard trace.Shard[F]) {
			shards[id] = shard
		})
	)
	//
	stats.Log("Trace generation")
	// Handle any errors arising
	handleErrors(errors)
	//
	return shards
}

func handleErrors(errors []error) {
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
	traceCmd.Flags().StringP("output", "o", "", "specify output file for writing trace")
	traceCmd.Flags().String("sharding", "", "specify sharding strategy")
	traceCmd.Flags().BoolP("check", "c", false, "check generated trace against constraints")
	traceCmd.Flags().Bool("stats", false, "show overall stats for the generated trace")
	traceCmd.Flags().BoolP("raw", "r", false, "show raw stats (rather than human-readable stats like 1K 234M 2G, etc)")
	traceCmd.Flags().Uint("sort", 1, "sort table column")
	traceCmd.Flags().Uint("cell-width", 32, "specify maximum display width for a cell")
	traceCmd.Flags().StringArrayP("include", "i", []string{"columns", "lines", "cells", "bytes"},
		fmt.Sprintf("specify information to include in module summaries: %s", moduleSummariserOptions[koalabear.Element]()))
	traceCmd.Flags().BoolP("print", "p", false, "print the generated trace")
	traceCmd.Flags().Bool("sequential", false, "force sequential tracing")
	traceCmd.Flags().Bool("inspect", false, "open the generated trace in the interactive inspector")
	traceCmd.PersistentFlags().UintP("batch", "b", 1024, "specify batch size for constraint checking")
	rootCmd.AddCommand(traceCmd)
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

const (
	oneK = 1000
	oneM = oneK * oneK
	oneG = oneM * oneK
)

type traceStatsConfig[F field.Element[F]] struct {
	human        bool
	summarisers  []ModuleSummariser[F]
	sortedBy     util.Option[uint]
	maxCellWidth uint
}

type shardStats struct {
	cells uint64
	bytes uint64
}

// printStats prints a per-module summary for a raw (row-major) trace, much
// like the corset trace command's module listing.  For each module it reports
// the column count, line (row) count, total bit-width, total cells, non-zero
// cells and total bytes.  Native (field-element) limbs, which have no fixed
// bit-width, are excluded from the bit-width and byte totals.
func printStats[F field.Element[F]](cfg traceStatsConfig[F], tr trace.LazyTrace[F]) {
	var (
		stats  = make([]shardStats, tr.Len())
		tables = make([]termio.FormattedTable, tr.Len())
		// Generate all shards
		errors = tr.Apply(func(id uint, shard trace.Shard[F]) {
			// Record summary stats
			stats[id] = getShardStats(shard)
			// Build shard stats
			tables[id] = *buildShardModuleStats(cfg, shard)
		})
	)
	// Print overall summary stats
	printSummaryStats(cfg, stats)
	// Print individual shard stats (in order)
	for _, tbl := range tables {
		tbl.Print(AnsiEscapes)
	}
	//
	handleErrors(errors)
}

// printTraceStats prints overall statistics for a raw (row-major) trace, much
// like the corset "trace --stats" command.  It reports the total number of
// traced cells (both human-readable and raw) plus a breakdown of columns by
// bit-width.  Columns backed by field elements (i.e. native registers, which
// have no fixed bit-width) are reported separately.
func printSummaryStats[F field.Element[F]](cfg traceStatsConfig[F], stats []shardStats) {
	// Render it.
	tbl := termio.NewFormattedTable(3, uint(len(stats)+1))
	//
	tbl.SetRow(0, termio.NewText("shard"), termio.NewText("cells"), termio.NewText("bytes"))
	tbl.SetRule(1)
	//
	for i, shard := range stats {
		var (
			sid = fmt.Sprintf("%d", i)
		)
		//
		cs := humanCount(cfg.human, shard.cells)
		bs := humanCount(cfg.human, shard.bytes)
		//
		tbl.SetRow(uint(i+1), termio.NewText(sid), termio.NewText(cs), termio.NewText(bs))
	}
	//
	tbl.SetMaxWidths(64)
	tbl.Print(AnsiEscapes)
}

func getShardStats[F field.Element[F]](shard trace.Shard[F]) shardStats {
	var (
		cells uint64
		bytes uint64
	)
	// Tally cells and per-column bit-widths across all modules.
	for mid := range shard.Width() {
		mod := shard.Module(mid)
		cells += uint64(mod.Width()) * uint64(mod.Height())
		bytes += moduleBytesSummariser(mod)
	}
	//
	return shardStats{cells, bytes}
}

// humanCount formats a (potentially large) count using K/M/G suffixes, matching
// the corset trace command's cell-count formatting.
func humanCount(enable bool, total uint64) string {
	switch {
	case enable && total > oneG:
		return fmt.Sprintf("%.01fG", float64(total)/oneG)
	case enable && total > oneM:
		return fmt.Sprintf("%.01fM", float64(total)/oneM)
	case enable && total > oneK:
		return fmt.Sprintf("%.01fK", float64(total)/oneK)
	default:
		return fmt.Sprintf("%d", total)
	}
}

func buildShardModuleStats[F field.Element[F]](cfg traceStatsConfig[F], shard trace.Shard[F]) *termio.FormattedTable {
	var (
		n   = shard.Width()
		tbl = termio.NewFormattedTable(uint(len(cfg.summarisers))+1, n+1)
	)
	// Set column titles (leaving the top-left cell blank, as corset does).
	for i, s := range cfg.summarisers {
		tbl.Set(uint(i)+1, 0, termio.NewText(s.Name))
	}
	//
	for mid := range n {
		var (
			mod = shard.Module(mid)
			row = make([]termio.FormattedText, len(cfg.summarisers)+1)
		)
		//
		row[0] = termio.NewText(mod.Name())
		//
		for i, summary := range cfg.summarisers {
			var count = summary.Summary(mod)
			//
			row[i+1] = termio.NewText(humanCount(cfg.human, count))
		}
		//
		tbl.SetRow(mid+1, row...)
	}
	//
	tbl.SetMaxWidths(cfg.maxCellWidth)
	// Sort modules (descending) by cell count, skipping the title row.
	if cfg.sortedBy.HasValue() {
		sorter := termio.NewTableSorter().
			SortNumericalColumn(cfg.sortedBy.Unwrap()).
			Invert()
			//
		tbl.Sort(1, sorter)
	}
	//
	tbl.SetRule(n + 1)
	//
	return tbl
}

// ============================================================================
// Module Summarisers
// ============================================================================

// ModuleSummariser abstracts the notion of a function which summarises the
// contents of a given column.
type ModuleSummariser[F field.Element[F]] struct {
	Name        string
	Description string
	Summary     func(trace.Module[F]) uint64
}

// Used to show the available options on the command-line.
func moduleSummariserOptions[F field.Element[F]]() string {
	summarisers := "\n"
	//
	for _, s := range moduleSummarisers[F]() {
		summarisers = fmt.Sprintf("%s--- %s (%s)\n", summarisers, s.Name, s.Description)
	}
	//
	return summarisers
}

// moduleSummarisers provides a list of suitable summarisers.
func moduleSummarisers[F field.Element[F]]() []ModuleSummariser[F] {
	return []ModuleSummariser[F]{
		{"columns", "column count for module", moduleColumnSummariser[F]},
		{"lines", "line count for module", moduleLineSummariser[F]},
		{"cells", "total number of cells traced for module", moduleCellSummariser[F]},
		{"bytes", "total number of bytes used to hold trace", moduleBytesSummariser[F]},
		{"unique", "total number of unique cells traced for module", moduleUniqueSummariser[F]},
	}
}

func moduleColumnSummariser[F field.Element[F]](mod trace.Module[F]) uint64 {
	return uint64(mod.Width())
}

func moduleCellSummariser[F field.Element[F]](mod trace.Module[F]) uint64 {
	return uint64(mod.Height()) * uint64(mod.Width())
}

func moduleLineSummariser[F field.Element[F]](mod trace.Module[F]) uint64 {
	return uint64(mod.Height())
}

func moduleBytesSummariser[F field.Element[F]](mod trace.Module[F]) uint64 {
	var count uint64
	//
	for i := range mod.Descriptor().Columns {
		var data = mod.Column(uint(i))
		//
		if data != nil {
			count += uint64(data.Bytes())
		}
	}
	//
	return count
}

func moduleUniqueSummariser[F field.Element[F]](mod trace.Module[F]) uint64 {
	var count uint64
	//
	for i := range mod.Descriptor().Columns {
		var data = mod.Column(uint(i))
		//
		if data != nil {
			count += uniqueElementsSummariser(data)
		}
	}
	//
	return count
}

func uniqueElementsSummariser[F field.Element[F]](data array.Array[F]) uint64 {
	//
	elems := hash.NewSet[word.BigEndian](data.Len() / 2)
	// Add all the elements
	for i := uint(0); i < data.Len(); i++ {
		var (
			ith  = data.Get(i)
			word word.BigEndian
		)
		//
		elems.Insert(word.SetBytes(ith.Bytes()))
	}
	// Done
	return uint64(elems.Size())
}
