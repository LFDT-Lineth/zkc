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

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/debug"
	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
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
	expansion in order to debug them.`,
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

	srcmapOnly := GetFlag(cmd, "source-map")
	showStatic := GetFlag(cmd, "show-static")
	textWidth := GetUint(cmd, "textwidth")
	// Read in constraint files
	stacker := *getSchemaStack[F](cmd, SCHEMA_DEFAULT_MIR, args...)
	stack := stacker.Build()
	// Print source map (if requested)
	if srcmapOnly {
		printSourceMap(&stack)
	} else {
		debug.PrintSchemas(stack, textWidth, showStatic)
	}
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.Flags().Bool("source-map", false, "Print source map information")
	debugCmd.Flags().Bool("show-static", false, "Show static tables when printing schemas")
	debugCmd.Flags().Uint("textwidth", 130, "Set maximum textwidth to use")
}

func printSourceMap[F field.Element[F]](stack *cmd_util.SchemaStack[F]) {
	srcmap, ok := stack.SourceMap()
	// Sanity check debug information is available.
	if !ok {
		fmt.Println("constraints missing source map")
		os.Exit(1)
	}
	//
	debug.PrintSourceMap(srcmap)
}
