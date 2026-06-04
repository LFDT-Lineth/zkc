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

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset/verify/picus"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/spf13/cobra"
)

var genVerifyCmd = &cobra.Command{
	Use:   "verify [flags]",
	Short: "Verify constraints using [tool] verifier.",
	Long:  `Extract constraints into a form required by target verification tool`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, verifyCmd)
	},
}

// Available instances
var verifyCmd = []FieldAgnosticCmd{
	{field.GF_251, runVerifyCmd[gf251.Element]},
	{field.GF_8209, runVerifyCmd[gf8209.Element]},
	{field.KOALABEAR_16, runVerifyCmd[koalabear.Element]},
	{field.BLS12_377, runVerifyCmd[bls12_377.Element]},
}

// The `verify` command takes as input a constraint file and translates the constraints
// into a constraint verification backend. The current translator supports translating AIR and
// MIR schemas.
func runVerifyCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	mirEnable := GetFlag(cmd, "mir")

	backend := GetString(cmd, "tool")
	if backend != "picus" {
		fmt.Fprintf(os.Stderr, "Only picus backend supported")
		os.Exit(1)
	}
	// Construct schema stack
	stack := getSchemaStack[F](cmd, SCHEMA_DEFAULT_MIR, args...).Build()
	// Identify concrete (i.e. lowest) schema
	schema := stack.ConcreteSchema()
	//
	switch v := schema.(type) {
	case mir.Schema[F]:
		// only translate mir schema if explicitly specified
		if mirEnable {
			picusLowering := picus.NewMirPicusTranslator(v)
			picusProgram := picusLowering.Translate()

			if _, err := picusProgram.WriteTo(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing out Picus program: %v", err)
				os.Exit(1)
			}
		}
	case air.Schema[F]:
		picusLowering := picus.NewAirPicusTranslator(v)
		picusProgram := picusLowering.Translate()

		if _, err := picusProgram.WriteTo(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing out Picus program: %v", err)
			os.Exit(1)
		}
	}
}

//nolint:errcheck
func init() {
	rootCmd.AddCommand(genVerifyCmd)
	genVerifyCmd.Flags().StringP("tool", "t", "picus", "specify output file(s).")
}
