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
package source

import (
	"fmt"
)

// Span represents a contiguous slice of the original string.  Instead of
// representing this as a string slice, however, it is useful to retain the
// physical indices.  This allows us to do certain things, such as determine the
// enclosing line, etc.
type Span struct {
	// The first character of this span in the original string.
	start int
	// One past the final character of this span in the original string.
	end int
}

// NewSpan constructs a new span whilst checking the internal invariants are
// maintained.
func NewSpan(start int, end int) Span {
	if start > end {
		panic("invalid span")
	}

	return Span{start, end}
}

// Start returns the starting index of this span in the original string.
func (p *Span) Start() int {
	return p.start
}

// End returns one past the last index of this span in the original string.
func (p *Span) End() int {
	return p.end
}

// Length returns the number of characters covered by this span in the original
// string.
func (p *Span) Length() int {
	return p.end - p.start
}

// Maps provides a mechanism for mapping terms from an AST to multiple
// source files.
type Maps[T comparable] struct {
	// Arrray of known source maps.
	maps []Map[T]
}

// NewSourceMaps constructs an (initially empty) set of source maps.  The
// intention is that this is populated as each file is parsed.
func NewSourceMaps[T comparable]() *Maps[T] {
	return &Maps[T]{[]Map[T]{}}
}

// Has checks whether a given node has a mapping in one of the source maps
// embodied within.
func (p *Maps[T]) Has(node T) bool {
	for _, m := range p.maps {
		if m.Has(node) {
			return true
		}
	}
	//
	return false
}

// Lookup returns the source file and span for a given node. If the node is
// found in any of the contained source maps, it returns the corresponding File
// and Span with ok=true; otherwise it returns zero values with ok=false.
func (p *Maps[T]) Lookup(node T) (File, Span, bool) {
	for i := range p.maps {
		if p.maps[i].Has(node) {
			return p.maps[i].Source(), p.maps[i].Get(node), true
		}
	}
	//
	return File{}, Span{}, false
}

// SyntaxError constructs a syntax error for a given node contained within one
// of the source files managed by this set of source maps.
//
//nolint:revive
func (p *Maps[T]) SyntaxError(node T, msg string) *SyntaxError {
	for _, m := range p.maps {
		if m.Has(node) {
			span := m.Get(node)
			return m.srcfile.SyntaxError(span, msg)
		}
	}
	// If we get here, then it means the node on which the error occurrs is not
	// present in any of the source maps.  This should not be possible, provided
	// the parser is implemented correctly.
	panic(fmt.Sprintf("missing mapping for source node (error was %s)", msg))
}

// SyntaxErrors is really just a helper that construct a syntax error and then
// places it into an array of size one.  This is helpful for situations where
// sets of syntax errors are being passed around.
func (p *Maps[T]) SyntaxErrors(node T, msg string) []SyntaxError {
	err := p.SyntaxError(node, msg)
	return []SyntaxError{*err}
}

// Join a given source map into this set of source maps.  The effect of this is
// that nodes recorded in the given source map can be accessed from this set.
func (p *Maps[T]) Join(srcmap *Map[T]) {
	p.maps = append(p.maps, *srcmap)
}

// Copy copies the source mapping for one node to the source mapping for
// another.  The main use of this is when an existing node is expanded into some
// other nodes (e.g. during preprocessing).
func (p *Maps[T]) Copy(from T, to T) {
	for _, m := range p.maps {
		if m.Has(from) {
			span := m.Get(from)
			m.Put(to, span)
			// Done
			return
		}
	}
}

// Map maps terms from an AST to slices of their originating string.  This
// is important for error handling when we wish to highlight exactly where, in
// the original source file, a given error has arisen.
//
// This provides various useful functions to aid reporting syntax errors, such
// as identifying the enclosing line for a given span, etc.
type Map[T comparable] struct {
	// Maps a given AST object to a span in the original string.
	mapping map[T]Span
	// Enclosing source file
	srcfile File
}

// NewSourceMap constructs an initially empty source map for a given string.
func NewSourceMap[T comparable](srcfile File) *Map[T] {
	mapping := make(map[T]Span)
	return &Map[T]{mapping, srcfile}
}

// Source returns the underlying source file on which this map operates.
func (p *Map[T]) Source() File {
	return p.srcfile
}

// Put registers a new AST item with a given span.  Note, if the item exists
// already, then it will panic.
func (p *Map[T]) Put(item T, span Span) {
	if _, ok := p.mapping[item]; ok {
		panic(fmt.Sprintf("source map key already exists: %s", any(item)))
	}
	// Assign it
	p.mapping[item] = span
}

// Has checks whether a given item is contained within this source map.
func (p *Map[T]) Has(item T) bool {
	_, ok := p.mapping[item]
	return ok
}

// Get determines the span associated with a given AST item extract from the
// original text.  Note, if the item is not registered with this source map,
// then it will panic.
func (p *Map[T]) Get(item T) Span {
	if s, ok := p.mapping[item]; ok {
		return s
	}

	panic(fmt.Sprintf("invalid source map key: %s", any(item)))
}

// SyntaxError constructs a syntax error for a given node contained within the
// source file associated with this source map.
//
//nolint:revive
func (p *Map[T]) SyntaxError(item T, msg string) *SyntaxError {
	span := p.Get(item)
	return &SyntaxError{&p.srcfile, span, msg}
}

// SyntaxErrors is really just a helper that construct a syntax error and then
// places it into an array of size one.  This is helpful for situations where
// sets of syntax errors are being passed around.
func (p *Map[T]) SyntaxErrors(item T, msg string) []SyntaxError {
	err := p.SyntaxError(item, msg)
	return []SyntaxError{*err}
}

// JoinMaps incorporates all mappings from one source map (the source) into
// another source map (the target), whilst applying a given mapping to the node
// types.
func JoinMaps[S comparable, T comparable](target *Map[S], source *Map[T], mapping func(T) S) {
	for i, k := range source.mapping {
		target.Put(mapping(i), k)
	}
}
