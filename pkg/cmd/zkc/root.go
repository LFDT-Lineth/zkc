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

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
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
				fmt.Printf("\"%s\"", Version)
			} else if info, ok := debug.ReadBuildInfo(); ok {
				// Built via "go install"
				fmt.Printf("\"%s\"", info.Main.Version)
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
	f := startCpuProfiling(cmd)
	// run field agnostic command
	c.Function(cmd, args, *config)
	// Stop cpu profiling (if was requested)
	stopCpuProfiling(cmd, f)
	// Write memory profiling (if requested)
	writeMemProfile(cmd)
}

// GetBuildConfig constructs a build configuration from the provided
// command-line arguments.  The purpose of this is to provide a consistent
// mechanism for compiling constraint files across the various sub-commands.
func GetBuildConfig[F field.Element[F]](cmd *cobra.Command, field field.Config) BuildConfig {
	var (
		build                 BuildConfig
		fastMode              = GetFlag(cmd, "fast")
		verbosity             = GetVerboseLevel(cmd)
		padding               = GetString(cmd, "padding")
		strategy, strategy_ok = ir.GetPaddingStrategy(padding)
	)
	//
	if !strategy_ok {
		fmt.Printf("padding strategy %s unsupported\n", padding)
		os.Exit(2)
	}
	// Configure log level.  DEBUG (and above) raises logrus to its debug level
	// so the machine execution steps (logged via PerfStats.Log) are surfaced.
	if verbosity >= VERBOSE_DEBUG {
		log.SetLevel(log.DebugLevel)
	}
	// Configure padding strategy
	build.padding = strategy
	// Configure go generator
	build.gogen = GetFlag(cmd, "gogen")
	// Configure compiler config
	build.config = codegen.DEFAULT_CONFIG.
		Inlining(GetFlag(cmd, "inline")).
		FastMode(fastMode).
		MaxStaticDepth(GetUint(cmd, "max-static-depth")).
		Field(field).
		Verbose(verbosity >= VERBOSE_PRINTF)
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
	rootCmd.Flags().Bool("version", false, "Report version of this executable")
	//
	rootCmd.PersistentFlags().Bool("show-static", false, "Show static tables in the MIR/AIR output")
	rootCmd.PersistentFlags().BoolP("fast", "f", false, "Fast-mode execution (no tracing, no constraints)")
	rootCmd.PersistentFlags().CountP("verbose", "v",
		"verbosity: default INFO; -v (DEBUG) shows machine execution steps, -vv (PRINTF) additionally shows all printf output")
	rootCmd.PersistentFlags().Bool("inline", true, "Apply inlining of #[inline] functions")
	rootCmd.PersistentFlags().Bool("vectorize", true, "Apply instruction vectorization")
	rootCmd.PersistentFlags().BoolP("gogen", "g", false, "enable Go code generation")
	rootCmd.PersistentFlags().Uint("max-static-depth", codegen.DEFAULT_MAX_STATIC_DEPTH,
		"maximum depth (number of rows) of static tables")
	rootCmd.PersistentFlags().String("field", "KOALABEAR_16", "prime field to use throughout")
	rootCmd.PersistentFlags().String("padding", "next-power-of-two", "padding strategy to use (e.g. single-row)")
	// profiling commands'
	rootCmd.PersistentFlags().String("cpuprof", "", "write cpu profile to `file`")
	rootCmd.PersistentFlags().String("memprof", "", "write memory profile to `file`")
}
