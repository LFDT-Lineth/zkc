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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
	"go.lsp.dev/protocol"
)

// SignatureHelpFor returns signature help for the function call the cursor
// is inside.  The caller supplies an already-compiled program (typically
// taken from an IncrementalCompiler); this function only lexes the supplied
// document text to locate the enclosing call and active parameter.  Returns
// nil when the cursor is not inside a function-call argument list or the
// callee cannot be resolved.
func SignatureHelpFor(
	uri protocol.URI, text string, pos protocol.Position,
	program ast.Program,
) (*protocol.SignatureHelp, error) {
	srcfile := source.NewSourceFile(uri.Filename(), []byte(text))

	// Convert LSP cursor position to a rune offset in the source file.
	offset := posToOffset(*srcfile, pos)

	// Lex the document without comments; the backward scan only needs the
	// structural tokens, and token spans already refer to original source offsets.
	tokens := parser.Lex(*srcfile, false, false)

	// Find the enclosing function call and active parameter index.
	contents := srcfile.Contents()

	fnName, activeParam, ok := findCallContext(tokens, contents, offset)
	if !ok {
		return nil, nil
	}

	env := program.Environment()

	// Search for the function declaration with that name.
	for _, d := range program.Components() {
		fn, isFn := d.(*decl.ResolvedFunction)
		if !isFn || fn.Name() != fnName {
			continue
		}

		sig := buildSignatureInfo(fn, env)

		return &protocol.SignatureHelp{
			Signatures:      []protocol.SignatureInformation{sig},
			ActiveSignature: 0,
			ActiveParameter: activeParam,
		}, nil
	}

	return nil, nil
}

// findCallContext walks the token list backward from offset to find the name of
// the function call enclosing the cursor and the 0-based index of the argument
// position the cursor is in (counted by commas). Returns ok=false when the
// cursor is not inside a function-call argument list.
//
// The scan counts all grouping tokens — `(`, `)`, `[`, `]` — for depth
// tracking. A comma at depth 0 means we advance the active-parameter counter.
// When an unmatched `(` is found at depth 0 and the preceding token is an
// identifier, that identifier is the callee. An unmatched `[` at depth 0
// means the cursor is inside a memory subscript, so no signature help applies.
func findCallContext(tokens []lex.Token, contents []rune, offset int) (name string, activeParam uint32, ok bool) {
	depth := 0
	commas := 0

	for i := len(tokens) - 1; i >= 0; i-- {
		tok := tokens[i]

		// Only consider tokens that end before (or at) the cursor.
		if tok.Span.Start() >= offset {
			continue
		}

		switch tok.Kind {
		case parser.RBRACE, parser.RSQUARE:
			depth++

		case parser.LSQUARE:
			if depth <= 0 {
				// Cursor is directly inside a memory subscript — no sig help.
				return "", 0, false
			}
			//
			depth--

		case parser.LBRACE:
			if depth <= 0 {
				// Found the opening `(` of the enclosing call.
				// The token immediately before it must be an identifier.
				if i > 0 && tokens[i-1].Kind == parser.IDENTIFIER {
					t := tokens[i-1]
					name = string(contents[t.Span.Start():t.Span.End()])

					return name, uint32(commas), true
				}
				// Cursor is inside a bare parenthesised expression, not a call.
				return "", 0, false
			}
			//
			depth--
		case parser.COMMA:
			if depth == 0 {
				commas++
			}
		}
	}

	return "", 0, false
}

// buildSignatureInfo constructs the LSP SignatureInformation for the given
// function. The label is the full function signature; each input parameter
// contributes a ParameterInformation whose label is a substring of that label.
func buildSignatureInfo(fn *decl.ResolvedFunction, env data.ResolvedEnvironment) protocol.SignatureInformation {
	label := formatFunctionDecl(fn, env)

	params := make([]protocol.ParameterInformation, 0, len(fn.Inputs()))
	for _, v := range fn.Inputs() {
		params = append(params, protocol.ParameterInformation{
			Label: v.Name + ": " + dataTypeToString(v.DataType, env),
		})
	}

	return protocol.SignatureInformation{
		Label:      label,
		Parameters: params,
	}
}
