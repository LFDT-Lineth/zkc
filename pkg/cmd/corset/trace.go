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
	"fmt"
	"math"
	"math/big"
	"os"
	"regexp"
	"strings"

	"github.com/consensys/go-corset/pkg/asm"
	"github.com/consensys/go-corset/pkg/binfile"
	cmd_util "github.com/consensys/go-corset/pkg/cmd/corset/util"
	"github.com/consensys/go-corset/pkg/cmd/corset/view"
	"github.com/consensys/go-corset/pkg/corset"
	"github.com/consensys/go-corset/pkg/ir"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	tr "github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/trace/lt"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/hash"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/field/bls12_377"
	"github.com/consensys/go-corset/pkg/util/field/gf251"
	"github.com/consensys/go-corset/pkg/util/field/gf8209"
	"github.com/consensys/go-corset/pkg/util/field/koalabear"
	"github.com/consensys/go-corset/pkg/util/termio"
	"github.com/consensys/go-corset/pkg/util/termio/widget"
	"github.com/consensys/go-corset/pkg/util/word"
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
	{field.BLS12_377, runTraceCmd[bls12_377.Element]},
}

func runTraceCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	var (
		ltTraces []lt.TraceFile
		cfg      TraceConfig
		err      error
	)
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}
	// Parse trace
	batched := GetFlag(cmd, "batched")
	metadata := GetFlag(cmd, "metadata")
	output := GetString(cmd, "out")
	//
	cfg.columns = GetFlag(cmd, "columns")
	cfg.modules = GetFlag(cmd, "modules")
	cfg.trace = GetFlag(cmd, "print")
	cfg.stats = GetFlag(cmd, "stats")
	cfg.includes = GetStringArray(cmd, "include")
	cfg.maxCellWidth = GetUint(cmd, "cell-width")
	cfg.maxTitleWidth = GetUint(cmd, "title-width")
	cfg.showLimbs = GetFlag(cmd, "show-limbs")
	cfg.showComputed = GetFlag(cmd, "show-computed")
	cfg.startRow = GetUint(cmd, "start")
	cfg.endRow = GetUint(cmd, "end")
	cfg.filter, err = regexp.Compile(GetString(cmd, "filter"))
	padding := GetUint(cmd, "padding")
	// Check for error
	if err != nil {
		panic(err)
	}
	//
	cfg.sortColumn = GetUint(cmd, "sort")
	//ltv2 := GetFlag(cmd, "ltv2")
	// Read in constraint files
	stacker := *getSchemaStack[F](cmd, SCHEMA_OPTIONAL, args[1:]...)
	stack := stacker.Build()
	builder := stack.TraceBuilder().WithPadding(padding)
	//
	if stacker.HasBinaryFile() {
		// Extract debug information (if available)
		cfg.sourceMap, _ = binfile.GetAttribute[*corset.SourceMap](stacker.BinaryFile())
	}
	// Extract register mapping (for limbs)
	cfg.mapping = stack.RegisterMapping()
	// Parse trace file(s)
	if batched {
		// batched mode
		ltTraces = ReadBatchedTraceFile(args[0])
	} else {
		// unbatched (i.e. normal) mode
		ltTraces = []lt.TraceFile{ReadTraceFile(args[0])}
		// Print meta-data (if requested)
		if metadata {
			printTraceFileHeader(ltTraces[0].Header())
		}
	}
	//
	if builder.Expanding() && !stack.HasConcreteSchema() {
		fmt.Println("must specify one of --mir/air")
		os.Exit(2)
	} else if builder.Expanding() {
		var (
			tp_errors []error
			traces    []trace.Trace[F]
		)
		// Expand all the traces
		traces, tp_errors = expandLtTraces(ltTraces, stack, builder)
		// Print trace info
		for _, tf := range traces {
			printTraceInfo(cfg, tf)
		}
		// Report any propagation errors
		for _, err := range tp_errors {
			log.Errorln(err)
		}
		// Reconstruct traces (if needed)
		if output != "" {
			ltTraces = reconstructLtTraces(ltTraces, traces, false)
		}
	} else {
		// Use raw trace
		for _, ltf := range ltTraces {
			// Configure dummy mapping
			cfg.mapping = cmd_util.BigWordMapping(ltf)
			// Print what we have
			printTraceInfo(cfg, &ltf)
		}
	}
	// Write out results (if requested)
	if output != "" {
		writeBatchedTracesFile(output, ltTraces...)
	}
}

