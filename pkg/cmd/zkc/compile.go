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
	"os"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/debug"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var compileCmd = &cobra.Command{
	Use:   "compile [flags] file1.zkc file2.zkc ...",
	Short: "compile zkc source files into a binary package.",
	Long:  `Compile a given set of source file(s) into a single binary package.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, compileCmds)
	},
}

// compileFlags captures the permitted flag combinations for the compile command.
var compileFlags FlagChecks

// Available instances
var compileCmds = []FieldAgnosticCmd{
	{field.GF_251, runCompileCmd[gf251.Element]},
	{field.GF_8209, runCompileCmd[gf8209.Element]},
	{field.KOALABEAR_16, runCompileCmd[koalabear.Element]},
	{field.BLS12_377, runCompileCmd[bls12_377.Element]},
}

// CompileConfig brings together various configuration options specific to this
// command.
type CompileConfig struct {
	build BuildConfig
	// indicates whether or not to print the Abstract Syntax Tree (when available)
	ast bool
	// indicates whether or not to print the raw Intermediate Reprentation
	raw bool
	// indicates whether or not to print the Mid-level Intermediate Reprentation (MIR).
	mir bool
	// indicates whether or not to print the Mid-level Intermediate Reprentation (AIR).
	air bool
	// indicates whether or not to print summary statistics about the generated
	// AIR schema.
	stats bool
	// statsMatrix enables the secondary --stats output (the degree/cell matrix),
	// which -v turns on
	statsMatrix bool
	// order determines how modules are ordered in the --stats output
	// (name|total|complexity|lookups).
	order string
	// indicates whether or not to print everything in full (e.g. including
	// static reference tables).
	verbose bool
}

func runCompileCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build  = GetBuildConfig[F](cmd, field)
		output = GetString(cmd, "output")
		config CompileConfig
	)
	// Sanity check permitted flag combinations
	checkFlags(cmd, compileFlags)
	//
	config.ast = GetFlag(cmd, "ast")
	config.raw = GetFlag(cmd, "raw")
	config.mir = GetFlag(cmd, "mir")
	config.air = GetFlag(cmd, "air")
	config.stats = GetFlag(cmd, "stats")
	config.order = GetString(cmd, "order")
	config.statsMatrix = GetVerboseLevel(cmd) >= VERBOSE_INFO
	// Compile verbosity is the highest verbosity level.
	config.verbose = GetVerboseLevel(cmd) >= VERBOSE_PRINTF
	// Configure metadata for the binary output file.  Observe this is left
	// unset (rather than empty) when no definitions are given, so that metadata
	// embedded in a prebuilt binary is preserved.
	if cmd.Flags().Changed("define") {
		build.metadata = util.Some(buildMetadata(GetStringArray(cmd, "define")))
	}
	//
	config.build = build
	// Build all artifacts
	ast, binfile := Build[F](build, args...)
	// Perform validation.  This comes before anything is printed or written, so
	// that an invalid set of constraints cannot leave a binary file behind.
	validateArtifacts(binfile)
	//
	if output != "" {
		// Write to disk
		WriteBinaryFile(binfile, output)
	} else {
		// Print out requested artifacts
		printArtifacts(ast, binfile, config)
	}
}

// buildMetadata converts a set of "key=value" definitions, as given on the
// command-line via -D, into the JSON blob embedded in the binary file header.
func buildMetadata(items []string) []byte {
	var metadata = make(map[string]any)
	//
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		//
		if !ok {
			fmt.Printf("malformed definition \"%s\"\n", item)
			os.Exit(2)
		}
		//
		metadata[key] = value
	}
	//
	m := typed.NewMap(metadata)
	//
	bytes, err := m.ToJsonBytes()
	if err != nil {
		fmt.Printf("error encoding metadata: %s\n", err)
		os.Exit(2)
	}
	//
	return bytes
}

// Validate the given schema by ensuring that:
// - every register in every module is referenced in at least one vanishing
// constraint or lookup.
// - static tables are of power of two height
func validateArtifacts[F field.Element[F]](bf *constraints.BinaryFile[F]) {
	var air = bf.AirConstraints()
	// validate that all registers are referenced in at least one vanishing constraint or lookup
	if errs := constraints.Validate(air); len(errs) > 0 {
		for _, err := range errs {
			log.Errorf("%v", err)
		}
		// Exit with error if any errors triggered
		os.Exit(1)
	}
}

func printArtifacts[F field.Element[F]](ast *ast.Program, bf *constraints.BinaryFile[F], config CompileConfig) {
	// Determine whether or not to print the IR
	var ir = !(config.ast || config.mir || config.air || config.stats)
	// Abstract Syntax Tree
	if config.ast && ast != nil {
		writeAbstractSyntaxTree(*ast)
	} else if config.ast {
		log.Warn("Abstract Syntax Tree unavailable")
	}
	// Word-level Intermediate Representation
	if ir && config.raw {
		// Raw IR
		writeBytecodeProgram(config.verbose, ast, bf.RawProgram())
	} else if ir && config.build.fastMode {
		// Execution IR
		writeBytecodeProgram(config.verbose, ast, bf.ExecutionProgram())
	} else if ir {
		// Tracing IR
		writeBytecodeProgram(config.verbose, ast, bf.TracingProgram())
	}
	// Mid-level Intermediate Representation
	if config.mir {
		debug.PrintAnySchema(bf.MirConstraints(), 80, config.verbose)
	}
	// Arithmetic Intermediate Representation
	if config.air {
		debug.PrintAnySchema(bf.AirConstraints(), 80, config.verbose)
	}
	// Summary statistics
	if config.stats {
		if !ValidStatsOrder(config.order) {
			log.Errorf("invalid --order %q (expected name|total|complexity|lookups)", config.order)
			os.Exit(1)
		}
		// Register counts are reported before register splitting.  Splitting is
		// the only field-specific transform that changes register widths, so
		// transform program with splitting disabled to recover the pre-split widths.
		preSplit := vm.TransformForTracing[vm.Uint, vm.Uint](bf.RawProgram(), "split-registers")
		// Print stats
		PrintCompileStats(bf.AirConstraints(), preSplit, config.order, config.statsMatrix)
	}
}

// ============================================================================
// AST
// ============================================================================

func writeAbstractSyntaxTree(program ast.Program) {
	var env = ast.NewEnvironment()
	//
	for i, d := range program.Components() {
		if i != 0 {
			fmt.Println()
		}
		//
		writeDeclaration(d, env)
	}
}

func writeDeclaration(d decl.Resolved, env data.ResolvedEnvironment) {
	switch d := d.(type) {
	case *decl.ResolvedConstant:
		writeConstant(d, env)
	case *decl.ResolvedFunction:
		writeFunction(d, env)
	case *decl.ResolvedMemory:
		writeMemory(d, env)
	case *decl.ResolvedTypeAlias:
		writeTypeAlias(d, env)
	default:
		panic(fmt.Sprintf("unknown declaration encountered (%v)", d))
	}
}

func writeTypeAlias(t *decl.ResolvedTypeAlias, env data.ResolvedEnvironment) {
	fmt.Printf("type %s = %s\n", t.Name(), t.DataType.String(env))
}

func writeConstant(m *decl.ResolvedConstant, env data.ResolvedEnvironment) {
	var mapping = variable.ArrayMap[symbol.Resolved]()
	//
	fmt.Print("const ")
	// type
	fmt.Printf("%s ", m.DataType.String(env))
	// name
	fmt.Printf("%s = ", m.Name())
	// contents
	fmt.Println(m.ConstExpr.String(mapping))
}

func writeAnnotations(annotations []string) {
	for _, a := range annotations {
		fmt.Printf("#[%s]\n", a)
	}
}

func writeMemory(m *decl.ResolvedMemory, env data.ResolvedEnvironment) {
	writeAnnotations(m.Annotations())
	//
	switch m.Kind {
	case decl.PUBLIC_READ_ONLY_MEMORY:
		fmt.Printf("public input")
	case decl.PRIVATE_READ_ONLY_MEMORY:
		fmt.Printf("input")
	case decl.PUBLIC_WRITE_ONCE_MEMORY:
		fmt.Printf("public output")
	case decl.PRIVATE_WRITE_ONCE_MEMORY:
		fmt.Printf("output")
	case decl.PUBLIC_STATIC_MEMORY:
		fmt.Printf("public static")
	case decl.PRIVATE_STATIC_MEMORY:
		fmt.Printf("static")
	case decl.RANDOM_ACCESS_MEMORY:
		fmt.Printf("memory")
	}

	fmt.Printf(" %s", m.Name())
	// timestamp type (read-write memory only)
	if m.TimestampType != nil {
		fmt.Printf("[%s]", m.TimestampType.String(env))
	}
	// address lines
	fmt.Printf("(")
	writeMemoryParams(m.Address, env)
	fmt.Printf(") -> (")
	writeMemoryParams(m.Data, env)
	fmt.Printf(")")
	//
	if m.Contents != nil {
		fmt.Println(" = {")
		writeMemoryContents(m.Contents)
		fmt.Printf("}")
	}
	//
	fmt.Println()
}

func writeMemoryParams(params []variable.ResolvedDescriptor, env data.ResolvedEnvironment) {
	for i, p := range params {
		if i > 0 {
			fmt.Printf(", ")
		}

		fmt.Printf("%s %s", p.DataType.String(env), p.Name)
	}
}

func writeMemoryContents(values []expr.Resolved) {
	var N = 20
	//
	for i := 0; i < len(values); i += N {
		var left = len(values) - i
		//
		for j := range min(N, left) {
			fmt.Printf("%s", values[i+j].String(variable.ArrayMap[symbol.Resolved]()))
			//
			if i+j+1 != len(values) {
				fmt.Printf(", ")
			}
		}
		//
		fmt.Println()
	}
}

func writeFunction(f *decl.ResolvedFunction, env data.ResolvedEnvironment) {
	writeAnnotations(f.Annotations())
	//
	fmt.Printf("fn %s", f.Name())
	// Write optional effects
	if len(f.Effects) > 0 {
		writeEffects(f.Effects)
	}
	//
	fmt.Printf("(")
	// parameters
	writeFunctionArgs(variable.PARAMETER, f.Variables, env)
	//
	fmt.Printf(") -> (")
	// returns
	writeFunctionArgs(variable.RETURN, f.Variables, env)
	//
	fmt.Println(") {")
	//
	writeFunctionVariables(f, env)
	//
	for pc, insn := range f.Code {
		fmt.Printf("[%d]\t%s\n", pc, insn.String(f))
	}
	// Done
	fmt.Println("}")
}

func writeEffects(effects []*symbol.Resolved) {
	fmt.Print("<")
	//
	for i, effect := range effects {
		if i != 0 {
			fmt.Print(",")
		}
		//
		fmt.Print(effect)
	}
	//
	fmt.Print(">")
}

func writeFunctionArgs(kind variable.Kind, variables []variable.ResolvedDescriptor, env data.ResolvedEnvironment) {
	var first = true
	//
	for _, r := range variables {
		if r.Kind == kind {
			if !first {
				fmt.Printf(", ")
			} else {
				first = false
			}
			//
			fmt.Printf("%s:%s", r.Name, r.DataType.String(env))
		}
	}
}

func writeFunctionVariables(f *decl.ResolvedFunction, env data.ResolvedEnvironment) {
	for _, r := range f.Variables {
		if r.IsLocal() {
			fmt.Printf("\tvar %s:%s\n", r.Name, r.DataType.String(env))
		}
	}
}

// ============================================================================
// Bytecode Interpreter
// ============================================================================

// bytecodeListing accumulates the rows of a disassembly table.  Each row is
// stored as its "core" cells plus a parallel flow-graph string.  When binary
// output is enabled the core column layout is [address, encoding, marker,
// text]; otherwise it is just [marker, text].  The trailing "text" column
// carries all assembly content (function signatures, register declarations,
// instructions and memory declarations); the leading columns are populated
// only on instruction rows.  A final flow column (rendering the control-flow
// ranges of skip instructions) is appended to the right of the text column,
// but only when at least one row actually has flow content.
type bytecodeListing struct {
	// binary indicates whether the address / encoding columns are present.
	binary bool
	// encodingWidth is the fixed width to which every encoding cell is padded,
	// so the encoding column lines up across all functions in the program.
	encodingWidth uint
	// rows holds the accumulated core cells, each of coreWidth() cells.
	rows [][]termio.FormattedText
	// flows holds the flow-graph rendering for each row (parallel to rows);
	// the empty string denotes a row with no flow content.
	flows []string
}

// coreWidth returns the number of core columns (i.e. excluding the flow
// column) in the listing.
func (p *bytecodeListing) coreWidth() uint {
	if p.binary {
		return 4
	}
	//
	return 2
}

// blank returns a fresh row of empty core cells of the correct width.
func (p *bytecodeListing) blank() []termio.FormattedText {
	row := make([]termio.FormattedText, p.coreWidth())
	//
	for i := range row {
		row[i] = termio.NewText("")
	}
	//
	return row
}

// darkGrey is the escape used to render variable (register) declarations, and
// darkYellow the escape used to render the skip-arrow flow column.
var (
	darkGrey   = termio.NewAnsiEscape().Fg256Colour(240)
	darkYellow = termio.NewAnsiEscape().Fg256Colour(136)
)

// addText appends a text-only row (with no flow content), placing the given
// (possibly formatted) cell in the trailing text column.
func (p *bytecodeListing) addText(cell termio.FormattedText) {
	row := p.blank()
	row[p.coreWidth()-1] = cell
	p.rows = append(p.rows, row)
	p.flows = append(p.flows, "")
}

// add appends an unformatted text-only row (e.g. an annotation), placed in the
// trailing text column.
func (p *bytecodeListing) add(text string) {
	p.addText(termio.NewText(text))
}

// addDeclaration appends a variable (register) declaration row, rendered in
// dark grey.
func (p *bytecodeListing) addDeclaration(text string) {
	p.addText(termio.NewFormattedText(text, darkGrey))
}

// addInstruction appends an instruction row, with the instruction text itself
// rendered in the default colour and flow holding the control-flow rendering
// for this row.  The address and encoding are omitted when binary output is
// disabled.
func (p *bytecodeListing) addInstruction(address, encoding, marker, text, flow string) {
	row := p.blank()
	instruction := termio.NewText(text)
	//
	if p.binary {
		// Pad the encoding to the program-wide width so the column lines up
		// across every function.
		encoding = fmt.Sprintf("%*s", int(p.encodingWidth), encoding)
		row[0] = termio.NewFormattedText(address, darkYellow)
		row[1] = termio.NewFormattedText(encoding, darkGrey)
		row[2] = termio.NewText(marker)
		row[3] = instruction
	} else {
		row[0] = termio.NewText(marker)
		row[1] = instruction
	}
	//
	p.rows = append(p.rows, row)
	p.flows = append(p.flows, flow)
}

// hasFlow reports whether any row carries flow content, and hence whether the
// flow column should be included in the rendered table.
func (p *bytecodeListing) hasFlow() bool {
	for _, f := range p.flows {
		if f != "" {
			return true
		}
	}
	//
	return false
}

// table renders the accumulated rows into a FormattedTable, appending the flow
// column to the right of the core columns when any row has flow content.
func (p *bytecodeListing) table() *termio.FormattedTable {
	var (
		flow  = p.hasFlow()
		width = p.coreWidth()
	)
	//
	if flow {
		width++
	}
	//
	tbl := termio.NewFormattedTable(width, uint(len(p.rows)))
	// Drop the vertical lines between columns.
	tbl.SetSeparator("")
	// Left-align the text column (function signatures, register declarations
	// and instructions), so the assembly reads flush-left.
	tbl.SetLeftAlign(p.coreWidth() - 1)
	//
	for i, row := range p.rows {
		if flow {
			row = append(row, termio.NewFormattedText(p.flows[i], darkYellow))
		}
		//
		tbl.SetRow(uint(i), row...)
	}
	//
	return tbl
}

func writeBytecodeProgram[W vm.Word[W]](binary bool, ast *ast.Program, program vm.Program[W]) {
	var (
		bin           [][]uint32
		address       uint32
		encodingWidth uint
		annotations   = annotationsOf(ast)
	)
	//
	if binary {
		// Extract encoding for all bytecodes
		bin = vm.CompileProgram(program).Encoding()
		// Determine the widest encoding across the entire program, so the
		// encoding column can be given a uniform width in every function.
		for _, codes := range bin {
			encodingWidth = max(encodingWidth, uint(len(fmt.Sprintf("%08x", codes))))
		}
	}
	//
	for i, m := range program.Modules() {
		if i != 0 {
			// Blank line between module tables
			fmt.Println()
		}
		// Write and print this module's table
		address, bin = writeBytecodeModule(binary, encodingWidth, uint16(i), program, m, annotations, address, bin)
	}
}

// annotationsOf extracts the annotations associated with each annotated
// declaration (i.e. function or memory) in a given program, keyed by the
// declaration's name.
func annotationsOf(program *ast.Program) map[string][]string {
	var annotations = make(map[string][]string)
	//
	if program != nil {
		for _, d := range program.Components() {
			if annots := d.Annotations(); len(annots) > 0 {
				annotations[d.Name()] = annots
			}
		}
	}
	//
	return annotations
}

// writeBytecodeModule builds and prints the FormattedTable for a single module.
// The encoding stream (bin) and running instruction address are threaded
// through (and returned) since they accumulate across the whole program.
func writeBytecodeModule[W vm.Word[W]](binary bool, encodingWidth uint, fid uint16, program vm.Program[W],
	m vm.Module[W], annotations map[string][]string, address uint32, bin [][]uint32) (uint32, [][]uint32) {
	//
	listing := &bytecodeListing{binary: binary, encodingWidth: encodingWidth}
	// Write any annotations for this module
	for _, a := range annotations[m.Name()] {
		listing.add(fmt.Sprintf("#[%s]", a))
	}
	// Write module contents
	switch m := m.(type) {
	case *vm.Function[W]:
		address, bin = writeBytecodeFunction(listing, address, program.EnvironmentOf(fid), m, bin)
	case *vm.Memory[W]:
		writeBytecodeMemory(listing, m)
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", m.Name()))
	}
	//
	writeModuleSignature(m)
	// Print this module's table
	listing.table().Print(true)
	//
	return address, bin
}

func writeModuleSignature[W vm.Word[W]](m vm.Module[W]) {
	// Write module contents
	switch m := m.(type) {
	case *vm.Function[W]:
		fmt.Printf("fn %s\n", signatureOf(m))
	case *vm.Memory[W]:
		//
		if m.IsPublic() {
			fmt.Print("pub ")
		}
		//
		switch {
		case m.IsStatic():
			fmt.Print("static ")
		case m.IsReadOnly():
			fmt.Print("input ")
		case m.IsWriteOnly():
			fmt.Print("output ")
		default:
			fmt.Print("memory ")
		}
		//
		fmt.Printf("%s\n", signatureOf(m))
	default:
		panic(fmt.Sprintf("unknown module \"%s\" encountered", m.Name()))
	}
}

func writeBytecodeFunction[W vm.Word[W]](listing *bytecodeListing, address uint32, env vm.BytecodeEnvironment[W],
	f *vm.Function[W], bin [][]uint32) (uint32, [][]uint32) {
	for _, r := range f.Registers() {
		if !r.IsInputOutput() {
			listing.addDeclaration(fmt.Sprintf("  %s %s", regType(r), r.Name()))
		}
	}
	// First pass: gather each bytecode's display fields, and record the
	// control-flow range of every skip instruction.  Flow ranges are expressed
	// in bytecode-row indices local to this function (row 0 == first bytecode).
	var (
		flow  util.FlowGraph
		insns []bytecodeRow
		idx   uint
	)
	//
	for pc, vec := range f.Vectors() {
		for cc, b := range vec.Bytecodes {
			var row bytecodeRow
			// Include low-level information (if requested)
			if bin != nil {
				var codes = array.Reverse(bin[0])
				//
				row.address = fmt.Sprintf("0x%04x", address)
				row.encoding = fmt.Sprintf("%08x", codes)
				//
				address += uint32(len(codes))
				bin = bin[1:]
			}
			//
			if cc == 0 {
				row.marker = fmt.Sprintf("[%02d]", pc)
			}
			// Sanity check to prevent crashing even in the presence of invalid
			// structure.
			if b == nil {
				row.text = "???"
			} else {
				row.text = b.String(env)
				// A skip's range spans from this row to each of its targets.
				for _, target := range vm.SkipTargets(b, idx) {
					flow.Add(idx, target)
				}
			}
			//
			insns = append(insns, row)
			idx++
		}
	}
	// Render the flow graph across all bytecode rows of this function.
	arrows := flow.Render(idx)
	// Second pass: emit each instruction row alongside its flow-graph column.
	for i, row := range insns {
		listing.addInstruction(row.address, row.encoding, row.marker, row.text, arrows[i])
	}
	//
	return address, bin
}

// bytecodeRow holds the display fields of a single instruction row, gathered in
// a first pass so that its flow-graph column can be rendered once all skip
// ranges within the function are known.
type bytecodeRow struct {
	address  string
	encoding string
	marker   string
	text     string
}

func regType[W vm.Word[W]](r vm.Register[W]) string {
	if r.IsNative() {
		return "𝔽"
	}
	//
	return fmt.Sprintf("u%d", r.Bitwidth().Unwrap())
}

func writeBytecodeMemory[W vm.Word[W]](listing *bytecodeListing, m *vm.Memory[W]) {
	//
	if m.IsStatic() {
		var builder strings.Builder
		builder.WriteString("{")
		//
		for i, v := range m.StaticContents() {
			if i > 5 {
				builder.WriteString(", ...")
				break
			} else if i != 0 {
				builder.WriteString(", ")
			}
			//
			fmt.Fprintf(&builder, "0x%s", v.Text(16))
		}
		//
		builder.WriteString("}")
		//
		listing.addText(termio.NewText(builder.String()))
	}
}

func signatureOf[W vm.Word[W]](m vm.Module[W]) string {
	var (
		args = array.Filter(m.Registers(), func(r vm.Register[W]) bool {
			return r.IsInput()
		})
		returns = array.Filter(m.Registers(), func(r vm.Register[W]) bool {
			return r.IsOutput()
		})
		// Read-write memories carry their timestamp width in the signature.
		stamp string
	)
	//
	if mem, ok := m.(*vm.Memory[W]); ok && mem.IsReadWrite() {
		stamp = fmt.Sprintf("[u%d]", mem.TimestampWidth().Unwrap())
	}
	//
	return fmt.Sprintf("%s%s(%s) -> (%s)", m.Name(), stamp, fnArgs(args), fnArgs(returns))
}

func fnArgs[W vm.Word[W]](regs []vm.Register[W]) string {
	var builder strings.Builder
	//
	for i, r := range regs {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(r.Name())
		builder.WriteString(":")
		//
		if r.IsNative() {
			builder.WriteString("𝔽")
		} else {
			fmt.Fprintf(&builder, "u%d", r.Bitwidth().Unwrap())
		}
	}
	//
	return builder.String()
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(compileCmd)
	compileCmd.Flags().StringP("output", "o", "", "specify output file for writing binary constraints")
	compileCmd.Flags().StringArrayP("define", "D", nil,
		"define a metadata attribute (as \"key=value\") to embed in the binary output file")
	compileCmd.PersistentFlags().Bool("ast", false, "Output Abstract Syntax Tree (AST)")
	compileCmd.PersistentFlags().Bool("raw", false, "Output raw Intermediate (Bytecode) Representation (IR)")
	compileCmd.PersistentFlags().Bool("mir", false, "Output Mid-Level Intermediate Representation (MIR)")
	compileCmd.PersistentFlags().Bool("air", false, "Output Arithmetic Intermediate Representation (AIR)")
	compileCmd.PersistentFlags().Bool("stats", false, "Output summary statistics")
	compileCmd.PersistentFlags().String("order", "total",
		"module ordering for --stats (name|total|complexity|lookups)")
	// --order only affects the --stats output.
	compileFlags.Require("order", "stats")
	// --define only affects the binary output file.
	compileFlags.Require("define", "output")
}
