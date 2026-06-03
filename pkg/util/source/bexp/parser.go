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
package bexp

import (
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/lex"
)

// Parse a given input string into logical proposition.  The environment
// determines the set of permitted variable names.
func Parse[T Term[T]](input string, environment func(string) bool) (T, []source.SyntaxError) {
	var (
		empty   T
		srcfile = source.NewSourceFile("expr", []byte(input))
		lexer   = lex.NewLexer[rune](srcfile.Contents(), rules...)
		// Lex as many tokens as possible
		tokens = lexer.Collect()
	)
	// Check whether anything was left (if so this is an error)
	if lexer.Remaining() != 0 {
		start, end := lexer.Index(), lexer.Index()+lexer.Remaining()
		err := srcfile.SyntaxError(source.NewSpan(int(start), int(end)), "unknown text encountered")

		return empty, []source.SyntaxError{*err}
	}
	// Remove any whitespace
	tokens = array.RemoveMatching(tokens, func(t lex.Token) bool { return t.Kind == WHITESPACE })
	//
	parser := &Parser[T]{environment, srcfile, tokens, 0}
	// Parse term
	p, errs := parser.parseTerm()
	// Check all parsed
	if len(errs) == 0 && !parser.Done() {
		return empty, parser.syntaxErrors(parser.lookahead(), "unknown token")
	}
	// All good!
	return p, errs
}

// END_OF signals "end of file"
const END_OF uint = 0

// WHITESPACE signals whitespace
const WHITESPACE uint = 1

// LBRACE signals "left brace"
const LBRACE uint = 2

// RBRACE signals "right brace"
const RBRACE uint = 3

// NUMBER signals an integer number
const NUMBER uint = 4

// IDENTIFIER signals a column variable.
const IDENTIFIER uint = 5

// EQUALS signals an equality
const EQUALS uint = 6

// NOT_EQUALS signals a non-equality
const NOT_EQUALS uint = 7

// LESSTHAN signals a (strict) inequality X < Y
const LESSTHAN uint = 8

// LESSTHAN_EQUALS signals a (non-strict) inequality X <= Y
const LESSTHAN_EQUALS uint = 9

// GREATERTHAN signals a (strict) inequality X > Y
const GREATERTHAN uint = 10

// GREATERTHAN_EQUALS signals a (non-strict) inequality X >= Y
const GREATERTHAN_EQUALS uint = 11

// BOTTOM represents logical falsehood
const BOTTOM uint = 12

// TOP represents logical truth
const TOP uint = 13

// OR represents logical disjunction
const OR uint = 14

// AND represents logical conjunction
const AND uint = 15

// ADD represents integer addition
const ADD uint = 16

// SUB represents integer subtraction
const SUB uint = 17

// MUL represents integer multiplication
const MUL uint = 18

// CONDITIONS captures the set of conditions.
var CONDITIONS = []uint{EQUALS, NOT_EQUALS, LESSTHAN, LESSTHAN_EQUALS, GREATERTHAN, GREATERTHAN_EQUALS}

// BINOPS captures the set of binary operations
var BINOPS = []uint{SUB, MUL, ADD}

// CONNECTIVES captures the set of logical connectives.
var CONNECTIVES = []uint{AND, OR}

// Rule for describing whitespace
var whitespace lex.Scanner[rune] = lex.Many(lex.Or(lex.Unit(' '), lex.Unit('\t')))

// Rule for describing numbers
var (
	binaryStart  = lex.Sequence(lex.String("0b"), lex.Within('0', '1'))
	binaryRest   = lex.Or(lex.Within('0', '1'), lex.Unit('_'))
	decimalStart = lex.Within('0', '9')
	decimalRest  = lex.Or(lex.Within('0', '9'), lex.Unit('_'))
	hexDigit     = lex.Or(lex.Within('0', '9'), lex.Within('A', 'F'), lex.Within('a', 'f'))
	hexStart     = lex.Sequence(lex.String("0x"), hexDigit)
	hexRest      = lex.Or(hexDigit, lex.Unit('_'))
	//
	number = lex.Or(
		lex.SequenceNullableLast(binaryStart, lex.Many(binaryRest)),
		lex.SequenceNullableLast(hexStart, lex.Many(hexRest)),
		lex.SequenceNullableLast(decimalStart, lex.Many(decimalRest)),
	)
)