func init() {
	rootCmd.AddCommand(traceCmd)
	traceCmd.Flags().BoolP("columns", "c", false, "show column stats for the trace file")
	traceCmd.Flags().BoolP("modules", "m", false, "show module stats for the trace file")
	traceCmd.Flags().StringArrayP("include", "i", []string{"lines", "bitwidth", "bytes", "elements"},
		fmt.Sprintf("specify information to include in column listing: %s", summariserOptions()))
	traceCmd.Flags().Bool("stats", false, "show overall stats for the trace file")
	traceCmd.Flags().BoolP("print", "p", false, "print entire trace file")
	traceCmd.Flags().Uint("sort", 0, "sort table column")
	traceCmd.Flags().Uint("start", 0, "filter out rows below this")
	traceCmd.Flags().Uint("end", math.MaxUint, "filter out this and all following rows")
	traceCmd.Flags().Uint("cell-width", 32, "specify maximum display width for a cell")
	traceCmd.Flags().Uint("title-width", 32, "specify maximum display width for a column title")
	traceCmd.Flags().Bool("show-computed", false, "show (low-level) computed registers")
	traceCmd.Flags().BoolP("show-limbs", "l", false, "show register limbs")
	traceCmd.Flags().Uint("padding", 0, "specify amount of (front) padding to apply")
	traceCmd.Flags().StringP("out", "o", "", "Specify output file to write trace")
	traceCmd.Flags().StringP("filter", "f", "", "Filter columns matching regex")
	traceCmd.Flags().Bool("batched", false,
		"specify trace file is batched (i.e. contains multiple traces, one for each line)")
	traceCmd.Flags().Bool("metadata", false, "Print embedded metadata")
	traceCmd.Flags().Bool("ltv2", false, "Use ltv2 file format")
}

// TraceConfig packages together useful things for the various supported
// options.
type TraceConfig struct {
	mapping      module.LimbsMap
	sourceMap    *corset.SourceMap
	maxCellWidth uint
	//nolint
	maxTitleWidth uint
	// Column / Module summarisers to include
	includes []string
	// Indicate whether or not to show limbs in trace.
	showLimbs bool
	// Indicate whether or not to show low-level registers in trace.
	showComputed bool
	// Column to sort on
	sortColumn uint
	// Start/end row for trace view
	startRow uint
	endRow   uint
	// Column filter for trace view
	filter *regexp.Regexp
	// show columns
	columns bool
	// show module summaries
	modules bool
	// show full trace
	trace bool
	// show summary stats
	stats bool
}

func constructTraceFilter[F field.Element[F]](cfg TraceConfig, trace tr.Trace[F]) view.TraceFilter {
	return view.NewTraceFilter(func(mid module.Id) view.ModuleFilter {
		return view.NewModuleFilter(cfg.startRow, cfg.endRow, func(col view.SourceColumn) bool {
			// Construct fully qualified name
			var qualifiedName = tr.QualifiedColumnName(trace.Module(mid).Name(), col.Name)
			//
			return cfg.filter.MatchString(qualifiedName)
		})
	})
}

// RawColumn provides a convenient alias
type RawColumn = lt.Column[word.BigEndian]

func expandLtTraces[F field.Element[F]](traceFiles []lt.TraceFile, stack cmd_util.SchemaStack[F],
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
		traces[i], errs = expandLtTrace(traceFiles[i], stack, bldr)
		//
		errors = append(errors, errs...)
	}
	//
	return traces, errors
}

func reconstructLtTraces[F field.Element[F]](ltTraces []lt.TraceFile, traces []trace.Trace[F], legacy bool,
) []lt.TraceFile {
	//
	var ntraces = make([]lt.TraceFile, len(ltTraces))
	// Convert all traces back to lt files.
	for i := range traces {
		var (
			header = ltTraces[i].Header()
			ith    = lt.FromRawTrace(header.MetaData, traces[i])
		)
		// Construct legacy trace file (if requested)
		if legacy {
			ith = lt.NewTraceFileV1(ith.Header().MetaData, ith.Heap(), ith.RawModules())
		}
		// Done
		ntraces[i] = ith
	}
	//
	return ntraces
}

