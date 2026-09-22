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

	"github.com/LFDT-Lineth/zkc/pkg/cmd/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/goldilocks"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var executeCmd = &cobra.Command{
	Use:     "execute [flags] input.json file1.zkc file2.zkc ...",
	Short:   "Execute a zkc program.",
	Long:    `Execute a zkc program to produce a set of outputs a from given a set of inputs.`,
	Aliases: []string{"exec"},
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
	{field.BLS12_377, runExecuteCmd[bls12_377.Element, vm.Uint128]},
}

// Permitted flag combinations
var executeFlags FlagChecks

func runExecuteCmd[F field.Element[F], W vm.Word[W]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		errors  []error
		build   = GetBuildConfig[F](cmd, field)
		input   map[string][]byte
		outputs map[string][]byte
	)
	// Sanity permitted flag combinations
	checkFlags(cmd, executeFlags)
	// Build artifacts (compiles source files or loads a prebuilt binary).
	_, binfile := Build[F, W](build, args[1:]...)
	// =====================================================
	// Trace / Execute
	// =====================================================
	// Parse an filter input file
	input = filterInputs(binfile.RawProgram(), ParseInputFile(args[0]))
	// decide what is happening
	if build.gogen {
		// Execute via native Go generated from the word machine.
		outputs, errors = executeWithGogen(binfile.RawProgram(), input)
	} else {
		outputs, errors = binfile.Execute(input)
	}
	// =====================================================
	// Generate output
	// =====================================================
	// Write outputs
	for name, bytes := range outputs {
		fmt.Printf("%s = 0x%s\n", name, hex.EncodeToString(bytes))
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
	rootCmd.AddCommand(executeCmd)
}

// executeWithGogen executes the word machine by generating native Go, compiling
// it, and running the resulting binary as a subprocess — the same path the
// gogen differential tests take, exposed here as the "--gogen" execution mode.
func executeWithGogen(program vm.Program[vm.Uint], input map[string][]byte) (map[string][]byte, []error) {
	var (
		stats = util.NewPerfStats()
	)
	// Generate native Go source for the word machine.
	src, err := vm.GenerateGo(program, vm.GoGenConfig{})
	if err != nil {
		return nil, []error{err}
	}
	// Compile the generated source to a temporary executable.
	prog, cleanup, err := gogen.Build(src)
	if err != nil {
		return nil, []error{err}
	}
	// Log result
	stats.Log(fmt.Sprintf("compiling binary %s", prog))
	//
	defer cleanup()
	// Run the compiled program, passing only the machine's declared inputs (the
	// generated harness rejects unknown keys).  Forward its debug/printf and
	// fail output to our stderr so those statements are surfaced to the user.
	outputs, errored, err := gogen.Run(prog, input, os.Stderr)
	//
	switch {
	case err != nil:
		return nil, []error{err}
	case errored:
		return nil, []error{fmt.Errorf("execution rejected (trace rejected)")}
	}
	//
	return outputs, nil
}
