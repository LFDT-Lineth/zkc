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
	"bytes"
	"fmt"
	"os"

	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/format"
	"github.com/spf13/cobra"
)

var formatCmd = &cobra.Command{
	Use:     "format [flags] file1.zkc file2.zkc ...",
	Aliases: []string{"fmt"},
	Short:   "Format zkc source files.",
	Long:    `Format (pretty-print) a given set of zkc source file(s) in a canonical style.`,
	Run: func(cmd *cobra.Command, args []string) {
		check := GetFlag(cmd, "check")
		tabs := GetFlag(cmd, "tabs")
		spaces := GetUint(cmd, "spaces")
		different := false

		for _, filename := range args {
			if runFormatFile(filename, check, tabs, spaces) {
				different = true
			}
		}

		if check && different {
			os.Exit(1)
		}
	},
}

// runFormatFile formats a single file, returning true if the file differs from
// the formatted output (relevant only when check=true).
func runFormatFile(filename string, check bool, tabs bool, spaces uint) bool {
	// Attempt to read file
	text, err := os.ReadFile(filename)
	// Report error as necessary
	if err != nil {
		fmt.Println(err)
		os.Exit(3)
	}
	//
	var (
		// temporary buffer for writing output
		buf bytes.Buffer
		// source file representation
		src = source.NewSourceFile(filename, text)
		// construct default formatter
		formatter, errs = format.NewFormatter(&buf, src)
	)

	if len(errs) > 0 {
		for _, e := range errs {
			printSyntaxError(&e)
		}

		os.Exit(4)
	}
	// Apply indentation style: --tabs takes priority over --spaces.
	if tabs {
		formatter.IndentWithTabs()
	} else if spaces > 0 {
		formatter.IndentWithSpaces(spaces)
	}
	// Run the formatter (finally)
	if err := formatter.Format(); err != nil {
		fmt.Println(err)
		os.Exit(3)
	}
	// Extract formatted bytes
	formatted := buf.Bytes()
	// Apply check (if requested)
	if check {
		if !bytes.Equal(text, formatted) {
			fmt.Fprintf(os.Stderr, "%s: incorrectly formatted\n", filename)

			return true
		}

		return false
	}
	// Write out formatted file
	if err := os.WriteFile(filename, formatted, 0600); err != nil {
		fmt.Println(err)
		os.Exit(3)
	}

	return false
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(formatCmd)
	formatCmd.Flags().Bool("check", false, "report files that differ without rewriting")
	formatCmd.Flags().Bool("tabs", false, "indent using tabs instead of spaces")
	formatCmd.Flags().Uint("spaces", format.DEFAULT_INDENTATION, "number of spaces per indentation level")
}