func expandLtTrace[F field.Element[F]](tf lt.TraceFile, stack cmd_util.SchemaStack[F], bldr ir.TraceBuilder[F],
) (tr.Trace[F], []error) {
	//
	var (
		schema    = stack.BinaryFile().Schema
		tb_errors []error
		tp_errors []error
		tr        trace.Trace[F]
	)
	// Apply trace propagation
	if bldr.Expanding() {
		perf := util.NewPerfStats()
		//
		tf, tp_errors = asm.Propagate(schema, tf)
		//
		perf.Log("Trace propagation")
	}
	// Construct expanded trace
	tr, tb_errors = bldr.Build(stack.ConcreteSchema(), tf)
	// Handle errors
	if len(tb_errors) > 0 {
		for _, err := range tb_errors {
			log.Errorln(err)
		}
		//
		os.Exit(1)
	}
	// Now, reconstruct it!
	return tr, tp_errors
}

func printTraceFileHeader(header lt.Header) {
	fmt.Printf("Format: %d.%d\n", header.MajorVersion, header.MinorVersion)
	// Attempt to parse metadata
	metadata, err := header.GetMetaData()
	//
	if err != nil {
		log.Errorln(err)
		os.Exit(1)
	} else if !metadata.IsEmpty() {
		fmt.Println("Metadata:")
		//
		printTypedMetadata(1, metadata)
	}
}

func printTraceInfo[F field.Element[F]](cfg TraceConfig, trace tr.Trace[F]) {
	// Construct trace window
	view := view.NewBuilder[F](cfg.mapping).
		WithCellWidth(cfg.maxCellWidth).
		WithTitleWidth(cfg.maxTitleWidth).
		WithLimbs(cfg.showLimbs).
		WithComputed(cfg.showComputed)
	// Add source map (if applicable)
	if cfg.sourceMap != nil {
		view = view.WithSourceMap(*cfg.sourceMap)
	}
	// Construct viewing window
	window := view.Build(trace)
	// Construct & apply trace filter
	window = window.Filter(constructTraceFilter(cfg, trace))
	// Print column summaries (if requested)
	if cfg.columns {
		listColumns(cfg, window)
	}
	// Print module summaries (if requested)
	if cfg.modules {
		listModules(cfg, window)
	}
	// Print trace summary (if requested)
	if cfg.stats {
		summaryStats(window)
	}
	// Print full trace (if requested)
	if cfg.trace {
		printTrace(cfg, window)
	}
}

func printTrace(cfg TraceConfig, window view.TraceView) {
	// Print all windows
	for i := range window.Width() {
		var (
			ith       = window.Module(i)
			_, height = ith.Dimensions()
			// Construct & configure printer
			tp = widget.NewTable(ith)
			//
			name = ith.Data().Name().String()
		)
		// Print out module name
		if height <= 1 {
			// Don't bother print empty modules
			continue
		} else if window.Width() > 1 && name != "" {
			fmt.Printf("%s:\n", name)
		}
		// Print out report
		tp.Print()
		fmt.Println()
	}
}

func listModules(cfg TraceConfig, window view.TraceView) {
	var (
		//
		summarisers = moduleSumarisers
		m           = 1 + uint(len(summarisers))
		n           = window.Width()
		// Go!
		tbl = termio.NewFormattedTable(m, n+1)
	)
	// Set column titles
	for i := uint(0); i < uint(len(summarisers)); i++ {
		tbl.Set(i+1, 0, termio.NewText(summarisers[i].name))
	}
	// Compute column data
	for i := range n {
		var (
			mod = window.Module(i)
			row = summariseModule(mod, moduleSumarisers)
		)
		// Set row
		tbl.SetRow(i+1, row...)
	}
	//
	tbl.SetMaxWidths(cfg.maxCellWidth)
	tbl.Sort(1, termio.NewTableSorter().
		SortNumericalColumn(cfg.sortColumn).
		Invert())
	tbl.Print(true)
}

