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
package zkc

import (
	"fmt"
	"os"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/goldilocks"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/mamabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/mamabear2"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var executeCmd = &cobra.Command{
	Use:     "execute [flags] input.json file1.zkc file2.zkc ...",
	Short:   "Execute a zkc program.",
	Long:    `Execute a zkc program to produce a set of outputs a from given a set of inputs.`,
	Aliases: []string{"exec"},
	Args:    cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, executeCmds)
	},
}

// Available instances
var executeCmds = []FieldAgnosticCmd{
	{field.GF_251, runExecuteCmd[gf251.Element, vm.Uint32]},
	{field.GF_8209, runExecuteCmd[gf8209.Element, vm.Uint32]},
	{field.KOALABEAR_16, runExecuteCmd[koalabear.Element, vm.Uint32]},
	{field.KOALABEAR_24, runExecuteCmd[koalabear.Element, vm.Uint32]},
	{field.GOLDILOCKS_32, runExecuteCmd[goldilocks.Element, vm.Uint64]},
	{field.MAMABEAR_32, runExecuteCmd[mamabear.Element, vm.Uint64]},
	{field.MAMABEAR2_32, runExecuteCmd[mamabear2.Element, vm.Uint64]},
}

// Permitted flag combinations
var executeFlags FlagChecks

func runExecuteCmd[F field.Element[F], W vm.Word[W]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		errors  []error
		build   = GetBuildConfig[F](cmd, field)
		input   map[string]bit.Array
		outputs map[string]bit.Array
		// extract checkpointing config
		checkpointing = GetString(cmd, "checkpointing")
	)
	// Sanity permitted flag combinations
	checkFlags(cmd, executeFlags)
	//
	execConfig := vm.DEFAULT_EXECUTION_CONFIG.
		WithGoGen(build.gogen)
	// Including checkpointing configuration (if this makes sense).
	if checkpointing != "" {
		execConfig = execConfig.WithCheckpointing(parseShardingConfig(checkpointing))
	}
	// Build artifacts (compiles source files or loads a prebuilt binary).
	_, binfile := Build[F, W](build, args[1:]...)
	// =====================================================
	// Trace / Execute
	// =====================================================
	// Parse an filter input file
	input = filterInputs(binfile.RawProgram(), ParseInputFile(args[0]))
	// decide what is happening
	outputs, errors = binfile.Execute(input, execConfig)
	// =====================================================
	// Generate output
	// =====================================================
	// Write outputs
	for name, bits := range outputs {
		fmt.Printf("%s = 0x%s\n", name, bits.String())
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

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	executeCmd.Flags().String("checkpointing", "", "specify checkpointing strategy")
	rootCmd.AddCommand(executeCmd)
}
