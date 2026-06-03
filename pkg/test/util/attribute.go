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
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// Attribute provides a generic mechanism for extract attributes from the
// beginning of a file.
// Parse a given line (assuming it has matched) producing an item and,
// potentially, one or more syntax errors.
type Attribute[T any] func(int, []source.Line, *source.File) (bool, T, error)

// ExtractAttributes extracts any matches attributes at the beginning of a
// source file.
func ExtractAttributes[T any](srcfile *source.File, attributes ...Attribute[T]) ([]T, []error) {
	var (
		// Calculate the character offset of each line
		lines = srcfile.Lines()
		// Now construct items
		items []T
		//
		errors []error
		//
		matched = true
	)
	// scan file line-by-line until no more errors found
	for i := 0; i < len(lines) && matched; i++ {
		matched = false

		for _, attribute := range attributes {
			var (
				item T
				err  error
			)
			//
			matched, item, err = attribute(i, lines, srcfile)
			//
			if err != nil {
				errors = append(errors, err)
			} else if matched {
				items = append(items, item)
			}
		}
	}
	//
	return items, errors
}