func listColumns(cfg TraceConfig, window view.TraceView) {
	var (
		summarisers = selectColumnSummarisers(cfg.includes)
		m           = 1 + uint(len(summarisers))
		n           = totalActiveRegisters(window)
		// Go!
		tbl   = termio.NewFormattedTable(m, n+1)
		c     = make(chan util.Pair[uint, []termio.FormattedText], n)
		index uint
	)
	// Set titles
	tbl.Set(0, 0, termio.NewText("Column"))

	for i := uint(0); i < uint(len(summarisers)); i++ {
		tbl.Set(i+1, 0, termio.NewText(summarisers[i].name))
	}
	// Compute data
	for i := range window.Width() {
		var (
			mod     = window.Module(i)
			data    = mod.Data()
			mapping = data.Mapping()
		)
		//
		for _, reg := range activeRegisters(mod) {
			// Launch summarisers
			go func(index uint, reg register.Id) {
				name := mapping.Register(reg).Name()
				// Apply summarisers to column
				row := summariseColumn(data.Name(), name, data.DataOf(mapping.LimbIds(reg)), summarisers)
				// Package result
				c <- util.NewPair(index, row)
			}(index, reg)
			//
			index = index + 1
		}
	}
	// Collect results
	for range n {
		// Read packaged result from channel
		res := <-c
		// Set row
		tbl.SetRow(res.Left+1, res.Right...)
	}
	//
	tbl.SetMaxWidths(cfg.maxCellWidth)
	tbl.Sort(1, termio.NewTableSorter().
		SortNumericalColumn(cfg.sortColumn).
		Invert())
	tbl.Print(true)
}

func selectColumnSummarisers(includes []string) []ColumnSummariser {
	includes = flattenIncludes(includes)
	summarisers := make([]ColumnSummariser, len(includes))
	// Iterate included summarisers
	for i, ss := range includes {
		// Look them up
		for _, cs := range columnSummarisers {
			if cs.name == ss {
				summarisers[i] = cs
				break
			}
		}
		// Sanity check we found something
		if summarisers[i].name != ss {
			panic(fmt.Sprintf("unknown column summariser: %s", ss))
		}
	}
	// Done
	return summarisers
}

func flattenIncludes(includes []string) []string {
	count := 0
	// Determine total number of columns
	for _, s := range includes {
		extras := strings.Count(s, ",")
		if extras > 0 {
			count += extras
		}

		count++
	}
	// Expand (if necessary)
	if count != len(includes) {
		nincludes := make([]string, count)
		index := 0
		// Process each include
		for _, s := range includes {
			if strings.Contains(s, ",") {
				for _, t := range strings.Split(s, ",") {
					nincludes[index] = t
					index++
				}
			} else {
				nincludes[index] = s
				index++
			}
		}
		// Done
		includes = nincludes
	}
	// Done
	return includes
}

func summariseModule(mod view.ModuleView, summarisers []ModuleSummariser) []termio.FormattedText {
	var (
		m   = 1 + uint(len(summarisers))
		row = make([]termio.FormattedText, m)
	)
	//
	row[0] = termio.NewText(mod.Data().Name().String())
	//
	for j, s := range summarisers {
		row[j+1] = termio.NewText(s.summary(mod))
	}
	//
	return row
}

func summariseColumn(module module.Name, name string, column view.RegisterView,
	summarisers []ColumnSummariser) []termio.FormattedText {
	//
	var (
		m   = 1 + uint(len(summarisers))
		row = make([]termio.FormattedText, m)
	)
	//
	row[0] = termio.NewText(fmt.Sprintf("%s.%s", module, name))
	// Generate each summary
	for j := 0; j < len(summarisers); j++ {
		row[j+1] = termio.NewText(summarisers[j].summary(column))
	}
	// Done
	return row
}

func summaryStats(window view.TraceView) {
	m := uint(len(trSummarisers))
	tbl := termio.NewFormattedTable(2, m)
	// Go!
	for i := uint(0); i < m; i++ {
		ith := trSummarisers[i]
		summary := ith.summary(window)
		tbl.SetRow(i, termio.NewText(ith.name), termio.NewText(summary))
	}
	//
	tbl.SetMaxWidths(64)
	tbl.Print(true)
}

func totalActiveRegisters(trace view.TraceView) uint {
	var count = 0
	//
	for i := range trace.Width() {
		count += len(activeRegisters(trace.Module(i)))
	}
	//
	return uint(count)
}

