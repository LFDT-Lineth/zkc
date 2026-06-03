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
	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/debug"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug [flags] constraint_file",
	Short: "print constraints at various levels of expansion.",
	Long: `Print a given set of constraints at specific levels of
	expansion in order to debug them.  Constraints can be given
	either as lisp or bin files.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, debugCmds)
	},
}

// Available instances
var debugCmds = []FieldAgnosticCmd{
	{field.GF_251, runDebugCmd[gf251.Element]},
	{field.GF_8209, runDebugCmd[gf8209.Element]},
	{field.KOALABEAR_16, runDebugCmd[koalabear.Element]},
	{field.BLS12_377, runDebugCmd[bls12_377.Element]},
}

func runDebugCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Println(cmd.UsageString())
		os.Exit(1)
	}
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}

	stats := GetFlag(cmd, "stats")
	modules := GetFlag(cmd, "modules")
	attrs := GetFlag(cmd, "attributes")
	metadata := GetFlag(cmd, "metadata")
	constants := GetFlag(cmd, "constants")
	spillage := GetFlag(cmd, "spillage")
	textWidth := GetUint(cmd, "textwidth")
	sort := GetUint(cmd, "sort")
	// Read in constraint files
	stacker := *getSchemaStack[F](cmd, SCHEMA_DEFAULT_MIR, args...)
	stack := stacker.Build()
	// Print constant info (if requested)
	if constants {
		debug.PrintExternalisedConstants(stack.BinaryFile())
	}
	// Print spillage info (if requested)
	if spillage {
		printSpillage(stack.BinaryFile(), true)
	}
	// Print meta-data (if requested)
	if metadata {
		printBinaryFileHeader(stack.BinaryFile())
	}
	// Print stats (if requested)
	if stats {
		debug.PrintStats(stack)
	}
	// Print module stats (if requested)
	if modules {
		debug.PrintModuleStats(stack, 32, sort)
	}
	// Print embedded attributes (if requested
	if attrs {
		printAttributes(stack.BinaryFile())
	}
	//
	if !stats && !modules && !attrs {
		debug.PrintSchemas(stack, textWidth)
	}
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.Flags().Bool("attributes", false, "Print attribute information")
	debugCmd.Flags().Bool("constants", false, "Print information about externalised constants")
	debugCmd.Flags().Bool("metadata", false, "Print embedded metadata")
	debugCmd.Flags().Bool("stats", false, "Print summary information")
	debugCmd.Flags().BoolP("modules", "m", false, "show module stats")
	debugCmd.Flags().Bool("spillage", false, "Print spillage information")
	debugCmd.Flags().Uint("textwidth", 130, "Set maximum textwidth to use")
	debugCmd.Flags().Uint("sort", 0, "sort table column")
}

func printAttributes(binf *binfile.BinaryFile) {
	// Print attributes
	for _, attr := range binf.Attributes {
		fmt.Printf("attribute \"%s\":\n", attr.AttributeName())
		//
		if attr.AttributeName() == "CorsetSourceMap" {
			debug.PrintSourceMap(attr.(*corset.SourceMap))
		}
	}
}

func printSpillage(binf *binfile.BinaryFile, defensive bool) {
	// fmt.Println("Spillage:")
	// // Compute spillage for optimisation level
	// spillage := determineSpillage(&binf.Schema, defensive, optConfig)
	// // Define module ID
	// mid := uint(0)
	// // Iterate modules and print spillage
	// for i := uint(0); i < uint(len(spillage)); i++ {
	// 	name := binf.Schema.Module(i).Name()
	// 	//
	// 	if name == "" {
	// 		name = "<prelude>"
	// 	}
	// 	//
	// 	fmt.Printf("\t%s: %d\n", name, spillage[i])
	// 	//
	// 	mid++
	// }
	panic("todo")
}

func printBinaryFileHeader(binf *binfile.BinaryFile) {
	header := binf.Header
	//
	fmt.Printf("Format: %d.%d\n", header.MajorVersion, header.MinorVersion)
	// Attempt to parse metadata
	metadata, err := header.GetMetaData()
	//
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	} else if !metadata.IsEmpty() {
		fmt.Println("Metadata:")
		//
		printTypedMetadata(1, metadata)
	}
}
