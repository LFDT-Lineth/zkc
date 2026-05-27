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
package lsp

import (
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
	"go.lsp.dev/protocol"
)

// SemTokOptions is a local replacement for protocol.SemanticTokensOptions,
// which in go.lsp.dev/protocol@v0.12.0 is missing the Legend and Full fields.
type SemTokOptions struct {
	Legend protocol.SemanticTokensLegend `json:"legend"`
	Full   bool                          `json:"full"`
}

// SemTokLegend declares the token types the server emits. The index of each
// entry is the numeric value encoded in SemanticTokens.Data.
var SemTokLegend = protocol.SemanticTokensLegend{
	TokenTypes: []protocol.SemanticTokenTypes{
		protocol.SemanticTokenKeyword,  // 0
		protocol.SemanticTokenComment,  // 1
		protocol.SemanticTokenString,   // 2
		protocol.SemanticTokenNumber,   // 3
		protocol.SemanticTokenOperator, // 4
		protocol.SemanticTokenType,     // 5
		protocol.SemanticTokenFunction, // 6
		protocol.SemanticTokenVariable, // 7
	},
	TokenModifiers: []protocol.SemanticTokenModifiers{},
}

// Named indices into SemTokLegend.TokenTypes. The order here must mirror the
// legend exactly; the iota ensures the values stay in step if entries are added.
const (
	semTokKeyword  uint32 = iota // protocol.SemanticTokenKeyword
	semTokComment                // protocol.SemanticTokenComment
	semTokString                 // protocol.SemanticTokenString
	semTokNumber                 // protocol.SemanticTokenNumber
	semTokOperator               // protocol.SemanticTokenOperator
	semTokType                   // protocol.SemanticTokenType
	semTokFunction               // protocol.SemanticTokenFunction
	semTokVariable               // protocol.SemanticTokenVariable
)

// semTokForKind maps a ZkC lexer token kind to an index into SemTokLegend.TokenTypes.
// The second return value is false for tokens that should not be emitted
// (punctuation, whitespace, EOF).
func semTokForKind(kind uint) (uint32, bool) {
	switch kind {
	// Keywords
	case parser.KEYWORD_AS, parser.KEYWORD_BREAK, parser.KEYWORD_CONST,
		parser.KEYWORD_CONTINUE, parser.KEYWORD_ELSE, parser.KEYWORD_FAIL,
		parser.KEYWORD_FN, parser.KEYWORD_FOR, parser.KEYWORD_IF,
		parser.KEYWORD_SWITCH, parser.KEYWORD_CASE, parser.KEYWORD_DEFAULT,
		parser.KEYWORD_INCLUDE, parser.KEYWORD_INPUT, parser.KEYWORD_MEMORY,
		parser.KEYWORD_OUTPUT, parser.KEYWORD_PRINTF, parser.KEYWORD_PUB,
		parser.KEYWORD_RETURN, parser.KEYWORD_STATIC, parser.KEYWORD_TYPE,
		parser.KEYWORD_VAR, parser.KEYWORD_WHILE:
		return semTokKeyword, true
	// Comments
	case parser.COMMENT:
		return semTokComment, true
	// String literals
	case parser.STRING:
		return semTokString, true
	// Numeric literals
	case parser.NUMBER:
		return semTokNumber, true
	// Operators and punctuation-like tokens that carry meaning
	case parser.ADD, parser.SUB, parser.MUL, parser.DIV, parser.REM,
		parser.EQUALS, parser.EQUALS_EQUALS, parser.NOT_EQUALS,
		parser.LESS_THAN, parser.LESS_THAN_EQUALS,
		parser.GREATER_THAN, parser.GREATER_THAN_EQUALS,
		parser.LOGICAL_AND, parser.LOGICAL_OR, parser.LOGICAL_NOT,
		parser.BITWISE_AND, parser.BITWISE_OR, parser.BITWISE_XOR,
		parser.BITWISE_NOT, parser.BITWISE_SHL, parser.BITWISE_SHR,
		parser.RIGHTARROW, parser.QMARK:
		return semTokOperator, true
	// Identifiers — emitted as 'variable'; future work can refine via AST
	case parser.IDENTIFIER:
		return semTokVariable, true
	// Punctuation (braces, colon, comma, semicolon) and EOF: skip
	default:
		return 0, false
	}
}

// encodeTokens converts a slice of lexer tokens into the LSP relative encoding.
// Each token is represented by 5 consecutive uint32 values:
//
//	[deltaLine, deltaStartChar, length, tokenType, tokenModifiers]
//
// Positions are relative to the previous token (or the start of the file for
// the first token). tokenModifiers is always 0 for now.
//
// A small state machine tracks the previous non-comment token kind so that
// identifiers in type-annotation position (after ':' or 'as') are classified
// as type tokens, and function-name identifiers (after 'fn') as function tokens.
// Lookahead is also used: an identifier immediately followed by '(' is a call
// site and is classified as a function token.
func encodeTokens(srcfile source.File, tokens []lex.Token) []uint32 {
	data := make([]uint32, 0, len(tokens)*5)

	var prevLine, prevChar uint32
	// prevMeaningfulKind tracks the kind of the last non-comment token seen.
	// Initialised to END_OF (0) which matches no special case.
	prevMeaningfulKind := parser.EOF

	for i, tok := range tokens {
		var (
			tokType uint32
			ok      bool
		)

		if tok.Kind == parser.IDENTIFIER {
			// Look ahead past any intervening comment to the next meaningful token.
			nextKind := parser.EOF

			for j := i + 1; j < len(tokens); j++ {
				if tokens[j].Kind != parser.COMMENT {
					nextKind = tokens[j].Kind
					break
				}
			}

			switch {
			case prevMeaningfulKind == parser.COLON || prevMeaningfulKind == parser.KEYWORD_AS:
				tokType, ok = semTokType, true // type annotation
			case prevMeaningfulKind == parser.KEYWORD_FN || nextKind == parser.LBRACE:
				tokType, ok = semTokFunction, true // function declaration name or call site
			default:
				tokType, ok = semTokVariable, true // variable
			}
		} else {
			tokType, ok = semTokForKind(tok.Kind)
		}

		// Update state for all non-comment tokens so that a comment between
		// ':' and a type name does not reset the type-position detection.
		if tok.Kind != parser.COMMENT {
			prevMeaningfulKind = tok.Kind
		}

		if !ok {
			continue
		}

		line := srcfile.FindFirstEnclosingLine(tok.Span)
		// LSP uses 0-indexed lines and characters.
		lspLine := uint32(line.Number() - 1)
		lspChar := uint32(tok.Span.Start() - line.Start())
		length := uint32(tok.Span.Length())

		deltaLine := lspLine - prevLine

		deltaChar := lspChar
		if deltaLine == 0 {
			deltaChar = lspChar - prevChar
		}

		data = append(data, deltaLine, deltaChar, length, tokType, 0)
		prevLine = lspLine
		prevChar = lspChar
	}

	return data
}

// SemanticTokensFor lexes the given document text and returns a SemanticTokens
// response for the full document.
func SemanticTokensFor(uri protocol.URI, text string) (*protocol.SemanticTokens, error) {
	srcfile := source.NewSourceFile(string(uri), []byte(text))
	tokens := parser.Lex(*srcfile, false, true)

	return &protocol.SemanticTokens{
		Data: encodeTokens(*srcfile, tokens),
	}, nil
}
