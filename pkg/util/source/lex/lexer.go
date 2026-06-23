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
package lex

import "github.com/LFDT-Lineth/zkc/pkg/util/source"

// Token associates a piece of information with a given range of characters in
// the string being scanned.
type Token struct {
	Kind uint
	Span source.Span
}

// LexRule is simply a rule for associating groups of characters with a given
// tag.
//
// nolint
type LexRule[T any] struct {
	scanner Scanner[T]
	tag     uint
}

// Rule constructs a new lexing rule which maps matching characters to a given
// tag.
func Rule[T any](scanner Scanner[T], tag uint) LexRule[T] {
	return LexRule[T]{scanner, tag}
}

// Lexer provides a top-level construct for tokenising a given input string.
type Lexer[T any] struct {
	items  []T
	index  int
	rules  []LexRule[T]
	buffer []Token
}

// NewLexer constructs a new lexer with a given set of lexing rules.
func NewLexer[T any](input []T, rules ...LexRule[T]) *Lexer[T] {
	return &Lexer[T]{
		input,
		0,
		rules,
		nil,
	}
}

// Index returns the current index within the items array.
func (p *Lexer[T]) Index() uint {
	return uint(p.index)
}

// Remaining determines how many characters from the original sequence were
// left.
func (p *Lexer[T]) Remaining() uint {
	return uint(max(0, len(p.items)-p.index))
}

// HasNext checks whether or not there are any items remaining to visit.
func (p *Lexer[T]) HasNext() bool {
	p.scan()
	return len(p.buffer) > 0
}

// Next returns the next item and advances the lexer.
func (p *Lexer[T]) Next() Token {
	next := p.buffer[0]
	p.buffer = p.buffer[1:]
	//
	if p.index == len(p.items) {
		// EOF condition
		p.index++
	} else {
		p.index = next.Span.End()
	}
	//
	return next
}

// Collect is a convenience function which parses all remaining tokens in one
// go, producing an array of tokens.
func (p *Lexer[T]) Collect() []Token {
	var tokens []Token
	// Keep scanning
	for p.HasNext() {
		tokens = append(tokens, p.Next())
	}
	//
	return tokens
}

// internal scan functions.
func (p *Lexer[T]) scan() {
	if len(p.buffer) == 0 && p.index <= len(p.items) {
		// Look for item
		for _, r := range p.rules {
			if n := r.scanner(p.items[p.index:]); n > 0 {
				end := min(len(p.items), p.index+int(n))
				span := source.NewSpan(p.index, end)
				// Insert into buffer
				p.buffer = append(p.buffer, Token{r.tag, span})
				// Done
				return
			}
		}
	}
}
