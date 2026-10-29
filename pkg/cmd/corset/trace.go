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
package corset

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"

	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/view"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/goldilocks"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/mamabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/mamabear2"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio/widget"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// traceCmd represents the trace command for manipulating traces.
var traceCmd = &cobra.Command{
	Use:   "trace [flags] trace_file [constraint_file(s)]",
	Short: "Operate on a trace file.",
	Long: `Operate on a trace file, such as converting
	it from one format (e.g. lt) to another (e.g. json),
	or filtering out modules, or listing columns, etc.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, traceCmds)
	},
}

// Available instances
var traceCmds = []FieldAgnosticCmd{
	{field.GF_251, runTraceCmd[gf251.Element]},
	{field.GF_8209, runTraceCmd[gf8209.Element]},
	{field.KOALABEAR_16, runTraceCmd[koalabear.Element]},
	{field.GOLDILOCKS_32, runTraceCmd[goldilocks.Element]},
	{field.MAMABEAR_32, runTraceCmd[mamabear.Element]},
	{field.MAMABEAR2_32, runTraceCmd[mamabear2.Element]},
	{field.BLS12_377, runTraceCmd[bls12_377.Element]},
}

func runTraceCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	var (
		traces []tr.Trace[F]
		cfg    TraceConfig[F]
		err    error
	)
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}
	// Parse trace
	batched := GetFlag(cmd, "batched")
	output := GetString(cmd, "out")
	//
	cfg.trace = GetFlag(cmd, "print")
	cfg.maxCellWidth = GetUint(cmd, "cell-width")
	cfg.maxTitleWidth = GetUint(cmd, "title-width")
	cfg.showComputed = GetFlag(cmd, "show-computed")
	cfg.startRow = GetUint(cmd, "start")
	cfg.endRow = GetUint(cmd, "end")
	cfg.filter, err = regexp.Compile(GetString(cmd, "filter"))

	padding, ok := ir.GetPaddingStrategy(GetString(cmd, "padding"))
	if !ok {
		fmt.Printf("unknown padding strategy \"%s\"\n", GetString(cmd, "padding"))
		os.Exit(3)
	}
	// Check for error
	if err != nil {
		panic(err)
	}
	//
	cfg.sortColumn = GetUint(cmd, "sort")
	// Read in constraint files
	stacker := *getSchemaStack[F](cmd, SCHEMA_OPTIONAL, args[1:]...)
	stack := stacker.Build()
	builder := stack.TraceBuilder().WithPadding(padding)
	//
	if stacker.HasSchema() {
		// Extract debug information (if available)
		cfg.sourceMap, _ = stacker.SourceMap()
	}
	// Parse trace file(s)
	if batched {
		// batched mode
		traces = ReadBatchedTraceFile[F](args[0])
	} else {
		// unbatched (i.e. normal) mode
		traces = []tr.Trace[F]{ReadTraceFile[F](args[0])}
	}
	//
	if builder.Expanding() && !stack.HasConcreteSchema() {
		fmt.Println("must specify one of --mir/air")
		os.Exit(2)
	} else if builder.Expanding() {
		var tp_errors []error
		// Expand all the traces
		traces, tp_errors = expandTraces(traces, stack, builder)
		// Print trace info
		for _, tf := range traces {
			printTraceInfo(cfg, tf)
		}
		// Report any propagation errors
		for _, err := range tp_errors {
			log.Errorln(err)
		}
	} else {
		// Use raw trace
		for _, tf := range traces {
			// Print what we have
			printTraceInfo(cfg, tf)
		}
	}
	// Write out results (if requested)
	if output != "" {
		writeBatchedTracesFile(output, traces...)
	}
}

func init() {
	rootCmd.AddCommand(traceCmd)
	traceCmd.Flags().BoolP("print", "p", false, "print entire trace file")
	traceCmd.Flags().Uint("sort", 0, "sort table column")
	traceCmd.Flags().Uint("start", 0, "filter out rows below this")
	traceCmd.Flags().Uint("end", math.MaxUint, "filter out this and all following rows")
	traceCmd.Flags().Uint("cell-width", 32, "specify maximum display width for a cell")
	traceCmd.Flags().Uint("title-width", 32, "specify maximum display width for a column title")
	traceCmd.Flags().Bool("show-computed", false, "show (low-level) computed registers")
	traceCmd.Flags().String("padding", "next-power-of-two",
		"front padding strategy for each module (none, single-row, next-power-of-two)")
	traceCmd.Flags().StringP("out", "o", "", "Specify output file to write trace")
	traceCmd.Flags().StringP("filter", "f", "", "Filter columns matching regex")
	traceCmd.Flags().Bool("batched", false,
		"specify trace file is batched (i.e. contains multiple traces, one for each line)")
}

// TraceConfig packages together useful things for the various supported
// options.
type TraceConfig[F field.Element[F]] struct {
	sourceMap    *corset.SourceMap
	maxCellWidth uint
	//nolint
	maxTitleWidth uint
	// Indicate whether or not to show low-level registers in trace.
	showComputed bool
	// Column to sort on
	sortColumn uint
	// Start/end row for trace view
	startRow uint
	endRow   uint
	// Column filter for trace view
	filter *regexp.Regexp
	// show full trace
	trace bool
}

func constructTraceFilter[F field.Element[F]](cfg TraceConfig[F], trace tr.Shard[F]) view.TraceFilter {
	return view.NewTraceFilter(func(mid module.Id) view.ModuleFilter {
		return view.NewModuleFilter(cfg.startRow, cfg.endRow, func(col view.SourceColumn) bool {
			// Construct fully qualified name
			var qualifiedName = tr.QualifiedColumnName(trace.Module(mid).Name(), col.Name)
			//
			return cfg.filter.MatchString(qualifiedName)
		})
	})
}

func expandTraces[F field.Element[F]](traceFiles []tr.Trace[F], stack cmd_util.SchemaStack[F],
	bldr ir.TraceBuilder[F]) ([]tr.Trace[F], []error) {
	//
	var (
		traces = make([]tr.Trace[F], len(traceFiles))
		errors []error
	)
	//
	for i := range traceFiles {
		var errs []error
		//
		traces[i], errs = bldr.Build(stack.ConcreteSchema(), traceFiles[i])
		//
		errors = append(errors, errs...)
	}
	//
	return traces, errors
}

func printTraceInfo[F field.Element[F]](cfg TraceConfig[F], trace tr.Trace[F]) {
	for i, shard := range trace {
		//
		if len(trace) > 0 {
			fmt.Println()
			fmt.Printf("Shard #%d\n", i)
			fmt.Println(strings.Repeat("-", int(80)))
		}
		// Construct trace window
		view := view.NewBuilder().
			WithCellWidth(cfg.maxCellWidth).
			WithTitleWidth(cfg.maxTitleWidth).
			WithComputed(cfg.showComputed)
		// Add source map (if applicable)
		if cfg.sourceMap != nil {
			view = view.WithSourceMap(*cfg.sourceMap)
		}
		// Construct viewing window
		window := view.Build(shard)
		// Construct & apply trace filter
		window = window.Filter(constructTraceFilter(cfg, shard))
		// Print full trace (if requested)
		if cfg.trace {
			printTrace(window)
		}
	}
}

// PrintTrace prints out a given trace in tabular form, with one table per
// module.  This is the non-interactive counterpart of InspectTrace, and
// produces the same output as "go-corset trace --print".  Every module holding
// data is printed, since (unlike the inspector) there is no way to reveal one
// which was hidden.
func PrintTrace[F field.Element[F]](trace tr.Trace[F], cellWidth, titleWidth uint) {
	for _, shard := range trace {
		// Build the viewing window (no source map, so show computed registers).
		builder := view.NewBuilder().
			WithCellWidth(cellWidth).
			WithTitleWidth(titleWidth).
			WithComputed(true)
		//
		printTrace(builder.Build(shard))
	}
}

func printTrace(window view.TraceView) {
	// Print all windows
	for i := range window.Width() {
		var (
			ith           = window.Module(i)
			width, height = ith.Dimensions()
			// Construct & configure printer
			tp = widget.NewTable(ith)
			//
			name = ith.Data().Name()
		)
		// NOTE: dimensions include the row / column titles, hence the
		// comparisons against one rather than zero.
		if height <= 1 || width <= 1 {
			// Don't bother printing modules with no columns, or no rows.  The
			// latter includes static reference tables (e.g. a "$range_uN"
			// lookup table), whose contents are fixed by the schema and hence
			// carry no trace data at all.
			continue
		} else if window.Width() > 1 && name != "" {
			// Print out module name
			fmt.Printf("%s:\n", name)
		}
		// Print out report
		tp.Print()
		fmt.Println()
	}
}
