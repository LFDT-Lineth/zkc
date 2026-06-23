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
package util

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// ErrorCompiler compiles a source file and produces zero or more errors.
type ErrorCompiler func(source.File) []source.SyntaxError

// Check that a given source file fails to compiler.
// nolint
func CheckInvalid(t *testing.T, test, ext, prefix string, compiler ErrorCompiler) {
	var filename = fmt.Sprintf("%s/%s.%s", TestDir, test, ext)
	// Enable testing each trace in parallel
	t.Parallel()
	//
	srcfile := readSourceFile(t, filename)
	// Compile source file to produce errors
	actual := compiler(*srcfile)
	// Extract expected errors for comparison
	expected, errs := ExtractAttributes(srcfile, extractSyntaxError(prefix))
	// For now.
	if len(errs) > 0 {
		// Report any errors encountered parsing the attributes themselves.
		t.Fatal(errors.Join(errs...))
	}
	// Check program did not compile!
	checkExpectedErrors(t, srcfile, actual, expected)
}

func checkExpectedErrors(t *testing.T, srcfile *source.File, actual, expected []source.SyntaxError) {
	if len(actual) == 0 {
		t.Fatalf("Error %s should not have compiled\n", srcfile.Filename())
	} else {
		error := false
		// Construct initial message
		msg := fmt.Sprintf("Error %s\n", srcfile.Filename())
		// Pad out with what received
		for i := 0; i < max(len(actual), len(expected)); i++ {
			if i < len(actual) && i < len(expected) {
				expected := expected[i]
				actual := actual[i]
				// Check whether message OK
				if expected.Message() == actual.Message() && expected.Span() == actual.Span() {
					continue
				}
			}
			// Indicate error arose
			error = true
			// actual
			if i < len(actual) {
				actual := actual[i]
				msg = fmt.Sprintf("%s unexpected error %s\n", msg, errorToString(actual))
			}
			// expected
			if i < len(expected) {
				expected := expected[i]
				msg = fmt.Sprintf("%s   expected error %s\n", msg, errorToString(expected))
			}
		}
		//
		if error {
			t.Fatal(msg)
		}
	}
}

func readSourceFile(t *testing.T, filename string) *source.File {
	// Read constraints file
	bytes, err := os.ReadFile(filename)
	// Check test file read ok
	if err != nil {
		t.Fatal(err)
	}
	// Package up as source file
	return source.NewSourceFile(filename, bytes)
}

// Convert a span into a useful human readable string.
func errorToString(err source.SyntaxError) string {
	span := err.Span()
	line := err.FirstEnclosingLine()
	lineOffset := span.Start() - line.Start()
	// Calculate length (ensures don't overflow line)
	length := min(line.Length()-lineOffset, span.Length())
	// Print error + line number
	return fmt.Sprintf("%s:%d:%d-%d %s\n", err.SourceFile().Filename(),
		line.Number(), 1+lineOffset, 1+lineOffset+length, err.Message())
}
