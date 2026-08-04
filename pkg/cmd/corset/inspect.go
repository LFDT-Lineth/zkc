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
	"os"

	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/inspector"
	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/view"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [flags] trace_file constraint_file(s)",
	Short: "Inspect a trace file",
	Long:  `Inspect a trace file using an interactive (terminal-based) environment`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, inspectCmds)
	},
}

// Available instances
var inspectCmds = []FieldAgnosticCmd{
	{field.GF_251, runInspectCmd[gf251.Element]},
	{field.GF_8209, runInspectCmd[gf8209.Element]},
	{field.KOALABEAR_16, runInspectCmd[koalabear.Element]},
	{field.BLS12_377, runInspectCmd[bls12_377.Element]},
}

func runInspectCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	var (
		errors []error
		trace  tr.Trace[F]
	)
	//
	if len(args) != 2 {
		fmt.Println(cmd.UsageString())
		os.Exit(1)
	}
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}
	//
	validate := GetFlag(cmd, "validate")
	showLimbs := GetFlag(cmd, "show-limbs")
	cellWidth := GetUint(cmd, "cell-width")
	titleWidth := GetUint(cmd, "title-width")
	// Read in constraint files
	stacker := *getSchemaStack[F](cmd, SCHEMA_DEFAULT_AIR, args[1:]...)
	stack := stacker.Build()
	//
	stats := util.NewPerfStats()
	// Parse constraints
	binf := stacker.BinaryFile()
	// Sanity check debug information is available.
	srcmap, srcmap_ok := binfile.GetAttribute[*corset.SourceMap](binf)
	//
	if !srcmap_ok {
		fmt.Printf("binary file \"%s\" missing source map", args[1])
	}
	//
	stats.Log("Reading constraints file")
	// Parse trace file
	tracefile := ReadTraceFile(args[0])
	// Extract schema
	schema := stack.ConcreteSchema()
	//
	stats.Log("Reading trace file")
	// Apply trace expansion
	if len(errors) != 0 && validate {
		fmt.Println("(use --validate=false to ignore trace propagation errors)")
		fmt.Println()
	} else {
		trace, errors = stack.TraceBuilder().Build(schema, tracefile)
	}
	//
	if len(errors) == 0 {
		// Run the inspector.
		errors = inspect(stack.TraceBuilder().Mapping(), srcmap, trace, showLimbs, cellWidth, titleWidth)
	}
	// Sanity check what happened
	if len(errors) > 0 {
		for _, err := range errors {
			log.Errorln(err)
		}

		os.Exit(1)
	}
}

// InspectTrace opens the given trace in the interactive inspector using the
// supplied register mapping, without requiring a corset source map.  Columns are
// shown as computed (low-level) registers.  This is intended for callers (e.g.
// the zkc toolchain) which generate a trace but have no source map available.
//
// The optional "public" predicate determines which modules are publicly visible
// (shown by default); when nil, all modules are public.  Callers use this to
// hide synthetic modules such as range-check tables.
func InspectTrace[F field.Element[F]](mapping module.LimbsMap, trace tr.Trace[F],
	public func(tr.ModuleName) bool, limbs bool, cellWidth, titleWidth uint) []error {
	//
	term, err := termio.NewTerminal()
	if err != nil {
		return []error{err}
	}
	// Build the viewing window (no source map, so show computed registers).
	builder := view.NewBuilder[F](mapping).
		WithTitleWidth(titleWidth).
		WithCellWidth(cellWidth).
		WithFormatting(inspector.NewFormatter()).
		WithLimbs(limbs).
		WithComputed(true)
	// Apply the visibility predicate (if any).
	if public != nil {
		builder = builder.WithVisibility(public)
	}
	//
	window := builder.Build(trace)
	// Construct the inspector.
	insp := inspector.NewInspector(term, window)
	// Render it.
	if err := insp.Render(); err != nil {
		return []error{err}
	}
	//
	return insp.Start()
}

// Inspect a given trace using a given schema.
func inspect[F field.Element[F]](mapping module.LimbsMap, srcmap *corset.SourceMap, trace tr.Trace[F],
	limbs bool, cellWidth, titleWidth uint) []error {
	// Construct inspector window
	inspector := construct(mapping, trace, srcmap, limbs, cellWidth, titleWidth)
	// Render inspector
	if err := inspector.Render(); err != nil {
		return []error{err}
	}
	//
	return inspector.Start()
}

func construct[F field.Element[F]](mapping module.LimbsMap, trace tr.Trace[F], srcmap *corset.SourceMap, limbs bool,
	cellWidth, titleWidth uint) *inspector.Inspector {
	//
	term, err := termio.NewTerminal()
	// Check whether successful
	if err == nil {
		window := view.NewBuilder[F](mapping).
			WithSourceMap(*srcmap).
			WithTitleWidth(titleWidth).
			WithCellWidth(cellWidth).
			WithFormatting(inspector.NewFormatter()).
			WithLimbs(limbs).
			WithComputed(true).
			Build(trace)
		// Construct inspector state
		return inspector.NewInspector(term, window)
	}

	fmt.Println(error.Error(err))
	os.Exit(1)
	// Unreachable
	return nil
}

//nolint:errcheck
func init() {
	rootCmd.AddCommand(inspectCmd)
	inspectCmd.Flags().Bool("show-limbs", false, "specify whether to show register limbs")
	inspectCmd.Flags().Uint("cell-width", 32, "specify maximum display width for a cell")
	inspectCmd.Flags().Uint("title-width", 128, "specify maximum display width for a column title")
}
