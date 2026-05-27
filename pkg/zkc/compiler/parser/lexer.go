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
package parser

import (
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
)

const (
	// EOF signals "end of file"
	EOF uint = iota
	// WHITESPACE signals whitespace
	WHITESPACE
	// NEWLINE signals a newline character
	NEWLINE
	// COMMENT signals "// ... \n"
	COMMENT
	// LBRACE signals "("
	LBRACE
	// RBRACE signals ")"
	RBRACE
	// LCURLY signals "{"
	LCURLY
	// RCURLY signals "}"
	RCURLY
	// LSQUARE signals "["
	LSQUARE
	// RSQUARE signals "]"
	RSQUARE
	// COMMA signals ","
	COMMA
	// COLON signals ":"
	COLON
	// COLONCOLON signals "::"
	COLONCOLON
	// SEMICOLON signals ";"
	SEMICOLON
	// NUMBER signals an integer number
	NUMBER
	// STRING signals a quoted string
	STRING
	// IDENTIFIER signals a column variable
	IDENTIFIER
	// KEYWORD_AS signals a type cast expression (e.g. "x as u8")
	KEYWORD_AS
	// KEYWORD_BREAK signals a break statement
	KEYWORD_BREAK
	// KEYWORD_CONTINUE signals a continue statement
	KEYWORD_CONTINUE
	// KEYWORD_CONST signals a constant declaration
	KEYWORD_CONST
	// KEYWORD_ELSE signals an else branch
	KEYWORD_ELSE
	// KEYWORD_FAIL signals a return statement
	KEYWORD_FAIL
	// KEYWORD_FN signals a function declaration
	KEYWORD_FN
	// KEYWORD_FOR signals a for loop
	KEYWORD_FOR
	// KEYWORD_IF signals a return statement
	KEYWORD_IF
	// KEYWORD_INCLUDE signals an include declaration
	KEYWORD_INCLUDE
	// KEYWORD_INPUT signals a read-only memory
	KEYWORD_INPUT
	// KEYWORD_MEMORY signals a random-access memory declaration
	KEYWORD_MEMORY
	// KEYWORD_RETURN signals a return statement
	KEYWORD_RETURN
	// KEYWORD_STATIC signals a static read-only memory
	KEYWORD_STATIC
	// KEYWORD_OUTPUT signals a write-once memory
	KEYWORD_OUTPUT
	// KEYWORD_PRINTF signals a printf statement
	KEYWORD_PRINTF
	// KEYWORD_PUB signals a public input / output
	KEYWORD_PUB
	// KEYWORD_WHILE signals a while loop
	KEYWORD_WHILE
	// KEYWORD_VAR signals a local variable declaration
	KEYWORD_VAR
	// KEYWORD_TYPE signals a type alias declaration
	KEYWORD_TYPE
	// KEYWORD_SWITCH signals the beginning of a switch statement
	KEYWORD_SWITCH
	// KEYWORD_CASE signals a case in a switch statement
	KEYWORD_CASE
	// KEYWORD_DEFAULT signals the default case in a switch statement
	KEYWORD_DEFAULT
	// RIGHTARROW signals "->"
	RIGHTARROW
	// EQUALS signals "="
	EQUALS
	// EQUALS_EQUALS signals "=="
	EQUALS_EQUALS
	// NOT_EQUALS signals "!="
	NOT_EQUALS
	// LESS_THAN signals "<"
	LESS_THAN
	// LESS_THAN_EQUALS signals "<="
	LESS_THAN_EQUALS
	// GREATER_THAN signals ">"
	GREATER_THAN
	// GREATER_THAN_EQUALS signals ">="
	GREATER_THAN_EQUALS
	// LOGICAL_AND signals "&&"
	LOGICAL_AND
	// LOGICAL_OR signals "||"
	LOGICAL_OR
	// LOGICAL_NOT signals "!"
	LOGICAL_NOT
	// ADD signals "+"
	ADD
	// SUB signals "-"
	SUB
	// MUL signals "*"
	MUL
	// DIV signals "/"
	DIV
	// BITWISE_AND signals "&"
	BITWISE_AND
	// BITWISE_OR signals "|"
	BITWISE_OR
	// BITWISE_XOR signals "^"
	BITWISE_XOR
	// BITWISE_NOT signals "~"
	BITWISE_NOT
	// BITWISE_SHL signals "<<"
	BITWISE_SHL
	// BITWISE_SHR signals ">>"
	BITWISE_SHR
	// REM signals "%"
	REM
	// QMARK signals "?"
	QMARK
	// HASH signals "#"
	HASH
	// AT signals "@"
	AT
	// UNKNOWN signals an unknown chunk of text
	UNKNOWN
	// SPACES signal a set of one or more spaces.  This is a virtual token only
	// used by the formatter.
	SPACES
	// TABS signal a set of one or more tabs.  This is a virtual token only used
	// by the formatter.
	TABS
	// FIELD_ELEMENT signals the "𝔽" field element type token.
	FIELD_ELEMENT
	// MAX_TOKEN signals the maximum token index
	MAX_TOKEN
)

