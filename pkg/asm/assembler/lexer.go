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
package assembler

import (
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
)

// END_OF signals "end of file"
const END_OF uint = 0

// WHITESPACE signals whitespace
const WHITESPACE uint = 1

// COMMENT signals ";; ... \n"
const COMMENT uint = 2

// LBRACE signals "("
const LBRACE uint = 3

// RBRACE signals ")"
const RBRACE uint = 4

// LCURLY signals "{"
const LCURLY uint = 5

// RCURLY signals "}"
const RCURLY uint = 6

// COMMA signals ","
const COMMA uint = 7

// COLON signals ":"
const COLON uint = 8

// SEMICOLON signals ";"
const SEMICOLON uint = 9

// NUMBER signals an integer number
const NUMBER uint = 10

// STRING signals a quoted string
const STRING uint = 11

// IDENTIFIER signals a column variable
const IDENTIFIER uint = 20

// KEYWORD_CONST signals a constant declaration
const KEYWORD_CONST uint = 21

// KEYWORD_INCLUDE signals an include declaration
const KEYWORD_INCLUDE uint = 22

// KEYWORD_FN signals a function declaration
const KEYWORD_FN uint = 23

// KEYWORD_PUB signals a public function declaration
const KEYWORD_PUB uint = 24

// RIGHTARROW signals "->"
const RIGHTARROW uint = 30

// EQUALS signals "="
const EQUALS uint = 31

// EQUALS_EQUALS signals "=="
const EQUALS_EQUALS uint = 32

// NOT_EQUALS signals "!="
const NOT_EQUALS uint = 33

// LESS_THAN signals "<"
const LESS_THAN uint = 34

// LESS_THAN_EQUALS signals "<="
const LESS_THAN_EQUALS uint = 35

// GREATER_THAN signals ">"
const GREATER_THAN uint = 36

// GREATER_THAN_EQUALS signals ">="
const GREATER_THAN_EQUALS uint = 37

// ADD signals "+"
const ADD uint = 38

// SUB signals "-"
const SUB uint = 39

// MUL signals "*"
const MUL uint = 40

// DIV signals "/"
const DIV uint = 41

// QMARK signals "?"
const QMARK uint = 50

// Rule for describing whitespace
var whitespace lex.Scanner[rune] = lex.Many(lex.Or(lex.Unit(' '), lex.Unit('\t'), lex.Unit('\n')))

// Rule for describing numbers
// A number is either a hexadecimal, binary, or decimal one.
// Allowing (and ignoring) '_' in the middle of a number for readability.
var (
	binaryStart  = lex.Sequence(lex.String("0b"), lex.Within('0', '1'))
	binaryRest   = lex.Or(lex.Within('0', '1'), lex.Unit('_'))
	decimalStart = lex.Within('0', '9')
	decimalRest  = lex.Or(lex.Within('0', '9'), lex.Unit('_'), lex.Unit('^'))
	hexDigit     = lex.Or(lex.Within('0', '9'), lex.Within('A', 'F'), lex.Within('a', 'f'))
	hexStart     = lex.Sequence(lex.String("0x"), hexDigit)
	hexRest      = lex.Or(hexDigit, lex.Unit('_'))

	number = lex.Or(
		lex.SequenceNullableLast(binaryStart, lex.Many(binaryRest)),
		lex.SequenceNullableLast(hexStart, lex.Many(hexRest)),
		lex.SequenceNullableLast(decimalStart, lex.Many(decimalRest)),
	)
)

var identifierStart lex.Scanner[rune] = lex.Or(
	lex.Unit('_'),
	lex.Unit('\''),
	lex.Within('a', 'z'),
	lex.Within('A', 'Z'))

var identifierRest lex.Scanner[rune] = lex.Many(lex.Or(
	lex.Unit('_'),
	lex.Unit('\''),
	lex.Within('0', '9'),
	lex.Within('a', 'z'),
	lex.Within('A', 'Z')))

// Rule for describing identifiers
var identifier lex.Scanner[rune] = lex.And(identifierStart, identifierRest)

// Rule for describing strings in quotes
var strung lex.Scanner[rune] = lex.Sequence(lex.Unit('"'), lex.Many(lex.Not('"')), lex.Unit('"'))

// Comments start with ';;'
var commentStart lex.Scanner[rune] = lex.Unit(';', ';')

// Comments continue until a newline or EOF.
var commentRest lex.Scanner[rune] = lex.Until('\n')

var comment lex.Scanner[rune] = lex.And(commentStart, commentRest)

// lexing rules
var rules []lex.LexRule[rune] = []lex.LexRule[rune]{
	lex.Rule(comment, COMMENT),
	lex.Rule(lex.Unit('('), LBRACE),
	lex.Rule(lex.Unit(')'), RBRACE),
	lex.Rule(lex.Unit('{'), LCURLY),
	lex.Rule(lex.Unit('}'), RCURLY),
	lex.Rule(lex.Unit(','), COMMA),
	lex.Rule(lex.Unit(':'), COLON),
	lex.Rule(lex.Unit(';'), SEMICOLON),
	lex.Rule(lex.Unit('-', '>'), RIGHTARROW),
	lex.Rule(lex.Unit('=', '='), EQUALS_EQUALS),
	lex.Rule(lex.Unit('!', '='), NOT_EQUALS),
	lex.Rule(lex.Unit('<', '='), LESS_THAN_EQUALS),
	lex.Rule(lex.Unit('>', '='), GREATER_THAN_EQUALS),
	lex.Rule(lex.Unit('<'), LESS_THAN),
	lex.Rule(lex.Unit('>'), GREATER_THAN),
	lex.Rule(lex.Unit('='), EQUALS),
	lex.Rule(lex.Unit('+'), ADD),
	lex.Rule(lex.Unit('-'), SUB),
	lex.Rule(lex.Unit('*'), MUL),
	lex.Rule(lex.Unit('/'), DIV),
	lex.Rule(lex.Unit('?'), QMARK),
	lex.Rule(whitespace, WHITESPACE),
	lex.Rule(number, NUMBER),
	lex.Rule(strung, STRING),
	lex.Rule(lex.String("pub"), KEYWORD_PUB),
	lex.Rule(lex.String("const"), KEYWORD_CONST),
	lex.Rule(lex.String("include"), KEYWORD_INCLUDE),
	lex.Rule(lex.String("fn"), KEYWORD_FN),
	lex.Rule(identifier, IDENTIFIER),
	lex.Rule(lex.Eof[rune](), END_OF),
}

// Lex a given source file into a sequence of zero or more tokens, along with
// any syntax errors arising.
func Lex(srcfile source.File) ([]lex.Token, []source.SyntaxError) {
	var (
		lexer = lex.NewLexer(srcfile.Contents(), rules...)
		// Lex as many tokens as possible
		tokens = lexer.Collect()
	)
	// Check whether anything was left (if so this is an error)
	if lexer.Remaining() != 0 {
		start, end := lexer.Index(), lexer.Index()+lexer.Remaining()
		err := srcfile.SyntaxError(source.NewSpan(int(start), int(end)), "unknown text encountered")
		// errors
		return nil, []source.SyntaxError{*err}
	}
	// Remove any whitespace
	tokens = array.RemoveMatching(tokens, func(t lex.Token) bool { return t.Kind == WHITESPACE })
	// Remove any comments (for not)
	tokens = array.RemoveMatching(tokens, func(t lex.Token) bool { return t.Kind == COMMENT })
	// Done
	return tokens, nil
}