// ============================================================================
// Module Summarisers
// ============================================================================

// ModuleSummariser abstracts the notion of a function which summarises the
// contents of a given column.
type ModuleSummariser struct {
	name        string
	description string
	summary     func(view.ModuleView) string
}

var moduleSumarisers = []ModuleSummariser{
	{"columns", "column count for module", moduleColumnSummariser},
	{"lines", "line count for module", moduleLineSummariser},
	{"bitwidth", "bitwidth of module", moduleBitwidthSummariser},
	{"cells", "total number of cells traced for module", moduleCellSummariser},
	{"nonzero", "total number of nonzero cells traced for module", moduleNonZeroCounter},
	{"bytes", "total number of bytes traced for module", moduleBytesSummariser},
}

func moduleCellSummariser(mod view.ModuleView) string {
	var (
		data    = mod.Data()
		mapping = data.Mapping()
		count   uint
	)
	//
	for _, reg := range activeRegisters(mod) {
		count += data.DataOf(mapping.LimbIds(reg)).Len()
	}
	//
	return fmt.Sprintf("%d", count)
}

func moduleColumnSummariser(mod view.ModuleView) string {
	return fmt.Sprintf("%d", len(activeRegisters(mod)))
}

func moduleLineSummariser(mod view.ModuleView) string {
	var (
		data     = mod.Data()
		width, _ = data.Dimensions()
	)
	//
	return fmt.Sprintf("%d", width)
}

func moduleBitwidthSummariser(mod view.ModuleView) string {
	var (
		total   = uint(0)
		data    = mod.Data()
		mapping = data.Mapping()
	)
	//
	for _, reg := range activeRegisters(mod) {
		total += data.DataOf(mapping.LimbIds(reg)).BitWidth()
	}
	//
	return fmt.Sprintf("%d", total)
}

func moduleBytesSummariser(mod view.ModuleView) string {
	var (
		total   = uint(0)
		data    = mod.Data()
		mapping = data.Mapping()
	)
	//
	for _, reg := range activeRegisters(mod) {
		reg := data.DataOf(mapping.LimbIds(reg))
		bitwidth := reg.BitWidth()
		byteWidth := bitwidth / 8
		// Determine proper bytewidth
		if bitwidth%8 != 0 {
			byteWidth++
		}
		//
		total += reg.Len() * byteWidth
	}
	//
	return fmt.Sprintf("%d", total)
}

func moduleNonZeroCounter(mod view.ModuleView) string {
	var (
		count   = uint(0)
		data    = mod.Data()
		mapping = data.Mapping()
	)

	for _, reg := range activeRegisters(mod) {
		count += nonZeroCount(data.DataOf(mapping.LimbIds(reg)))
	}
	//
	return fmt.Sprintf("%d", count)
}

func activeRegisters(mod view.ModuleView) []register.Id {
	var (
		window    = mod.Window()
		_, height = window.Dimensions()
		bits      bit.Set
		registers []register.Id
	)
	// NOTE: height - 1 because the view dimensions include the title row.
	for i := range height {
		var (
			sid = window.Row(i)
			col = mod.Data().SourceColumn(sid)
		)
		//
		if !bits.Contains(col.Register.Unwrap()) {
			registers = append(registers, col.Register)
			bits.Insert(col.Register.Unwrap())
		}
	}
	//
	return registers
}

// ============================================================================
// Column Summarisers
// ============================================================================

// ColumnSummariser abstracts the notion of a function which summarises the
// contents of a given column.
type ColumnSummariser struct {
	name        string
	description string
	summary     func(view.RegisterView) string
}

var columnSummarisers = []ColumnSummariser{
	{"lines", "line count for column", columnCountSummariser},
	{"bitwidth", "bitwidth for column as specified in trace file", columnBitwidthSummariser},
	{"bytes", "total bytes required for column", columnBytesSummariser},
	{"elements", "number of unique elements in column", uniqueElementsSummariser},
	{"entropy", "number of lines in column whose value differs from previous line", entropySummariser},
	{"nonzero", "number of lines in column whose value is not zero", nonZeroCounter},
}

