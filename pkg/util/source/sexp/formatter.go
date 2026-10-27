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
	"math"
)

// FormattingChunk represents a chunk of a lisp expression which is to be
// indented at a given priority level.
type FormattingChunk struct {
	Priority uint
	Indent   uint
	Contents SExp
}

// Formatter encapsulates and applies a given set of rules.
type Formatter struct {
	// Maximum desired width
	maxWidth uint
	// Determine whether or not to quote symbols
	quote bool
	// Rules to be used for formatting
	rules []FormattingRule
}

// NewFormatter constructs a new formatter which aims to fit its output within a
// given width.
func NewFormatter(width uint, quote bool) *Formatter {
	return &Formatter{width, quote, nil}
}

// Add a new formatting rule to this formatter.
func (p *Formatter) Add(rule FormattingRule) {
	p.rules = append(p.rules, rule)
}

// Format a given S-Expression using the rules embedded within this formatter.
func (p *Formatter) Format(sexp SExp) string {
	var (
		priority uint = 0
		changed       = true
		text     FormattedText
	)
	// Keep going whilst things are still changing.
	for changed {
		changed = false
		text = format(priority, p.maxWidth, p.quote, sexp, p.rules)
		//
		if w := text.MaxWidth(); w > p.maxWidth && priority < 10 {
			changed = true
			priority++
		}
	}
	//
	return text.String()
}

func format(priority, maxWidth uint, quote bool, sexp SExp, rules []FormattingRule) FormattedText {
	var text FormattedText
	//
	format_inner(priority, maxWidth, quote, false, sexp, rules, &text)
	// Done
	return text
}

func format_inner(priority, maxWidth uint, quote, newline bool, sexp SExp, rules []FormattingRule,
	text *FormattedText) {
	//
	switch sexp := sexp.(type) {
	case *Symbol, *Number:
		text.WriteString(sexp.String(quote))
	case *List:
		for _, rule := range rules {
			// Override priority?
			if text.LineWidth()+uint(len(sexp.String(quote))) <= maxWidth {
				priority = 0
			}
			//
			if chunks, indent := rule.Split(sexp); chunks != nil {
				format_with(priority, maxWidth, quote, newline, chunks, indent, rules, text)
				return
			}
		}
		// default rule
		format_default(priority, maxWidth, quote, sexp, rules, text)
	default:
		panic("unreachable")
	}
}

func format_with(priority, maxWidth uint, quote, newline bool, chunks []FormattingChunk, indent uint,
	rules []FormattingRule, text *FormattedText) {
	//
	if indent != math.MaxUint && !newline {
		text.Indent(int(indent))
		text.NewLine()
	}
	//
	text.WriteString("(")
	//
	for i, chunk := range chunks {
		var nl bool
		//
		if chunk.Priority <= priority {
			text.Indent(int(chunk.Indent))
			text.NewLine()
			// Request newline
			nl = true
		} else if i != 0 {
			text.WriteString(" ")
		}
		//
		format_inner(priority, maxWidth, quote, nl, chunk.Contents, rules, text)
		//
		if chunk.Priority <= priority {
			text.Indent(-int(chunk.Indent))
		}
	}
	//
	text.WriteString(")")
	//
	if indent != math.MaxUint && !newline {
		text.Indent(-int(indent))
	}
}

func format_default(priority, maxWidth uint, quote bool, sexp *List, rules []FormattingRule, text *FormattedText) {
	//
	text.WriteString("(")
	//
	for i := 0; i < sexp.Len(); i++ {
		if i != 0 {
			text.WriteString(" ")
		}

		format_inner(priority, maxWidth, quote, false, sexp.Get(i), rules, text)
	}
	//
	text.WriteString(")")
}