// Rule for describing whitespace (spaces and tabs only, not newlines)
var whitespace lex.Scanner[rune] = lex.Many(lex.Or(lex.Unit(' '), lex.Unit('\t')))

// Rule for describing a newline
var newline lex.Scanner[rune] = lex.Unit('\n')

// Rule for capturing everything upto the next bit of whitespace.  This is
// useful as it allows lexing to continue even when encountering something not
// matched by any rule.
var notWhitespaceOrNewline lex.Scanner[rune] = lex.Many(lex.And(lex.Not(' '), lex.Not('\t'), lex.Not('\n')))

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

// Comments start with '//'
var commentStart lex.Scanner[rune] = lex.Unit('/', '/')

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
	lex.Rule(lex.Unit('['), LSQUARE),
	lex.Rule(lex.Unit(']'), RSQUARE),
	lex.Rule(lex.Unit(','), COMMA),
	lex.Rule(lex.Unit(':', ':'), COLONCOLON),
	lex.Rule(lex.Unit(':'), COLON),
	lex.Rule(lex.Unit(';'), SEMICOLON),
	lex.Rule(lex.Unit('-', '>'), RIGHTARROW),
	lex.Rule(lex.Unit('=', '='), EQUALS_EQUALS),
	lex.Rule(lex.Unit('!', '='), NOT_EQUALS),
	lex.Rule(lex.Unit('<', '='), LESS_THAN_EQUALS),
	lex.Rule(lex.Unit('>', '='), GREATER_THAN_EQUALS),
	lex.Rule(lex.Unit('<', '<'), BITWISE_SHL),
	lex.Rule(lex.Unit('>', '>'), BITWISE_SHR),
	lex.Rule(lex.Unit('<'), LESS_THAN),
	lex.Rule(lex.Unit('>'), GREATER_THAN),
	lex.Rule(lex.Unit('='), EQUALS),
	lex.Rule(lex.Unit('+'), ADD),
	lex.Rule(lex.Unit('-'), SUB),
	lex.Rule(lex.Unit('*'), MUL),
	lex.Rule(lex.Unit('/'), DIV),
	lex.Rule(lex.Unit('%'), REM),
	lex.Rule(lex.Unit('!'), LOGICAL_NOT),
	lex.Rule(lex.Unit('&', '&'), LOGICAL_AND),
	lex.Rule(lex.Unit('|', '|'), LOGICAL_OR),
	lex.Rule(lex.Unit('&'), BITWISE_AND),
	lex.Rule(lex.Unit('|'), BITWISE_OR),
	lex.Rule(lex.Unit('^'), BITWISE_XOR),
	lex.Rule(lex.Unit('~'), BITWISE_NOT),
	lex.Rule(lex.Unit('?'), QMARK),
	lex.Rule(lex.Unit('#'), HASH),
	lex.Rule(lex.Unit('@'), AT),
	lex.Rule(whitespace, WHITESPACE),
	lex.Rule(newline, NEWLINE),
	lex.Rule(number, NUMBER),
	lex.Rule(strung, STRING),
	lex.Rule(identifier, IDENTIFIER),
	lex.Rule(lex.Unit('𝔽'), FIELD_ELEMENT),
	lex.Rule(notWhitespaceOrNewline, UNKNOWN),
	lex.Rule(lex.Eof[rune](), EOF),
}

