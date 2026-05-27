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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"go.lsp.dev/protocol"
)

// spanToRange converts a source.Span to a protocol.Range for the given file.
// Both start and end positions are computed by finding the enclosing line for
// each boundary, then computing the character offset within that line.
func spanToRange(srcfile source.File, span source.Span) protocol.Range {
	startLine := srcfile.FindFirstEnclosingLine(span)
	startLspLine := uint32(startLine.Number() - 1)
	startLspChar := uint32(span.Start() - startLine.Start())

	var endLspLine, endLspChar uint32

	if span.End() > span.Start() {
		// Find the line that contains the last character of the span.
		endSpan := source.NewSpan(span.End()-1, span.End())
		endLine := srcfile.FindFirstEnclosingLine(endSpan)
		endLspLine = uint32(endLine.Number() - 1)
		endLspChar = uint32(span.End() - endLine.Start())
	} else {
		// Empty span: end == start.
		endLspLine = startLspLine
		endLspChar = startLspChar
	}

	return protocol.Range{
		Start: protocol.Position{Line: startLspLine, Character: startLspChar},
		End:   protocol.Position{Line: endLspLine, Character: endLspChar},
	}
}

// posToOffset converts an LSP Position (0-indexed line and character) to a
// rune offset within the source file.  If the line number exceeds the number
// of lines in the file, the offset of the end of the file is returned.
func posToOffset(srcfile source.File, pos protocol.Position) int {
	lines := srcfile.Lines()
	lineIdx := int(pos.Line)

	if lineIdx >= len(lines) {
		return len(srcfile.Contents())
	}

	return lines[lineIdx].Start() + int(pos.Character)
}

// tokenAtOffset finds the token whose span contains the given rune offset,
// returning it, its index in tokens, and true.  If no token spans that
// offset, a zero Token, -1, and false are returned.
func tokenAtOffset(tokens []lex.Token, offset int) (lex.Token, int, bool) {
	for i, tok := range tokens {
		if tok.Span.Start() <= offset && offset < tok.Span.End() {
			return tok, i, true
		}
	}

	return lex.Token{}, -1, false
}

// dataTypeToString converts a data type to its string representation. A nil
// type can arise when the compiler encounters an error (e.g. an unresolved
// symbol), so we guard against that here to avoid crashing the LSP server.
func dataTypeToString(datatype data.Type[symbol.Resolved], env data.ResolvedEnvironment) string {
	if datatype == nil {
		return "???"
	}
	//
	return datatype.String(env)
}