var identifierStart lex.Scanner[rune] = lex.Or(
	lex.Unit('_'),
	lex.Unit('\''),
	lex.Unit('$'),
	lex.Within('a', 'z'),
	lex.Within('A', 'Z'))

var identifierRest lex.Scanner[rune] = lex.Many(lex.Or(
	lex.Unit('_'),
	lex.Unit('\''),
	lex.Unit('/'),
	lex.Unit('$'),
	lex.Within('0', '9'),
	lex.Within('a', 'z'),
	lex.Within('A', 'Z')))

// Rule for describing identifiers
var identifier lex.Scanner[rune] = lex.And(identifierStart, identifierRest)

// lexing rules
var rules []lex.LexRule[rune] = []lex.LexRule[rune]{
	lex.Rule(lex.Unit('('), LBRACE),
	lex.Rule(lex.Unit(')'), RBRACE),
	lex.Rule(lex.Unit('+'), ADD),
	lex.Rule(lex.Unit('*'), MUL),
	lex.Rule(lex.Unit('-'), SUB),
	lex.Rule(lex.Unit('=', '='), EQUALS),
	lex.Rule(lex.Unit('!', '='), NOT_EQUALS),
	lex.Rule(lex.Unit('≠'), NOT_EQUALS),
	lex.Rule(lex.Unit('<'), LESSTHAN),
	lex.Rule(lex.Unit('<', '='), LESSTHAN_EQUALS),
	lex.Rule(lex.Unit('>'), GREATERTHAN),
	lex.Rule(lex.Unit('>', '='), GREATERTHAN_EQUALS),
	lex.Rule(lex.Unit('|', '|'), OR),
	lex.Rule(lex.Unit('∨'), OR),
	lex.Rule(lex.Unit('&', '&'), AND),
	lex.Rule(lex.Unit('∧'), AND),
	lex.Rule(lex.Unit('⊥'), BOTTOM),
	lex.Rule(lex.Unit('⊤'), TOP),
	lex.Rule(whitespace, WHITESPACE),
	lex.Rule(number, NUMBER),
	lex.Rule(identifier, IDENTIFIER),
	lex.Rule(lex.Eof[rune](), END_OF),
}

// Parser provides a general-purpose parser for propositions and arithmetic
// expressions.
type Parser[T Term[T]] struct {
	environment func(string) bool
	srcfile     *source.File
	tokens      []lex.Token
	// Position within the tokens
	index int
}

// Done determines whether or not the parser has parsed all the available
// tokens.
func (p *Parser[T]) Done() bool {
	return p.index+1 >= len(p.tokens)
}

func (p *Parser[T]) parseTerm() (T, []source.SyntaxError) {
	var (
		tmp        T
		term, errs = p.parseCondition()
	)
	// match all terms
	terms := []T{}
	// initialise lookahead
	kind := p.lookahead().Kind
	//
	for len(errs) == 0 && p.follows(CONNECTIVES...) {
		// Sanity check
		if !p.follows(kind) {
			return tmp, p.syntaxErrors(p.lookahead(), "braces required")
		}
		// Consume connective
		p.expect(p.lookahead().Kind)
		//
		tmp, errs = p.parseCondition()
		// Accumulate arguments
		terms = append(terms, tmp)
	}
	//
	switch {
	case len(errs) != 0:
		return term, errs
	case len(terms) == 0:
		return term, nil
	case kind == OR:
		return term.Or(terms...), nil
	case kind == AND:
		return term.And(terms...), nil
	}
	//
	panic("unreachable")
}

func (p *Parser[T]) parseCondition() (T, []source.SyntaxError) {
	lhs, errs := p.parseArithmeticTerm()
	// See whether binary or not.
	token := p.lookahead()
	// Check for infix expression
	if len(errs) != 0 {
		// Not a binary condition
		return lhs, errs
	} else if !p.follows(CONDITIONS...) {
		// Not a binary condition
		return lhs, nil
	}
	// Accept binary condition
	p.expect(token.Kind)
	// Parse rhs
	rhs, errs := p.parseArithmeticTerm()
	//
	if len(errs) == 0 {
		switch token.Kind {
		case EQUALS:
			lhs = lhs.Equals(rhs)
		case NOT_EQUALS:
			lhs = lhs.NotEquals(rhs)
		case LESSTHAN:
			lhs = lhs.LessThan(rhs)
		case LESSTHAN_EQUALS:
			lhs = lhs.LessThanEquals(rhs)
		case GREATERTHAN:
			lhs = rhs.LessThan(lhs)
		case GREATERTHAN_EQUALS:
			lhs = rhs.LessThanEquals(lhs)
		default:
			errs = p.syntaxErrors(token, "unknown condition")
		}
	}
	// Done
	return lhs, errs
}

