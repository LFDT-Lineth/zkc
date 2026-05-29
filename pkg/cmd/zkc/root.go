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
	"runtime/debug"

	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Version is filled when building with make, but *not* when installing via "go
// install".
var Version string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "zkc",
	Short: "A compiler for the ZkC language.",
	Long:  "A compiler (and general toolbox) for the ZkC language.",
	Run: func(cmd *cobra.Command, args []string) {
		if GetFlag(cmd, "version") {
			fmt.Print("zkc ")

			if Version != "" {
				// Built via "make"
				fmt.Printf("%s", Version)
			} else if info, ok := debug.ReadBuildInfo(); ok {
				// Built via "go install"
				fmt.Printf("%s", info.Main.Version)
			} else {
				// Unknown, perhaps "go run"
				fmt.Printf("(unknown version)")
			}

			fmt.Println()
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// FieldAgnosticCmd represents a command to be executed for a given field.
type FieldAgnosticCmd struct {
	Field    field.Config
	Function func(*cobra.Command, []string, field.Config)
}

// Run a field agnostic top-level command.
func runFieldAgnosticCmd(cmd *cobra.Command, args []string, cmds []FieldAgnosticCmd) {
	var (
		fieldName = GetString(cmd, "field")
		// Field configuration
		config = field.GetConfig(fieldName)
	)
	// Sanity check
	if config == nil {
		fmt.Printf("unknown field \"%s\"\n", fieldName)
		os.Exit(3)
	}
	// find command to dispatch
	c := findFieldAgnosticCmd(*config, cmds)
	// start CPU profiling (if requested)
	startCpuProfiling(cmd)
	// run field agnostic command
	c.Function(cmd, args, *config)
	// Stop cpu profiling (if was requested)
	stopCpuProfiling(cmd)
	// Write memory profiling (if requested)
	writeMemProfile(cmd)
}

// GetBuildConfig constructs a build configuration from the provided
// command-line arguments.  The purpose of this is to provide a consistent
// mechanism for compiling constraint files across the various sub-commands.
func GetBuildConfig[F field.Element[F]](cmd *cobra.Command, field field.Config) BuildConfig[F] {
	var build BuildConfig[F]

	lowerNative := GetFlag(cmd, "lower-native") || GetFlag(cmd, "mir") || GetFlag(cmd, "air")
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}
	// Configure target field
	build.field = field
	// Configure compiler config
	build.config = codegen.DEFAULT_CONFIG.
		LowerNatives(lowerNative).
		Vectorize(GetFlag(cmd, "vectorize")).
		SplitRegisters(GetFlag(cmd, "split")).
		Field(field)
	// Configure build targets
	build.ast = GetFlag(cmd, "ast")
	build.wir = GetFlag(cmd, "wir")
	build.fir = GetFlag(cmd, "fir")
	build.mir = GetFlag(cmd, "mir")
	build.air = GetFlag(cmd, "air")
	//
	return build
}

func findFieldAgnosticCmd(config field.Config, cmds []FieldAgnosticCmd) (cmd FieldAgnosticCmd) {
	//
	for _, c := range cmds {
		if c.Field == config {
			return c
		}
	}
	//
	fmt.Printf("field %s unsupported\n", config.Name)
	os.Exit(2)
	//
	return cmd
}

func init() {
	rootCmd.PersistentFlags().Bool("ast", false, "Output Abstract Syntax Tree (AST)")
	rootCmd.PersistentFlags().Bool("wir", false, "Output Word-level Intermediate Representation (WIR)")
	rootCmd.PersistentFlags().Bool("fir", false, "Output Field-level Intermediate Representation (FIR)")
	rootCmd.PersistentFlags().Bool("mir", false, "Output Mid-Level Intermediate Representation (MIR)")
	rootCmd.PersistentFlags().Bool("air", false, "Output Arithmetic Intermediate Representation (AIR)")
	rootCmd.PersistentFlags().Bool("lower-native", false, "Lower ZkC native functions into arithmetic instructions")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "increase logging verbosity")
	rootCmd.PersistentFlags().Bool("vectorize", true, "Apply instruction vectorization")
	rootCmd.PersistentFlags().Bool("split", false, "Apply register splitting")
	rootCmd.PersistentFlags().String("field", "KOALABEAR_16", "prime field to use throughout")
	// profiling commands'
	rootCmd.PersistentFlags().String("cpuprof", "", "write cpu profile to `file`")
	rootCmd.PersistentFlags().String("memprof", "", "write memory profile to `file`")
}
