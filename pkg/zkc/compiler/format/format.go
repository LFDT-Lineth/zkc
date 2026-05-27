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
	"io"
	"math"
	"slices"

	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
)

// DEFAULT_INDENTATION sets the default level of indentation (in spaces).
const DEFAULT_INDENTATION uint = 4

// Formatter provides a configurable mechanism for formatting source files (e.g.
// where indentation, maximum line length, etc can be specified).  The intention
// is that, having constructed a formatter, it is then configured before finally
// applying Format().
type Formatter struct {
	// out defines the output writer to use.
	out io.Writer
	// The original source file being formatted.
	srcfile *source.File
	// Holds the (optional) parsed source file.  When this is not present,
	// context-specific formatting rules will not be used.
	ast util.Option[parser.UnlinkedSourceFile]
	// rules is the formatter's own copy of the insertion rules, allowing
	// per-instance configuration (e.g. indentation style) without affecting
	// other formatters.
	rules []InsertionRule
	// removalRules is the formatter's own copy of the removal rules.
	removalRules []RemovalRule
}

// IndentWithSpaces configures the formatter to indent using a given number of
// spaces.
func (p *Formatter) IndentWithSpaces(spaces uint) *Formatter {
	p.rules[parser.NEWLINE] = InsertIndent(spaces)
	p.rules[parser.LCURLY] = InsertForOpenCurly(spaces)
	p.rules[parser.RCURLY] = InsertForCloseCurly(spaces)

	return p
}

// IndentWithTabs configures the formatter to indent using tabs, rather than
// spaces.
func (p *Formatter) IndentWithTabs() *Formatter {
	p.rules[parser.NEWLINE] = InsertIndent(math.MaxUint)
	p.rules[parser.LCURLY] = InsertForOpenCurly(math.MaxUint)
	p.rules[parser.RCURLY] = InsertForCloseCurly(math.MaxUint)

	return p
}

// NewFormatter constructs a new formatter.  This begins by attempting to parse
// the source file.  If this fails, then context-dependent formatting rules will
// not be used.
func NewFormatter(out io.Writer, srcfile *source.File) (*Formatter, []source.SyntaxError) {
	var (
		// attempt to parse source file.
		ast, errs = parser.Parse(srcfile)
		//
		src util.Option[parser.UnlinkedSourceFile]
	)
	// check whether succeeded or not
	if len(errs) > 0 {
		// failed, so disable context-dependent rules
		src = util.None[parser.UnlinkedSourceFile]()
	} else {
		// successfully parsed, so enable context-dependent rules
		src = util.Some(ast)
	}
	// Copy the default rules so each formatter has its own independent slice.
	rules := slices.Clone(DEFAULT_INSERTION_RULES)
	removalRules := slices.Clone(DEFAULT_REMOVAL_RULES)
	// Done.
	return &Formatter{out, srcfile, src, rules, removalRules}, errs
}

// Format the source file, pretty printing result to out.  This operates by
// directly manipulating the token set to remove / insert whitespace and
// newlines.  Specifically, it begins by removing all whitespace except newlines
// and comments.  Then, it reinserts whitespace to meet the indentation
// requirements according to a given set of rules.  If the original source file
// could not be parsed, then context-dependent rules are disabled.
func (p *Formatter) Format() error {
	var (
		tokens []lex.Token
		writer = NewTokenWriter(p.out, p.srcfile.Contents())
	)
	// Relex source file, this time preserving whitespace and comments.
	tokens = parser.Lex(*p.srcfile, true, true)
	// Remove whitespace
	tokens = p.StripWhiteSpace(tokens)
	// Reinsert whitespace
	tokens = p.ReinsertWhiteSpace(tokens)
	// Write out all tokens as they are
	if err := writer.WriteTokens(tokens...); err != nil {
		return err
	}
	// Flush writer
	if err := writer.Flush(); err != nil {
		return err
	}
	//
	return nil
}

// StripWhiteSpace strips white space from the given set of tokens, whilst
// retaining new lines.
func (p *Formatter) StripWhiteSpace(tokens []lex.Token) []lex.Token {
	// Remove any comments
	return array.RemoveMatching(tokens, func(t lex.Token) bool {
		return t.Kind == parser.WHITESPACE
	})
}

// ReinsertWhiteSpace reinserts whitespace according to a given set of insertion
// rules.
func (p *Formatter) ReinsertWhiteSpace(tokens []lex.Token) (ntokens []lex.Token) {
	var indentation uint
	//
	for i, t := range tokens {
		//
		indentation = updateIndentation(indentation, t)
		// Check whether the token should be removed based on surrounding context.
		if rule := p.removalRules[t.Kind]; rule != nil {
			rprev, rnext := getPrevNextTokens(ntokens, tokens[i:])
			if rule.Before(rprev) || rule.After(rnext) {
				continue
			}
		}
		// Determine the preceding and following tokens, if any.
		var prev, next = getPrevNextTokens(ntokens, tokens[i:])
		//
		if rule := p.rules[t.Kind]; rule != nil {
			ntokens = append(ntokens, rule.Before(indentation, prev)...)
			ntokens = append(ntokens, t)
			ntokens = append(ntokens, rule.After(indentation, next)...)
		} else {
			ntokens = append(ntokens, t)
		}
	}
	//
	return ntokens
}

func getPrevNextTokens(before, after []lex.Token) (iter.Iterator[lex.Token], iter.Iterator[lex.Token]) {
	return iter.NewReverseArrayIterator(before), iter.NewArrayIterator(after[1:])
}

func updateIndentation(indent uint, token lex.Token) uint {
	// update indentation
	switch token.Kind {
	case parser.LCURLY:
		return indent + 1
	case parser.RCURLY:
		// Prevent negative indentation
		if indent > 0 {
			return indent - 1
		}
	}
	//
	return indent
}
