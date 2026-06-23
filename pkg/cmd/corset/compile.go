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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var compileCmd = &cobra.Command{
	Use:   "compile [flags] constraint_file(s)",
	Short: "compile constraints into a binary package.",
	Long: `Compile a given set of constraint file(s) into a single binary package which can
	 be subsequently used without requiring a full compilation step.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, compileCmds)
	},
}

// Available instances
var compileCmds = []FieldAgnosticCmd{
	{field.GF_251, runCompileCmd[gf251.Element]},
	{field.GF_8209, runCompileCmd[gf8209.Element]},
	{field.KOALABEAR_16, runCompileCmd[koalabear.Element]},
	{field.BLS12_377, runCompileCmd[bls12_377.Element]},
}

func runCompileCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	// Configure log level
	if GetFlag(cmd, "verbose") {
		log.SetLevel(log.DebugLevel)
	}

	output := GetString(cmd, "output")
	defines := GetStringArray(cmd, "define")
	// Parse constraints
	stacker := getSchemaStack[F](cmd, SCHEMA_DEFAULT_MIR, args...)
	// Sanity check it can be build
	_ = stacker.Build()
	// Extract binfile
	binfile := stacker.BinaryFile()
	// Write metadata
	if err := binfile.Header.SetMetaData(buildMetadata(defines)); err != nil {
		fmt.Printf("error writing metadata: %s\n", err.Error())
		os.Exit(1)
	}
	// Serialise as a gob file.
	WriteBinaryFile(binfile, output)
}

func buildMetadata(items []string) typed.Map {
	metadata := make(map[string]any)
	//
	for _, item := range items {
		split := strings.Split(item, "=")
		if len(split) != 2 {
			fmt.Printf("malformed definition \"%s\"\n", item)
			os.Exit(2)
		}
		//
		metadata[split[0]] = split[1]
	}
	//
	return typed.NewMap(metadata)
}

//nolint:errcheck
func init() {
	rootCmd.AddCommand(compileCmd)
	compileCmd.Flags().StringP("output", "o", "a.bin", "specify output file.")
	compileCmd.Flags().StringArrayP("define", "D", []string{}, "define metadata attribute.")
	compileCmd.MarkFlagRequired("output")
}