func (p *Parser[T]) parseArithmeticTerm() (T, []source.SyntaxError) {
	var (
		tmp        T
		term, errs = p.parseUnitTerm()
	)
	// match all terms
	terms := []T{}
	// initialise lookahead
	kind := p.lookahead().Kind
	//
	for len(errs) == 0 && p.follows(BINOPS...) {
		// Sanity check
		if !p.follows(kind) {
			return tmp, p.syntaxErrors(p.lookahead(), "braces required")
		}
		// Consume connective
		p.expect(p.lookahead().Kind)
		//
		tmp, errs = p.parseUnitTerm()
		// Accumulate arguments
		terms = append(terms, tmp)
	}
	//
	switch {
	case len(errs) != 0:
		return term, errs
	case len(terms) == 0:
		return term, nil
	case kind == ADD:
		return term.Add(terms...), nil
	case kind == MUL:
		return term.Mul(terms...), nil
	case kind == SUB:
		return term.Sub(terms...), nil
	}
	//
	panic("unreachable")
}

// ParseTerm parses an expression.
func (p *Parser[T]) parseUnitTerm() (T, []source.SyntaxError) {
	var (
		empty T
		token = p.lookahead()
	)
	// Otherwise, assume connective
	switch token.Kind {
	case BOTTOM:
		return p.parseLogicalTruth(false)
	case LBRACE:
		return p.parseBracketedTerm()
	case IDENTIFIER:
		return p.parseVariable()
	case NUMBER:
		return p.parseNumber(), nil
	case TOP:
		return p.parseLogicalTruth(true)
	}
	//
	return empty, p.syntaxErrors(token, "unknown expression")
}

func (p *Parser[T]) parseBracketedTerm() (T, []source.SyntaxError) {
	var empty T
	//
	p.expect(LBRACE)
	//
	term, errs := p.parseTerm()
	//
	if len(errs) == 0 && !p.match(RBRACE) {
		return empty, p.syntaxErrors(p.lookahead(), "expected ')'")
	}
	//
	return term, errs
}

func (p *Parser[T]) parseLogicalTruth(val bool) (T, []source.SyntaxError) {
	var truth T
	//
	if val {
		p.expect(TOP)
	} else {
		p.expect(BOTTOM)
	}
	//
	return truth.Truth(val), nil
}

func (p *Parser[T]) parseVariable() (T, []source.SyntaxError) {
	var variable T
	//
	id := p.expect(IDENTIFIER)
	name := p.string(id)
	// Check variable valid
	if p.environment(name) {
		return variable.Variable(name), nil
	}
	// Nope
	return variable, p.syntaxErrors(id, "unknown variable")
}

func (p *Parser[T]) parseNumber() T {
	var num T
	//
	id := p.expect(NUMBER)
	//
	return num.Number(p.number(id))
}

// Get the text representing the given token as a string.
func (p *Parser[T]) string(token lex.Token) string {
	start, end := token.Span.Start(), token.Span.End()
	return string(p.srcfile.Contents()[start:end])
}

// Get the text representing the given token as a string.
func (p *Parser[T]) number(token lex.Token) big.Int {
	var number big.Int
	//
	number.SetString(p.string(token), 0)
	//
	return number
}

// Follows checks whether one of the given token kinds is next.
func (p *Parser[T]) follows(options ...uint) bool {
	return slices.Contains(options, p.lookahead().Kind)
}

// Lookahead returns the next token.  This must exist because EOF is always
// appended at the end of the token stream.
func (p *Parser[T]) lookahead() lex.Token {
	return p.tokens[p.index]
}

func (p *Parser[T]) expect(kind uint) lex.Token {
	if p.lookahead().Kind != kind {
		panic("internal failure")
	}
	//
	token := p.tokens[p.index]
	p.index++
	//
	return token
}

func (p *Parser[T]) match(kind uint) bool {
	if p.lookahead().Kind == kind {
		p.index++
		return true
	}
	//
	return false
}

func (p *Parser[T]) syntaxErrors(token lex.Token, msg string) []source.SyntaxError {
	return []source.SyntaxError{*p.srcfile.SyntaxError(token.Span, msg)}
}
