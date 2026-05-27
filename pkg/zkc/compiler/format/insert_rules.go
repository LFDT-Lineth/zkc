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
package format

import (
	"math"

	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
)

var (
	// DEFAULT_INSERTION_RULES contains the set of default whitespace insertion rules to use.
	DEFAULT_INSERTION_RULES []InsertionRule
	// ONE_SPACE is a fixed token representing a single space
	ONE_SPACE = lex.Token{Kind: parser.SPACES, Span: source.NewSpan(0, 1)}
	// ONE_NEWLINE is a fixed token representing a single newline
	ONE_NEWLINE = lex.Token{Kind: parser.NEWLINE, Span: source.NewSpan(0, 1)}
)

func init() {
	DEFAULT_INSERTION_RULES = make([]InsertionRule, parser.MAX_TOKEN)
	// Space after keywords that introduce declarations or statements.
	DEFAULT_INSERTION_RULES[parser.KEYWORD_CONST] = InsertAfter(ONE_SPACE)
	// 'fail' optionally takes a formatted message; only insert a space when one
	// is actually present, so a bare `fail` does not gain a trailing space.
	DEFAULT_INSERTION_RULES[parser.KEYWORD_FAIL] = InsertSpaceAfterIfString()
	DEFAULT_INSERTION_RULES[parser.KEYWORD_FN] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_FOR] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_IF] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_SWITCH] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_CASE] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_INCLUDE] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_INPUT] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_MEMORY] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_OUTPUT] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_PRINTF] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_PUB] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_STATIC] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_TYPE] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_VAR] = InsertAfter(ONE_SPACE)
	DEFAULT_INSERTION_RULES[parser.KEYWORD_WHILE] = InsertAfter(ONE_SPACE)
	// Space around 'as' (used in cast expressions, e.g. "x as u8").
	DEFAULT_INSERTION_RULES[parser.KEYWORD_AS] = InsertSpaceAround()
	// Space around 'else'.
	DEFAULT_INSERTION_RULES[parser.KEYWORD_ELSE] = InsertSpaceAround()
	// Space before and after the return-type arrow.
	DEFAULT_INSERTION_RULES[parser.RIGHTARROW] = InsertSpaceAround()
	// Space before and after assignment and comparison operators.  Note: '<' and
	// '>' are excluded because they also delimit generic type parameters (e.g.
	// fn f<T>()) and cannot be distinguished from comparisons at the token level.
	DEFAULT_INSERTION_RULES[parser.EQUALS] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.EQUALS_EQUALS] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.NOT_EQUALS] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.LESS_THAN_EQUALS] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.GREATER_THAN_EQUALS] = InsertSpaceAround()
	// Space before and after logical operators.
	DEFAULT_INSERTION_RULES[parser.LOGICAL_AND] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.LOGICAL_OR] = InsertSpaceAround()
	// Space before and after arithmetic operators.
	DEFAULT_INSERTION_RULES[parser.ADD] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.SUB] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.MUL] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.DIV] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.REM] = InsertSpaceAround()
	// Space before and after bitwise binary operators.
	DEFAULT_INSERTION_RULES[parser.BITWISE_AND] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.BITWISE_OR] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.BITWISE_XOR] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.BITWISE_SHL] = InsertSpaceAround()
	DEFAULT_INSERTION_RULES[parser.BITWISE_SHR] = InsertSpaceAround()
	// Space after separators.
	DEFAULT_INSERTION_RULES[parser.COMMA] = InsertSpaceAfter()
	DEFAULT_INSERTION_RULES[parser.SEMICOLON] = InsertSemicolonSpace()
	DEFAULT_INSERTION_RULES[parser.QMARK] = InsertSpaceAround()
	// Space before opening braces (unless already spaced), newline+indent after (unless one follows already).
	DEFAULT_INSERTION_RULES[parser.LCURLY] = InsertForOpenCurly(DEFAULT_INDENTATION)
	// Newline+indent before closing braces (unless a newline already precedes it).
	DEFAULT_INSERTION_RULES[parser.RCURLY] = InsertForCloseCurly(DEFAULT_INDENTATION)
	// Space before comments.
	DEFAULT_INSERTION_RULES[parser.COMMENT] = InsertSpaceBefore()
	// Indentation after newlines.
	DEFAULT_INSERTION_RULES[parser.NEWLINE] = InsertIndent(DEFAULT_INDENTATION)
}