// keywords maps exact identifier strings to their keyword token kinds.
// Reclassification happens as a post-processing step in Lex so that
// identifiers that merely start with a keyword (e.g. "as_X") are never
// misidentified: the identifier rule always consumes the full token, and
// only an exact match triggers reclassification.
var keywords = map[string]uint{
	"as":       KEYWORD_AS,
	"break":    KEYWORD_BREAK,
	"const":    KEYWORD_CONST,
	"continue": KEYWORD_CONTINUE,
	"else":     KEYWORD_ELSE,
	"fail":     KEYWORD_FAIL,
	"fn":       KEYWORD_FN,
	"for":      KEYWORD_FOR,
	"if":       KEYWORD_IF,
	"include":  KEYWORD_INCLUDE,
	"input":    KEYWORD_INPUT,
	"memory":   KEYWORD_MEMORY,
	"output":   KEYWORD_OUTPUT,
	"printf":   KEYWORD_PRINTF,
	"pub":      KEYWORD_PUB,
	"return":   KEYWORD_RETURN,
	"static":   KEYWORD_STATIC,
	"type":     KEYWORD_TYPE,
	"var":      KEYWORD_VAR,
	"while":    KEYWORD_WHILE,
	"switch":   KEYWORD_SWITCH,
	"case":     KEYWORD_CASE,
	"default":  KEYWORD_DEFAULT,
}

// MAX_KEYWORD_LENGTH is used to optimise lexing of keywords.
var MAX_KEYWORD_LENGTH int

// Lex a given source file into a sequence of zero or more tokens, along with
// any syntax errors arising.  This can be configured to retain whitespace
// and/or comments.
func Lex(srcfile source.File, whitespace, comments bool) []lex.Token {
	var (
		lexer = lex.NewLexer(srcfile.Contents(), rules...)
		// Lex as many tokens as possible
		tokens = lexer.Collect()
	)
	// Remove whitespace and/or comments unless the caller wants them (e.g. for
	// syntax highlighting)
	if !whitespace || !comments {
		// Remove any whitespace, newlines or comments
		tokens = array.RemoveMatching(tokens, func(t lex.Token) bool {
			switch t.Kind {
			case WHITESPACE, NEWLINE:
				return !whitespace
			case COMMENT:
				return !comments
			}

			return false
		})
	}
	// Reclassify identifiers whose full text is an exact keyword match.
	contents := srcfile.Contents()

	for i, tok := range tokens {
		// Check whether the given identifier is a keyword, or not.
		if tok.Kind == IDENTIFIER && tok.Span.Length() <= MAX_KEYWORD_LENGTH {
			text := string(contents[tok.Span.Start():tok.Span.End()])
			if kind, ok := keywords[text]; ok {
				tokens[i].Kind = kind
			}
		}
	}
	// Done
	return tokens
}

// IsValidIdentifier reports whether s parses as a single IDENTIFIER token,
// rejecting empty strings, keywords, and anything containing whitespace or
// punctuation.
func IsValidIdentifier(s string) bool {
	var (
		runes = []rune(s)
		n     = identifier(runes)
	)
	//
	if n != uint(len(runes)) || len(runes) == 0 {
		return false
	}
	//
	_, isKeyword := keywords[s]

	return !isKeyword
}

func init() {
	// Statically compute maximum length of any keyword
	for k := range keywords {
		MAX_KEYWORD_LENGTH = max(MAX_KEYWORD_LENGTH, len(k))
	}
}
