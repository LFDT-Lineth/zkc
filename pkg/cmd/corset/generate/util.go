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
package generate

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Get a suitable string representing a Java type which safely contains all
// values of the given bitwidth.
func getJavaType(bitwidth uint) string {
	switch {
	case bitwidth == 1:
		return "boolean"
	case bitwidth <= 63:
		return "long"
	default:
		return "Bytes"
	}
}

func normaliseBitwidth(bitwidth uint) uint {
	switch {
	case bitwidth == 1:
		return 1
	case bitwidth <= 63:
		return 63
	default:
		return math.MaxUint
	}
}

func toRegisterName(register register.Ref, name string) string {
	mid := register.Module()
	rid := register.Register().Unwrap()
	//
	return fmt.Sprintf("r%d_%d_%s", mid, rid, toCamelCase(name))
}

// Capitalise each word
func toPascalCase(name string) string {
	return camelify(name, true)
}

// Capitalise each word, except first.
func toCamelCase(name string) string {
	var word string
	//
	for i, w := range splitWords(name) {
		//
		w = replaceSymbols(w)
		//
		if i == 0 {
			word = camelify(w, false)
		} else {
			word = fmt.Sprintf("%s%s", word, camelify(w, true))
		}
	}
	//
	return word
}

// Replace symbols which are permitted in column names, but not in Java
// identifiers.
func replaceSymbols(name string) string {
	return strings.ReplaceAll(name, "'", "_")
}

// Make all letters lowercase, and optionally capitalise the first letter.
func camelify(name string, first bool) string {
	letters := strings.Split(name, "")
	for i := range letters {
		if first && i == 0 {
			letters[i] = strings.ToUpper(letters[i])
		} else {
			letters[i] = strings.ToLower(letters[i])
		}
	}
	//
	return strings.Join(letters, "")
}

func splitWords(name string) []string {
	var (
		words []string
	)
	//
	for _, w1 := range strings.Split(name, "_") {
		for _, w2 := range strings.Split(w1, "-") {
			words = append(words, splitCaseChange(w2)...)
		}
	}
	//
	return words
}

func splitCaseChange(word string) []string {
	var (
		runes = []rune(word)
		words []string
		last  bool = true
		start int
	)
	//
	for i, r := range runes {
		ith := unicode.IsUpper(r)
		if !last && ith {
			// case change
			words = append(words, string(runes[start:i]))
			start = i
		}

		last = ith
	}
	// Append whatever is left
	words = append(words, string(runes[start:]))
	//
	return words
}

// A string builder which supports indentation.
type indentBuilder struct {
	indent  uint
	builder *strings.Builder
}

func (p *indentBuilder) Indent() indentBuilder {
	return indentBuilder{p.indent + 1, p.builder}
}

func (p *indentBuilder) WriteString(raw string) {
	p.builder.WriteString(raw)
}

func (p *indentBuilder) WriteIndentedString(pieces ...string) {
	p.WriteIndent()
	//
	for _, s := range pieces {
		p.builder.WriteString(s)
	}
}

func (p *indentBuilder) WriteIndent() {
	for i := uint(0); i < p.indent; i++ {
		p.builder.WriteString("   ")
	}
}
