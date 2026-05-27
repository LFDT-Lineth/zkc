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

	"github.com/consensys/go-corset/pkg/cmd/zkc/debug"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/field/bls12_377"
	"github.com/consensys/go-corset/pkg/util/field/gf251"
	"github.com/consensys/go-corset/pkg/util/field/gf8209"
	"github.com/consensys/go-corset/pkg/util/field/koalabear"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug [flags] input.json file1.zkc file2.zkc ...",
	Short: "Debug a zkc program.",
	Long:  `Debug a zkc program to produce a set of outputs a from given a set of inputs.`,
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

func runDebugCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build    = GetBuildConfig[F](cmd, field)
		observer = debug.TraceObserver[vm.Uint]{}
		quiet    = GetFlag(cmd, "quiet")
	)
	// Suppress printf debug instructions when quiet mode is enabled.
	build.config = build.config.Quiet(quiet)
	// Force compilation of the word machine, which is what we execute.
	build.wir = true
	//
	input := ParseInputFile(args[0])
	// Build artifacts (compiles source files or loads a prebuilt binary).
	artifacts := build.Build(args[1:]...)
	wm := artifacts.wir.Unwrap()
	// Decode inputs against the compiled machine.
	inputs, errs := vm.DecodeInputs(&wm, input)
	if len(errs) == 0 {
		if err := wm.Boot("main", inputs); err != nil {
			errs = append(errs, err)
		} else if _, err := vm.ExecuteAndObserve(&wm, 1, &observer); err != nil {
			errs = append(errs, err)
		}
	}
	//
	if len(errs) > 0 {
		for _, e := range errs {
			log.Error(e)
		}
		//
		os.Exit(4)
	}
	//
	fmt.Println()
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.Flags().BoolP("quiet", "q", false, "suppress printf output")
}
