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
package corset

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/view"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio/widget"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// computeCmd represents the compute command
var checkCmd = &cobra.Command{
	Use:   "check [flags] trace_file constraint_file(s)",
	Short: "Check a given trace against a set of constraints.",
	Long: `Check a given trace against a set of constraints.
	Traces can be given either as JSON or binary lt files.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, checkCmds)
	},
}

// Available instances
var checkCmds = []FieldAgnosticCmd{
	{field.GF_251, runCheckCmd[gf251.Element]},
	{field.GF_8209, runCheckCmd[gf8209.Element]},
	{field.KOALABEAR_16, runCheckCmd[koalabear.Element]},
	{field.BLS12_377, runCheckCmd[bls12_377.Element]},
}

func runCheckCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	var cfg CheckConfig

	if len(args) < 2 {
		fmt.Println(cmd.UsageString())
		os.Exit(1)
	}
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}
	// Configure CPU profiling (if requested)
	startCpuProfiling(cmd)
	//
	batched := GetFlag(cmd, "batched")
	//
	strategy, ok := ir.GetPaddingStrategy(GetString(cmd, "padding"))
	if !ok {
		fmt.Printf("unknown padding strategy \"%s\"\n", GetString(cmd, "padding"))
		os.Exit(3)
	}
	//
	cfg.PaddingStrategy = strategy

	cfg.Report = GetFlag(cmd, "report")
	cfg.ReportPadding = GetUint(cmd, "report-context")
	cfg.ReportLimbs = GetFlag(cmd, "show-limbs")
	cfg.ReportComputed = GetFlag(cmd, "show-computed")
	cfg.ReportCellWidth = GetUint(cmd, "report-cellwidth")
	cfg.ReportTitleWidth = GetUint(cmd, "report-titlewidth")
	cfg.AnsiEscapes = GetFlag(cmd, "ansi-escapes")
	// Read in constraint files
	schemas := *getSchemaStack[F](cmd, SCHEMA_DEFAULT_AIR, args[1:]...)
	// enable / disable coverage
	if covfile := GetString(cmd, "coverage"); covfile != "" {
		cfg.Coverage = util.Some(covfile)
	}
	//
	tracefile := args[0]
	//
	checkWithLegacyPipeline(cfg, batched, tracefile, schemas)
	// Write memory profiling (if requested)
	writeMemProfile(cmd)
	// Stop cpu profiling (if was requested)
	stopCpuProfiling(cmd)
}

func startCpuProfiling(cmd *cobra.Command) {
	if filename := GetString(cmd, "cpuprof"); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		//
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
	}
}

func stopCpuProfiling(cmd *cobra.Command) {
	if filename := GetString(cmd, "cpuprof"); filename != "" {
		pprof.StopCPUProfile()
	}
}

func writeMemProfile(cmd *cobra.Command) {
	if filename := GetString(cmd, "memprof"); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		//nolint
		defer f.Close()
		//
		runtime.GC()
		//
		if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}

// CheckConfig encapsulates certain parameters to be used when checking traces.
type CheckConfig struct {
	// Corset source mapping (maybe nil if non available).
	CorsetSourceMap *corset.SourceMap
	// Specifies whether to use Coverage testing and, if so, where to write the
	// Coverage data.
	Coverage util.Option[string]
	// Specifies how much front padding to add to each module.
	PaddingStrategy ir.PaddingStrategy
	// Specifies whether or not to Report details of the failure (e.g. for
	// debugging purposes).
	Report bool
	// Specifies the number of additional rows to show eitherside of the failing
	// area. This essentially allows more contextual information to be shown.
	ReportPadding uint
	// Specifies the width of a cell to show.
	ReportCellWidth uint
	// Specifies the width of a column title to show.
	ReportTitleWidth uint
	// Specifies whether or not to show raw limbs
	ReportLimbs bool
	// Specifies whether or not to show raw computed registers
	ReportComputed bool
	// Enable ansi escape codes in reports
	AnsiEscapes bool
}

// Check raw constraints using the legacy pipeline.
func checkWithLegacyPipeline[F field.Element[F]](cfg CheckConfig, batched bool, tracefile string,
	schemas cmd_util.SchemaStacker[F]) {
	//
	var traces []tr.Trace[F]
	//
	stats := util.NewPerfStats()
	// Extract debug information (if available)
	cfg.CorsetSourceMap, _ = schemas.SourceMap()
	//
	stats.Log("Reading constraints file")
	// Parse trace file(s)
	if batched {
		// batched mode
		traces = ReadBatchedTraceFile[F](tracefile)
	} else {
		// unbatched (i.e. normal) mode
		traces = []tr.Trace[F]{ReadTraceFile[F](tracefile)}
	}
	// Go!
	if ok := checkTraces(traces, schemas, cfg); !ok {
		os.Exit(1)
	}
}

func checkTraces[F field.Element[F]](traces []tr.Trace[F], stacker cmd_util.SchemaStacker[F],
	cfg CheckConfig) bool {
	//
	for _, trace := range traces {
		// Configure stack.  This is important to ensure true separation
		// between runs (e.g. for the io.Executor).
		stack := stacker.Build()
		// configure trace builder
		builder := stack.TraceBuilder().WithPadding(cfg.PaddingStrategy)
		// identify concrete schema separately
		schema := stack.ConcreteSchema()
		// identify schema name
		ir := stack.ConcreteIrName()
		//
		if ok := CheckTrace(ir, schema, builder, cfg, trace); !ok {
			return false
		}
	}
	// Done
	return true
}

// CheckTrace checks a given set of constraints against a given trace file using
// a configured trace builder and check configuration.
func CheckTrace[F field.Element[F]](ir string, schema sc.AnySchema[F], builder ir.TraceBuilder[F],
	cfg CheckConfig, trace tr.Trace[F]) bool {
	// begin performance measurement
	var (
		mapping          = module.IdentityMap[F](schema.Modules().Collect()...)
		stats            = util.NewPerfStats()
		recoverable bool = true
		errs        []error
	)
	//
	for i, shard := range trace {
		var es []error

		trace[i], es = builder.Build(schema, shard)
		errs = append(errs, es...)
		recoverable = recoverable && (trace[i] != nil)
	}
	// Log cost of expansion
	stats.Log("Expanding trace columns")
	// Report any errors
	reportErrors(ir, errs)
	// Check whether considered unrecoverable
	if !recoverable || len(errs) > 0 {
		return false
	}
	//
	stats = util.NewPerfStats()
	// Check constraints
	if errs := sc.Accepts(builder.Parallelism(), schema, trace); len(errs) > 0 {
		ReportFailures(ir, mapping, cfg, trace, errs)
		return false
	}
	//
	stats.Log("Checking constraints")
	//
	return true
}

// ReportFailures reports constraint failures, whilst providing contextual
// information (when requested).
func ReportFailures[F field.Element[F]](ir string,
	mapping module.LimbsMap, cfg CheckConfig, trace tr.Trace[F], failures []sc.Failure[F]) {
	//
	var errs = make([]error, len(failures))
	//
	for j, f := range failures {
		errs[j] = errors.New(f.Message())
	}
	// First, log errors
	reportErrors(ir, errs)
	// Second, produce report (if requested)
	if cfg.Report {
		for _, f := range failures {
			reportFailure(f, trace, mapping, cfg)
		}
	}
}

// Print a human-readable report detailing the given failure
func reportFailure[F field.Element[F]](failure sc.Failure[F], trace tr.Trace[F], mapping module.LimbsMap,
	cfg CheckConfig) {
	// Identify all relevant cells
	var cells = failure.RequiredCells(trace)
	fmt.Printf("failing constraint %s:\n", failure.Handle())
	//
	for i, shard := range trace {
		// Filter out cells relevant to the given shard
		var (
			lsharded = array.Filter(cells, func(r tr.ShardedCellRef) bool {
				return r.Shard == uint(i)
			})
			// Map to cell refs
			lcells = array.Map(lsharded, func(_ uint, r tr.ShardedCellRef) tr.CellRef {
				return r.Ref
			})
		)
		// Check whether anything to report for this
		if len(lcells) > 0 {
			// Print out cells for the given shard.
			reportRelevantCells(lcells, shard, mapping, cfg)
		}
	}
}

// Print a human-readable report detailing the given failure with a vanishing constraint.
func reportRelevantCells[F field.Element[F]](cells []tr.CellRef, trace tr.Shard[F],
	mapping module.LimbsMap, cfg CheckConfig) {
	// Construct trace window
	builder := view.NewBuilder[F](mapping).
		WithLimbs(cfg.ReportLimbs).
		WithComputed(cfg.ReportComputed).
		WithCellWidth(cfg.ReportCellWidth).
		WithTitleWidth(cfg.ReportTitleWidth).
		WithFormatting(view.NewCellFormatter(cells, cfg.AnsiEscapes))
		//
	if cfg.CorsetSourceMap != nil {
		builder = builder.WithSourceMap(*cfg.CorsetSourceMap)
	}
	// Build window
	window := builder.Build(trace)
	// Focus window on those cells relevant to the failure
	window = window.Filter(view.FilterForCells(cells, cfg.ReportPadding))
	// Print all windows
	for i := range window.Width() {
		var (
			ith = window.Module(i)
			// Construct & configure printer
			tp = widget.NewTable(window.Module(i))
			//
			name = ith.Data().Name()
		)
		// Print out module name
		if window.Width() > 1 && name != "" {
			fmt.Printf("%s:\n", name)
		}
		// Print out report
		tp.Print()
		fmt.Println()
	}
}

func reportErrors(ir string, errs []error) {
	// Construct set to ensure deduplicate errors
	set := make(map[string]bool, len(errs))
	//
	for _, err := range errs {
		key := fmt.Sprintf("%s (%s)", err, ir)
		set[key] = true
	}
	// Report each one
	for e := range set {
		log.Errorln(e)
	}
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().Bool("report", false, "report details of failure for debugging")
	checkCmd.Flags().Uint("report-context", 2, "specify number of rows to show eitherside of failure in report")
	checkCmd.Flags().Bool("show-computed", false, "show (low-level) computed registers")
	checkCmd.Flags().Bool("show-limbs", false, "specify whether to show register limbs in report")
	checkCmd.Flags().Uint("report-cellwidth", 32, "specify max number of bytes to show in a given cell in report")
	checkCmd.Flags().Uint("report-titlewidth", 40, "specify maximum width of column titles in report")
	//
	checkCmd.Flags().String("coverage", "", "write JSON coverage data to file")
	checkCmd.Flags().String("padding", "next-power-of-two",
		"front padding strategy for each module (none, single-row, next-power-of-two)")
	checkCmd.Flags().Bool("batched", false,
		"specify trace file is batched (i.e. contains multiple traces, one for each line)")
	checkCmd.Flags().Bool("ansi-escapes", true, "specify whether to allow ANSI escapes or not (e.g. for colour reports)")
	// profiling commands'
	checkCmd.Flags().String("cpuprof", "", "write cpu profile to `file`")
	checkCmd.Flags().String("memprof", "", "write memory profile to `file`")
}
