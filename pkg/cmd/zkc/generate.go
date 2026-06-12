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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate [flags] file1.zkc file2.zkc ...",
	Short: "generate native Go source from a zkc program.",
	Long: `Compile a zkc program into native Go source exposing Run(inputs) (outputs, error):
the generated-code alternative to interpreting the word machine ("fast execution mode").
By default the artefact is a package main carrying a JSON stdin/stdout harness, so it
can be built and run standalone; naming any other package via --pkg yields an
importable package suitable for go:generate + ahead-of-time compilation.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, generateCmds)
	},
}

// Available instances
var generateCmds = []FieldAgnosticCmd{
	{field.GF_251, runGenerateCmd[gf251.Element]},
	{field.GF_8209, runGenerateCmd[gf8209.Element]},
	{field.KOALABEAR_16, runGenerateCmd[koalabear.Element]},
	{field.BLS12_377, runGenerateCmd[bls12_377.Element]},
}

func runGenerateCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build  = GetBuildConfig[F](cmd, field)
		output = GetString(cmd, "output")
		pkg    = GetString(cmd, "pkg")
		quiet  = GetFlag(cmd, "quiet")
	)
	// The generator consumes the unsplit machine: it lowers register widths to
	// Go types itself (splitting is a prover-shape concern, not an execution one).
	if GetFlag(cmd, "split") {
		log.Error("generate does not support register splitting")
		os.Exit(2)
	}

	applyGenerateDefaults(&build, quiet)
	// Build the word machine from the source files.
	artifacts := build.Build(args...)
	wm := artifacts.wir.Unwrap()
	//
	src, err := vm.GenerateGo(&wm, vm.GoGenConfig{
		Package: packageName(pkg),
		Source:  sourceProvenance(args),
	})
	if err != nil {
		log.Error(err)
		os.Exit(4)
	}
	//
	if output == "" {
		fmt.Print(src)
	} else if err := os.WriteFile(output, []byte(src), 0o644); err != nil {
		log.Error(err)
		os.Exit(5)
	}
}

func applyGenerateDefaults[F field.Element[F]](build *BuildConfig[F], quiet bool) {
	// Suppress printf debug instructions when quiet mode is enabled.
	build.config = build.config.Quiet(quiet)
	// Generation consumes the word machine.
	build.wir = true
}

// packageName determines the generated package name: the --pkg flag when set,
// otherwise "main" (which carries the standalone JSON harness).  Deriving a
// name from the output filename proved surprising — `-o test.go` yielded an
// unrunnable `package test` — so the importable case is always explicit.
func packageName(pkg string) string {
	if pkg != "" {
		return pkg
	}

	return "main"
}

// sourceProvenance renders the input filenames plus a digest of their contents,
// recorded in the generated header so go:generate workflows can detect a stale
// artefact.
func sourceProvenance(args []string) string {
	h := sha256.New()
	//
	for _, name := range args {
		if data, err := os.ReadFile(name); err == nil {
			h.Write(data)
		}
	}
	//
	return fmt.Sprintf("%s sha256:%s", strings.Join(args, " "), hex.EncodeToString(h.Sum(nil)[:8]))
}

//nolint:errcheck
func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().StringP("output", "o", "", "specify output file for generated Go source (default stdout)")
	generateCmd.Flags().String("pkg", "", "generated package name (default main, which carries the standalone harness)")
	generateCmd.Flags().BoolP("quiet", "q", false, "suppress printf output in the generated program")
}