// Used to show the available options on the command-line.
func summariserOptions() string {
	summarisers := "\n"
	//
	for _, s := range columnSummarisers {
		summarisers = fmt.Sprintf("%s--- %s (%s)\n", summarisers, s.name, s.description)
	}
	//
	return summarisers
}

func columnCountSummariser(col view.RegisterView) string {
	return fmt.Sprintf("%d", col.Len())
}

func columnBitwidthSummariser(col view.RegisterView) string {
	return fmt.Sprintf("%d", col.BitWidth())
}

func columnBytesSummariser(col view.RegisterView) string {
	bitwidth := col.BitWidth()
	byteWidth := bitwidth / 8
	// Determine proper bytewidth
	if bitwidth%8 != 0 {
		byteWidth++
	}

	return fmt.Sprintf("%d", col.Len()*byteWidth)
}

func uniqueElementsSummariser(data view.RegisterView) string {
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
	return fmt.Sprintf("%d", elems.Size())
}

func entropySummariser(data view.RegisterView) string {
	entropy := 0.0
	//
	if data.Len() > 0 {
		var (
			last  big.Int = data.Get(0)
			count         = 1
		)
		// Count all rows whose value differs from previous row.
		for i := uint(1); i < data.Len(); i++ {
			ith := data.Get(i)
			if last.Cmp(&ith) != 0 {
				count++
			}
			//
			last = ith
		}
		// Calculate entropy
		entropy = float64(count*100) / float64(data.Len())
	}
	// Done
	return fmt.Sprintf("%2.1f%%", entropy)
}

func nonZeroCounter(col view.RegisterView) string {
	return fmt.Sprintf("%d", nonZeroCount(col))
}

func nonZeroCount(data view.RegisterView) uint {
	var count = uint(0)
	// Count all non-zero rows.
	for i := range data.Len() {
		ith := data.Get(i)
		if ith.Sign() != 0 {
			count++
		}
	}
	//
	return count
}

// ============================================================================
// Trace Summarisers
// ============================================================================

type traceSummariser struct {
	name    string
	summary func(view.TraceView) string
}

var trSummarisers []traceSummariser = []traceSummariser{
	{"Cells", trCellCountSummariser},
	{"Cells (raw)", trRawCellCountSummariser},
	trWidthSummariser(1, 8),
	trWidthSummariser(9, 16),
	trWidthSummariser(17, 32),
	trWidthSummariser(33, 128),
	trWidthSummariser(129, 256),
}

func trRawCellCount(trace view.TraceView) uint {
	total := uint(0)
	//
	for i := range trace.Width() {
		var (
			mod     = trace.Module(i)
			data    = mod.Data()
			mapping = data.Mapping()
		)
		//
		for _, reg := range activeRegisters(mod) {
			total += data.DataOf(mapping.LimbIds(reg)).Len()
		}
	}
	//
	return total
}

func trRawCellCountSummariser(trace view.TraceView) string {
	total := trRawCellCount(trace)
	return fmt.Sprintf("%d", total)
}

const one_K = 1000
const one_M = one_K * one_K
const one_G = one_M * one_M

func trCellCountSummariser(trace view.TraceView) string {
	total := trRawCellCount(trace)
	//
	switch {
	case total > one_G:
		val := float64(total) / one_G
		return fmt.Sprintf("%.01fG", val)
	case total > one_M:
		val := float64(total) / one_M
		return fmt.Sprintf("%.01fM", val)
	case total > one_K:
		val := float64(total) / one_K
		return fmt.Sprintf("%.01fK", val)
	default:
		return fmt.Sprintf("%d", total)
	}
}

func trWidthSummariser(lowWidth uint, highWidth uint) traceSummariser {
	return traceSummariser{
		name: fmt.Sprintf("Columns (%d..%d bits)", lowWidth, highWidth),
		summary: func(tr view.TraceView) string {
			count := 0

			for i := range tr.Width() {
				var (
					mod     = tr.Module(i)
					data    = mod.Data()
					mapping = data.Mapping()
				)
				//
				for _, reg := range activeRegisters(mod) {
					ithWidth := data.DataOf(mapping.LimbIds(reg)).BitWidth()
					if ithWidth >= lowWidth && ithWidth <= highWidth {
						count++
					}
				}
			}

			return fmt.Sprintf("%d", count)
		},
	}
}
