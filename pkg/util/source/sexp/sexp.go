// Copyright Consensys Incorporated
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
package sexp

import (
	"fmt"
	"math/big"
	"unicode"
)

// SExp is an S-Expression is either a List of zero or more S-Expressions, or
// a Symbol.
type SExp interface {
	// AsList checks whether this S-Expression is a list and, if
	// so, returns it.  Otherwise, it returns nil.
	AsList() *List
	// AsSet checks whether this S-Expression is a set and, if
	// so, returns it.  Otherwise, it returns nil.
	AsSet() *Set
	// AsArray checks whether this S-Expression is an array and, if so, returns
	// it.  Otherwise, it returns nil.
	AsArray() *Array
	// AsSymbol checks whether this S-Expression is a symbol and,
	// if so, returns it.  Otherwise, it returns nil.
	AsSymbol() *Symbol
	// String generates a string representation which may (may not) be quoted.
	// Quoting is used to manage symbol names which contain whitespace
	// characters and braces, etc.
	String(quote bool) string
}

// ===================================================================
// List
// ===================================================================

// List represents a list of zero or more S-Expressions.
type List struct {
	Elements []SExp
}

// NOTE: This is used for compile time type checking if the given type
// satisfies the given interface.
var _ SExp = (*List)(nil)

// EmptyList creates an empty list.
func EmptyList() *List {
	return &List{}
}

// NewList creates a new list from a given array of S-Expressions.
func NewList(elements []SExp) *List {
	return &List{elements}
}

// AsArray returns the given array.
func (l *List) AsArray() *Array { return nil }

// AsList returns the given list.
func (l *List) AsList() *List { return l }

// AsSet returns nil for a list.
func (l *List) AsSet() *Set { return nil }

// AsSymbol returns nil for a list.
func (l *List) AsSymbol() *Symbol { return nil }

// Len gets the number of elements in this list.
func (l *List) Len() int { return len(l.Elements) }

// Get the ith element of this list
func (l *List) Get(i int) SExp { return l.Elements[i] }

// Append a new element onto this list.
func (l *List) Append(element SExp) {
	l.Elements = append(l.Elements, element)
}

func (l *List) String(quote bool) string {
	var s = "("

	for i := 0; i < len(l.Elements); i++ {
		if i != 0 {
			s += " "
		}

		s += l.Elements[i].String(quote)
	}

	s += ")"

	return s
}

// MatchSymbols matches a list which starts with at least n symbols, of which the
// first m match the given strings.
func (l *List) MatchSymbols(n int, symbols ...string) bool {
	if len(l.Elements) < n || len(symbols) > n {
		return false
	}

	for i := 0; i < len(symbols); i++ {
		switch ith := l.Elements[i].(type) {
		case *Symbol:
			if ith.Value != symbols[i] {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// ===================================================================
// Set
// ===================================================================

// Set represents a list of zero or more S-Expressions.
type Set struct {
	Elements []SExp
}

// NOTE: This is used for compile time type checking if the given type
// satisfies the given interface.
var _ SExp = (*Set)(nil)

// NewSet creates a new set from a given array of S-Expressions.
func NewSet(elements []SExp) *Set {
	return &Set{elements}
}

// AsArray returns the given array.
func (l *Set) AsArray() *Array { return nil }

// AsList returns nil for a set.
func (l *Set) AsList() *List { return nil }

// AsSet returns the given set.
func (l *Set) AsSet() *Set { return l }

// AsSymbol returns nil for a set.
func (l *Set) AsSymbol() *Symbol { return nil }

// Len gets the number of elements in this set.
func (l *Set) Len() int { return len(l.Elements) }

// Get the ith element of this set
func (l *Set) Get(i int) SExp { return l.Elements[i] }

func (l *Set) String(quote bool) string {
	var s = "{"

	for i := 0; i < len(l.Elements); i++ {
		if i != 0 {
			s += " "
		}

		s += l.Elements[i].String(quote)
	}

	s += "}"

	return s
}

// ===================================================================
// Number
// ===================================================================

// Number represents a terminating number
type Number struct {
	Value big.Int
}

// NOTE: This is used for compile time type checking if the given type
// satisfies the given interface.
var _ SExp = (*Number)(nil)

// NewNumber creates a new number from a given integer.
func NewNumber(value big.Int) *Number {
	return &Number{value}
}

// AsArray returns the given array.
func (s *Number) AsArray() *Array { return nil }

// AsList returns nil for a symbol.
func (s *Number) AsList() *List { return nil }

// AsSet returns nil for a symbol.
func (s *Number) AsSet() *Set { return nil }

// AsSymbol returns the given symbol
func (s *Number) AsSymbol() *Symbol { return nil }

func (s *Number) String(quote bool) string {
	// No quote required
	return s.Value.Text(10)
}

// ===================================================================
// Symbol
// ===================================================================

// Symbol represents a terminating symbol.
type Symbol struct {
	Value string
}

// NOTE: This is used for compile time type checking if the given type
// satisfies the given interface.
var _ SExp = (*Symbol)(nil)

// NewSymbol creates a new symbol from a given string.
func NewSymbol(value string) *Symbol {
	return &Symbol{value}
}

// AsArray returns the given array.
func (s *Symbol) AsArray() *Array { return nil }

// AsList returns nil for a symbol.
func (s *Symbol) AsList() *List { return nil }

// AsSet returns nil for a symbol.
func (s *Symbol) AsSet() *Set { return nil }

// AsSymbol returns the given symbol
func (s *Symbol) AsSymbol() *Symbol { return s }

func (s *Symbol) String(quote bool) string {
	if quote {
		needed := false
		// Check whether suitable symbol
		for i, r := range s.Value {
			if (i == 0 && unicode.IsNumber(r)) || !isSymbolLetter(r) {
				needed = true
				break
			}
		}
		// Quote (if necessary)
		if needed {
			return fmt.Sprintf("\"%s\"", s.Value)
		}
	}
	// No quote required
	return s.Value
}

func isSymbolLetter(r rune) bool {
	return r != '(' && r != ')' && !unicode.IsSpace(r)
}

// ===================================================================
// Array
// ===================================================================

// Array represents a list of zero or more S-Expressions.
type Array struct {
	Elements []SExp
}

// NOTE: This is used for compile time type checking if the given type
// satisfies the given interface.
var _ SExp = (*Array)(nil)

// NewArray creates a new Array from a given array of S-Expressions.
func NewArray(elements []SExp) *Array {
	return &Array{elements}
}

// AsArray returns the given array.
func (a *Array) AsArray() *Array { return a }

// AsList returns nil for a Array.
func (a *Array) AsList() *List { return nil }

// AsSet returns the given Array.
func (a *Array) AsSet() *Set { return nil }

// AsSymbol returns nil for a Array.
func (a *Array) AsSymbol() *Symbol { return nil }

// Len gets the number of elements in this Array.
func (a *Array) Len() int { return len(a.Elements) }

// Get the ith element of this Array
func (a *Array) Get(i int) SExp { return a.Elements[i] }

func (a *Array) String(quote bool) string {
	var s = "["

	for i := 0; i < len(a.Elements); i++ {
		if i != 0 {
			s += " "
		}

		s += a.Elements[i].String(quote)
	}

	s += "]"

	return s
}