// InsertionRule represents a rule for inserting whitespace of some kind (e.g.
// newlines, tabs or spaces).  Observe the spans for inserted token do not
// correspond with the orignal source file.  Rather, for the WHITESPACE token,
// it simply determines how many spaces to insert.
type InsertionRule interface {
	// Before indicates whether or not to insert whitespace before a given token
	// and, if so, what whitespace to insert (i.e. either WHITESPACE or
	// NEWLINE, etc).  The prev iterator yields preceding tokens in reverse order
	// (most recent first), allowing rules to look back as far as needed.
	Before(indent uint, prev iter.Iterator[lex.Token]) []lex.Token
	// After indicates whether or not to insert whitespace after a given token
	// and, if so, what whitespace to insert (i.e. either WHITESPACE or
	// NEWLINE, etc).  The next iterator yields following tokens in forward order
	// (nearest first), allowing rules to look ahead as far as needed.
	After(indent uint, next iter.Iterator[lex.Token]) []lex.Token
}

// ===================================================================
// Constructors
// ===================================================================

// InsertAfter constructs a rule which always inserts the given token
// afterwards.
func InsertAfter(token lex.Token) InsertionRule {
	return &insertAfter{token}
}

// InsertSpaceAfter constructs a rule which inserts a space after the matched
// token, unless the following token is a NEWLINE (avoiding a trailing space on
// the last item of a line, e.g. a comma at the end of a line).
func InsertSpaceAfter() InsertionRule {
	return &insertSpaceAfter{}
}

// InsertSpaceAfterIfString constructs a rule which inserts a space after the
// matched token only when the immediately following token is a STRING.  Used
// for keywords like 'fail' whose string argument is optional, so that a bare
// keyword does not gain a trailing space.
func InsertSpaceAfterIfString() InsertionRule {
	return &insertSpaceAfterIfString{}
}

// InsertSpaceBefore constructs a rule which inserts a space before the matched
// token only when the preceding token's rule did not already emit a trailing
// space. This avoids double-spacing when two rules would otherwise both
// contribute whitespace at the same boundary (e.g. "else {").
func InsertSpaceBefore() InsertionRule {
	return &insertSpaceBefore{}
}

// InsertForOpenCurly constructs a rule for '{' that inserts a space before (unless
// already spaced) and a newline+indent after (unless a newline or comment already follows).
func InsertForOpenCurly(indent uint) InsertionRule {
	return &insertOpenBrace{indent}
}

// InsertForCloseCurly constructs a rule for '}' that inserts a newline+indent before
// it unless the output already ends with a newline (possibly followed by indentation).
func InsertForCloseCurly(indent uint) InsertionRule {
	return &insertCloseBrace{indent}
}

// InsertSpaceAround constructs a rule which inserts a space both before and
// after the matched token, unless the following token is a NEWLINE (avoiding a
// trailing space when the operator appears at the end of a line).
func InsertSpaceAround() InsertionRule {
	return &insertSpaceAround{}
}

// InsertSemicolonSpace constructs a rule for ';' that inserts a space after
// the semicolon in most contexts (e.g. for-loop headers) but suppresses it
// inside square brackets (fixed-size array types like [u8;3]).
func InsertSemicolonSpace() InsertionRule {
	return &insertSemicolonSpace{}
}

// InsertIndent constructs a rule which always inserts the given token
// afterwards.
func InsertIndent(indent uint) InsertionRule {
	return &insertIndent{indent}
}

// ===================================================================
// InsertAfterRule
// ===================================================================

// insert after rule always inserts a fixed token after a give kind.
type insertAfter struct {
	token lex.Token
}

