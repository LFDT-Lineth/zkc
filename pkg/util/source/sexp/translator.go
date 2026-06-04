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
package sexp

import (
	"fmt"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// SymbolRule is a symbol generator is responsible for converting a terminating
// expression (i.e. a symbol) into an expression type T.  For
// example, a number or a column access.
type SymbolRule[T comparable] func(string) (T, bool, error)

// ListRule is a list translator is responsible converting a list with a given
// sequence of zero or more arguments into an expression type T.
// Observe that the arguments are already translated into the correct
// form.
type ListRule[T comparable] func(*List) (T, []source.SyntaxError)

// ArrayRule is an array translator which is responsible converting an array
// with a given sequence of zero or more arguments into an expression type T.
// Observe that the arguments are already translated into the correct form.
type ArrayRule[T comparable] func(*Array) (T, []source.SyntaxError)

// BinaryRule is a binary translator is a wrapper for translating lists which must
// have exactly two symbol arguments.  The wrapper takes care of
// ensuring sufficient arguments are given, etc.
type BinaryRule[T comparable] func(string, string) (T, error)

// RecursiveRule is a recursive translator is a wrapper for translating lists whose
// elements can be built by recursively reusing the enclosing
// translator.
type RecursiveRule[T comparable] func(string, []T) (T, error)

// ===================================================================
// Parser
// ===================================================================

// Translator is a generic mechanism for translating S-Expressions into a structured
// form.
type Translator[T comparable] struct {
	srcfile *source.File
	// Rules for parsing lists
	lists map[string]ListRule[T]
	// Fallback rule for generic user-defined lists.
	list_default ListRule[T]
	// Rules for parsing arrays
	arrays map[string]ArrayRule[T]
	// Fallback rule for generic user-defined arrays.
	array_default ArrayRule[T]
	// Rules for parsing symbols
	symbols []SymbolRule[T]
	// Maps S-Expressions to their spans in the original source file.  This is
	// used to build the new source map.
	old_srcmap *source.Map[SExp]
	// Maps translated expressions to their spans in the original source file.
	// This is constructed using the old source map.
	new_srcmap *source.Map[T]
}

// NewTranslator constructs a new Translator instance.
func NewTranslator[T comparable](srcfile *source.File, srcmap *source.Map[SExp]) *Translator[T] {
	return &Translator[T]{
		srcfile:       srcfile,
		lists:         make(map[string]ListRule[T]),
		list_default:  nil,
		array_default: nil,
		symbols:       make([]SymbolRule[T], 0),
		old_srcmap:    srcmap,
		new_srcmap:    source.NewSourceMap[T](srcmap.Source()),
	}
}

// SourceMap returns the source map maintained for terms constructed by this
// translator.
func (p *Translator[T]) SourceMap() *source.Map[T] {
	return p.new_srcmap
}

// SpanOf gets the span associated with a given S-Expression in the original
// source file.
func (p *Translator[T]) SpanOf(sexp SExp) source.Span {
	return p.old_srcmap.Get(sexp)
}

// Translate a given string into a given structured representation T
// using an appropriately configured.
func (p *Translator[T]) Translate(sexp SExp) (T, []source.SyntaxError) {
	// Process S-expression into target expression
	return translateSExp(p, sexp)
}

// AddListRule adds a raw list rule to this expression translator.
func (p *Translator[T]) AddListRule(name string, rule ListRule[T]) {
	// Construct a recursive list translator as a wrapper around a generic list translator.
	p.lists[name] = rule
}

// AddRecursiveListRule adds a new list translator to this expression translator.
func (p *Translator[T]) AddRecursiveListRule(name string, t RecursiveRule[T]) {
	// Construct a recursive list translator as a wrapper around a generic list translator.
	p.lists[name] = p.createRecursiveListRule(t)
}

// AddDefaultListRule adds a default rule to be applied when no other recursive
// rules apply.
func (p *Translator[T]) AddDefaultListRule(rule ListRule[T]) {
	p.list_default = rule
}

// AddDefaultRecursiveArrayRule adds a default recursive rule to be applied when no
// other recursive rules apply.
func (p *Translator[T]) AddDefaultRecursiveArrayRule(t RecursiveRule[T]) {
	// Construct a recursive list translator as a wrapper around a generic list translator.
	p.array_default = p.createRecursiveArrayRule(t)
}

func (p *Translator[T]) createRecursiveListRule(t RecursiveRule[T]) ListRule[T] {
	// Construct a recursive list translator as a wrapper around a generic list translator.
	return func(l *List) (T, []source.SyntaxError) {
		var (
			empty  T
			errors []source.SyntaxError
		)
		// Extract the "head" of the list.
		if len(l.Elements) == 0 || l.Elements[0].AsSymbol() == nil {
			return empty, p.SyntaxErrors(l, "invalid list")
		}
		// Extract expression name
		head := (l.Elements[0].(*Symbol)).Value
		// Translate arguments
		args := make([]T, len(l.Elements)-1)
		//
		for i, s := range l.Elements[1:] {
			var errs []source.SyntaxError
			//
			args[i], errs = translateSExp(p, s)
			errors = append(errors, errs...)
		}
		// Apply constructor
		term, err := t(head, args)
		// Check error
		if err != nil {
			errors = append(errors, *p.SyntaxError(l, err.Error()))
		}
		// Check for error
		if len(errors) == 0 {
			return term, nil
		}
		// Error case
		return empty, errors
	}
}

func (p *Translator[T]) createRecursiveArrayRule(t RecursiveRule[T]) ArrayRule[T] {
	// Construct a recursive list translator as a wrapper around a generic list translator.
	return func(l *Array) (T, []source.SyntaxError) {
		var (
			empty  T
			errors []source.SyntaxError
		)
		// Extract the "head" of the list.
		if len(l.Elements) == 0 || l.Elements[0].AsSymbol() == nil {
			return empty, p.SyntaxErrors(l, "invalid array")
		}
		// Extract expression name
		head := (l.Elements[0].(*Symbol)).Value
		// Translate arguments
		args := make([]T, len(l.Elements)-1)
		//
		for i, s := range l.Elements[1:] {
			var errs []source.SyntaxError
			//
			args[i], errs = translateSExp(p, s)
			errors = append(errors, errs...)
		}
		// Apply constructor
		term, err := t(head, args)
		// Check error
		if err != nil {
			errors = append(errors, *p.SyntaxError(l, err.Error()))
		}
		// Check for error
		if len(errors) == 0 {
			return term, nil
		}
		// Error case
		return empty, errors
	}
}

// AddBinaryRule .
func (p *Translator[T]) AddBinaryRule(name string, t BinaryRule[T]) {
	var empty T
	//
	p.lists[name] = func(l *List) (T, []source.SyntaxError) {
		if len(l.Elements) != 3 {
			// Should be unreachable.
			return empty, p.SyntaxErrors(l, "Incorrect number of arguments")
		}

		lhs, ok1 := any(l.Elements[1]).(*Symbol)
		rhs, ok2 := any(l.Elements[2]).(*Symbol)

		var msg string

		if ok1 && ok2 {
			term, err := t(lhs.Value, rhs.Value)
			if err == nil {
				return term, nil
			}
			// Adorn error
			msg = err.Error()
		} else {
			msg = fmt.Sprintf("Binary list malformed (%t,%t)", ok1, ok2)
		}
		// error
		return empty, p.SyntaxErrors(l, msg)
	}
}

// AddSymbolRule adds a new symbol translator to this expression translator.
func (p *Translator[T]) AddSymbolRule(t SymbolRule[T]) {
	p.symbols = append(p.symbols, t)
}

// SyntaxError constructs a suitable syntax error for a given S-Expression.
//
//nolint:revive
func (p *Translator[T]) SyntaxError(s SExp, msg string) *source.SyntaxError {
	// Get span of enclosing list
	span := p.old_srcmap.Get(s)
	// Construct syntax error
	return p.srcfile.SyntaxError(span, msg)
}

// SyntaxErrors constructs a suitable syntax error for a given S-Expression.
//
//nolint:revive
func (p *Translator[T]) SyntaxErrors(s SExp, msg string) []source.SyntaxError {
	return []source.SyntaxError{*p.SyntaxError(s, msg)}
}

// ===================================================================
// Private
// ===================================================================

// Translate an S-Expression into an IR expression.  Observe that
// this can still fail in the event that the given S-Expression does
// not describe a well-formed IR expression.
func translateSExp[T comparable](p *Translator[T], s SExp) (T, []source.SyntaxError) {
	var empty T

	switch e := s.(type) {
	case *List:
		return translateSExpList(p, e)
	case *Array:
		return translateSExpArray(p, e)
	case *Symbol:
		for i := 0; i != len(p.symbols); i++ {
			node, ok, err := (p.symbols[i])(e.Value)
			if ok && err != nil {
				// Transform into syntax error
				return empty, p.SyntaxErrors(s, err.Error())
			} else if ok {
				// Update source map
				map2sexp(p, node, s)
				// Done
				return node, nil
			}
		}
	}
	// This should be unreachable.
	typeof := reflect.TypeOf(s)
	// But, if it is reached ... produce a nice error :)
	return empty, p.SyntaxErrors(s, fmt.Sprintf("invalid s-expression (%s)", typeof))
}

// Translate a list of S-Expressions into a unary, binary or n-ary
// expression of some kind.  This type of expression is determined by
// the first element of the list.  The remaining elements are treated
// as arguments which are first recursively translated.
func translateSExpList[T comparable](p *Translator[T], l *List) (T, []source.SyntaxError) {
	var (
		empty  T
		node   T
		errors []source.SyntaxError
	)
	// Sanity check this list makes sense
	if len(l.Elements) == 0 || l.Elements[0].AsSymbol() == nil {
		return empty, p.SyntaxErrors(l, "invalid list")
	}
	// Extract expression name
	name := (l.Elements[0].(*Symbol)).Value
	// Lookup appropriate translator
	t := p.lists[name]
	// Check whether we found one.
	if t != nil {
		node, errors = (t)(l)
	} else if p.list_default != nil {
		node, err := (p.list_default)(l)
		// Update source mapping
		if err == nil {
			map2sexp(p, node, l)
		}
		// Done
		return node, err
	} else {
		// Default fall back
		return empty, p.SyntaxErrors(l, "unknown list encountered")
	}
	// Map source node
	if len(errors) == 0 {
		// Update source mapping
		map2sexp(p, node, l)
	}
	// Done
	return node, errors
}

// Translate an array of S-Expressions into a unary, binary or n-ary
// expression of some kind.  This type of expression is determined by
// the first element of the list.  The remaining elements are treated
// as arguments which are first recursively translated.
func translateSExpArray[T comparable](p *Translator[T], l *Array) (T, []source.SyntaxError) {
	var (
		empty  T
		node   T
		errors []source.SyntaxError
	)
	// Sanity check this list makes sense
	if len(l.Elements) == 0 || l.Elements[0].AsSymbol() == nil {
		return empty, p.SyntaxErrors(l, "invalid array")
	}
	// Extract expression name
	name := (l.Elements[0].(*Symbol)).Value
	// Lookup appropriate translator
	t := p.arrays[name]
	// Check whether we found one.
	if t != nil {
		node, errors = (t)(l)
	} else if p.array_default != nil {
		node, err := (p.array_default)(l)
		// Update source mapping
		if err == nil {
			map2sexp(p, node, l)
		}
		// Done
		return node, err
	} else {
		// Default fall back
		return empty, p.SyntaxErrors(l, "unknown array encountered")
	}
	// Map source node
	if len(errors) == 0 {
		// Update source mapping
		map2sexp(p, node, l)
	}
	// Done
	return node, errors
}

// Add a mapping from a given item to the S-expression from which it was
// generated.  This updates the underlying source map to reflect this.
func map2sexp[T comparable](p *Translator[T], item T, sexp SExp) {
	// Lookup enclosing span
	span := p.old_srcmap.Get(sexp)
	// Map it the new source map
	p.new_srcmap.Put(item, span)
}
