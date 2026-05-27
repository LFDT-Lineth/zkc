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
	"fmt"
	"strconv"
	"strings"

	"github.com/consensys/go-corset/pkg/util/source"
)

// Extract the syntax error from a given line in the source file, or return nil
// if it does not describe an error.
func extractSyntaxError(prefix string) Attribute[source.SyntaxError] {
	return func(lineno int, lines []source.Line, srcfile *source.File) (bool, source.SyntaxError, error) {
		var (
			line     = lines[lineno]
			contents = line.String()
		)
		//
		if strings.HasPrefix(contents, prefix) {
			line, start, end, msg, err := parseExpectedErrorLine(contents, prefix)
			//
			if err == nil {
				span, err := determineFileSpan(line, start, end, lines)
				// Done
				return true, *srcfile.SyntaxError(span, msg), err
			}
			//
			return true, source.SyntaxError{}, err
		}
		// No error
		return false, source.SyntaxError{}, nil
	}
}

func parseExpectedErrorLine(contents, prefix string) (line, start, end int, msg string, err error) {
	var splits = strings.Split(contents, ":")
	//
	if len(splits) < 4 {
		return 0, 0, 0, "",
			fmt.Errorf("malformed expected error \"%s\", should be e.g. \"%s:X:Y-Z:msg\"", contents, prefix)
	}
	// Parse line number
	if line, err = strconv.Atoi(splits[1]); err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid span \"%s:%s\" (%s)", splits[1], splits[2], err.Error())
	} else if line == 0 {
		return 0, 0, 0, "", fmt.Errorf("invalid span \"%s:%s\" (lines numbered from 1)", splits[1], splits[2])
	}
	// Parse split
	if start, end, err = parseExpectedErrorSpan(splits[2]); err != nil {
		return 0, 0, 0, "", err
	}
	//
	msg = strings.Join(splits[3:], ":")
	//
	return line, start, end, msg, nil
}

func parseExpectedErrorSpan(span_str string) (start, end int, err error) {
	var (
		// Split the span
		span_splits = strings.Split(span_str, "-")
	)
	//
	if len(span_splits) != 2 {
		return 0, 0, fmt.Errorf("invalid span \"%s\" (malformed, should be X-Y)", span_str)
	}
	// Parse span start as integer
	if start, err = strconv.Atoi(span_splits[0]); err != nil {
		return 0, 0, fmt.Errorf("invalid span \"%s\" (%s)", span_str, err.Error())
	} else if start == 0 {
		return 0, 0, fmt.Errorf("invalid span \"%s\" (columns numbered from 1)", span_str)
	}
	// Parse span end as integer
	if end, err = strconv.Atoi(span_splits[1]); err != nil {
		return 0, 0, fmt.Errorf("invalid span \"%s\" (%s)", span_str, err.Error())
	}
	//
	return start, end, err
}

// Determine the span that the the given line string and span string corresponds
// to.  We need the line offsets so that the computed span includes the starting
// offset of the relevant line.
func determineFileSpan(lineno, start, end int, lines []source.Line) (source.Span, error) {
	var (
		lineStart  int
		lineLength int
	)
	// Sanity checks
	if len(lines) > 0 && lineno == len(lines)+1 && start == 1 && end == 1 {
		// Special case to handle errors on the imaginary EOF terminator.
		line := lines[lineno-2]
		lineStart = line.Start() + line.Length() + 1
		lineLength = 1
	} else if lineno > len(lines) {
		return source.Span{}, fmt.Errorf("invalid span \"%d:%d-%d\" (non-existent line)", lineno, start, end)
	} else {
		// Normal case
		line := lines[lineno-1]
		lineStart = line.Start()
		lineLength = line.Length()
	}
	// Subtract one from each since column numbering starts from 1.
	start--
	end--
	//
	if start >= lineLength || end > lineLength {
		return source.Span{}, fmt.Errorf("invalid span \"%d:%d-%d\" (overflows to following line)", lineno, start, end)
	}
	// Add line offset
	start += lineStart
	end += lineStart
	// Create span, recalling that span's start from zero whereas column numbers
	// start from 1.
	return source.NewSpan(start, end), nil
}