func (p *insertAfter) Before(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

func (p *insertAfter) After(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return []lex.Token{p.token}
}

// ===================================================================
// InsertSpaceAfter
// ===================================================================

type insertSpaceAfter struct{}

func (p *insertSpaceAfter) Before(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

func (p *insertSpaceAfter) After(_ uint, next iter.Iterator[lex.Token]) []lex.Token {
	if next.HasNext() && next.Next().Kind == parser.NEWLINE {
		return nil
	}

	return []lex.Token{ONE_SPACE}
}

// ===================================================================
// InsertSpaceAfterIfString
// ===================================================================

type insertSpaceAfterIfString struct{}

func (p *insertSpaceAfterIfString) Before(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

func (p *insertSpaceAfterIfString) After(_ uint, next iter.Iterator[lex.Token]) []lex.Token {
	if next.HasNext() && next.Next().Kind == parser.STRING {
		return []lex.Token{ONE_SPACE}
	}
	//
	return nil
}

// ===================================================================
// InsertSpaceAround
// ===================================================================

type insertSpaceAround struct{}

func (p *insertSpaceAround) Before(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return []lex.Token{ONE_SPACE}
}

func (p *insertSpaceAround) After(_ uint, next iter.Iterator[lex.Token]) []lex.Token {
	if next.HasNext() && next.Next().Kind == parser.NEWLINE {
		return nil
	}

	return []lex.Token{ONE_SPACE}
}

// ===================================================================
// InsertBeforeIfUnspaced
// ===================================================================

type insertSpaceBefore struct {
}

func (p *insertSpaceBefore) Before(_ uint, prev iter.Iterator[lex.Token]) []lex.Token {
	if prev.HasNext() {
		tok := prev.Next()
		//
		if tok.Kind == parser.SPACES || tok.Kind == parser.TABS || tok.Kind == parser.NEWLINE {
			// Either indentation tokens (comment is at start of line) or a trailing
			// space already emitted by a prior rule — either way no extra space needed.
			return nil
		}

		return []lex.Token{ONE_SPACE}
	}
	// No previous tokens (start of output).
	return nil
}

func (p *insertSpaceBefore) After(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

// ===================================================================
// InsertOpenBrace
// ===================================================================

type insertOpenBrace struct {
	indent uint
}

func (p *insertOpenBrace) Before(_ uint, prev iter.Iterator[lex.Token]) []lex.Token {
	if prev.HasNext() {
		next := prev.Next()
		if next.Kind == parser.SPACES || next.Kind == parser.TABS {
			return nil
		}
	}

	return []lex.Token{ONE_SPACE}
}

func (p *insertOpenBrace) After(level uint, next iter.Iterator[lex.Token]) []lex.Token {
	if next.HasNext() {
		kind := next.Next().Kind
		if kind == parser.NEWLINE || kind == parser.COMMENT {
			return nil
		}
	}

	if level == 0 {
		return []lex.Token{ONE_NEWLINE}
	}

	var indentTok lex.Token
	if p.indent == math.MaxUint {
		indentTok = lex.Token{Kind: parser.TABS, Span: source.NewSpan(0, int(level))}
	} else {
		indentTok = lex.Token{Kind: parser.SPACES, Span: source.NewSpan(0, int(level*p.indent))}
	}

	return []lex.Token{ONE_NEWLINE, indentTok}
}

// ===================================================================
// InsertCloseBrace
// ===================================================================

type insertCloseBrace struct {
	indent uint
}

func (p *insertCloseBrace) Before(level uint, prev iter.Iterator[lex.Token]) []lex.Token {
	// Walk back over any indentation tokens the NEWLINE rule may have emitted,
	// then check whether the token underneath is a NEWLINE.  If so, we are
	// already at the start of a line and no insertion is needed.
	for prev.HasNext() {
		tok := prev.Next()
		if tok.Kind == parser.SPACES || tok.Kind == parser.TABS {
			continue
		}

		if tok.Kind == parser.NEWLINE {
			return nil
		}

		break
	}

	if level == 0 {
		return []lex.Token{ONE_NEWLINE}
	}

	var indentTok lex.Token
	if p.indent == math.MaxUint {
		indentTok = lex.Token{Kind: parser.TABS, Span: source.NewSpan(0, int(level))}
	} else {
		indentTok = lex.Token{Kind: parser.SPACES, Span: source.NewSpan(0, int(level*p.indent))}
	}

	return []lex.Token{ONE_NEWLINE, indentTok}
}

func (p *insertCloseBrace) After(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

// ===================================================================
// InsertIndent
// ===================================================================

type insertIndent struct {
	indent uint
}

func (p *insertIndent) Before(_ uint, _ iter.Iterator[lex.Token]) []lex.Token {
	return nil
}

func (p *insertIndent) After(level uint, next iter.Iterator[lex.Token]) []lex.Token {
	if next.HasNext() {
		kind := next.Next().Kind
		// Don't indent blank lines.
		if kind == parser.NEWLINE {
			return nil
		}
		// A closing brace belongs at the outer indentation level, so reduce by one.
		if kind == parser.RCURLY && level > 0 {
			level--
		}
	}

	if level == 0 {
		return nil
	} else if p.indent == math.MaxUint {
		return []lex.Token{{Kind: parser.TABS, Span: source.NewSpan(0, int(level))}}
	}

	return []lex.Token{{Kind: parser.SPACES, Span: source.NewSpan(0, int(level*p.indent))}}
}

// ===================================================================
// InsertSemicolonSpace
// ===================================================================

type insertSemicolonSpace struct {
	inBrackets bool
}

func (p *insertSemicolonSpace) Before(_ uint, prev iter.Iterator[lex.Token]) []lex.Token {
	depth := 0

	for prev.HasNext() {
		switch prev.Next().Kind {
		case parser.RSQUARE:
			depth++
		case parser.LSQUARE:
			if depth == 0 {
				p.inBrackets = true
				return nil
			}

			depth--
		case parser.NEWLINE, parser.LCURLY, parser.RCURLY:
			p.inBrackets = false
			return nil
		}
	}

	p.inBrackets = false

	return nil
}

func (p *insertSemicolonSpace) After(_ uint, next iter.Iterator[lex.Token]) []lex.Token {
	if p.inBrackets {
		return nil
	}

	if next.HasNext() && next.Next().Kind == parser.NEWLINE {
		return nil
	}

	return []lex.Token{ONE_SPACE}
}
