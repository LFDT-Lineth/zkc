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
	"strings"

	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
	"go.lsp.dev/protocol"
)

// HoverFor returns hover information for the symbol under the cursor at pos.
// The caller supplies an already-compiled program and source maps (typically
// taken from an IncrementalCompiler); this function only lexes the supplied
// document text to find the identifier under the cursor.  Returns nil when
// no hover content is available (e.g. the cursor is on punctuation or the
// identifier names neither a local nor a top-level declaration).
func HoverFor(
	uri protocol.URI, text string, pos protocol.Position,
	program ast.Program, srcmaps source.Maps[any],
) (*protocol.Hover, error) {
	srcfile := source.NewSourceFile(uri.Filename(), []byte(text))

	// Convert LSP cursor position to a rune offset in the source file.
	offset := posToOffset(*srcfile, pos)

	// Lex the document to find the identifier token under the cursor.
	tokens := parser.Lex(*srcfile, false, false)

	tok, idx, ok := tokenAtOffset(tokens, offset)
	if !ok {
		return nil, nil
	}

	contents := srcfile.Contents()

	// Annotations (e.g. #[inline]) take precedence: the cursor may be on any
	// token in the annotation.
	if h := hoverAnnotation(tokens, idx, contents); h != nil {
		return h, nil
	}

	if tok.Kind != parser.IDENTIFIER {
		return nil, nil
	}

	// Extract the identifier text.
	name := string(contents[tok.Span.Start():tok.Span.End()])

	env := program.Environment()

	// 1. Local variables take precedence: a local shadows any top-level
	//    declaration with the same name.
	if h := hoverLocalVariable(name, offset, &program, srcmaps, *srcfile, env); h != nil {
		return h, nil
	}

	// 2. Fall back to top-level declarations.
	for _, d := range program.Components() {
		if d.Name() != name {
			continue
		}

		sig := formatDeclaration(d, env)
		hover := sig

		// Prepend any // doc-comment lines immediately preceding the declaration.
		if declFile, span, found := srcmaps.Lookup(d); found {
			if doc := extractPrecedingComments(declFile, span.Start()); doc != "" {
				hover = doc + "\n\n---\n\n" + sig
			}
		}

		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: hover,
			},
		}, nil
	}

	return nil, nil
}

// hoverAnnotation returns hover content if the token at idx is part of an
// annotation usage. Returns nil otherwise, or when the annotation name is
// not registered in decl.ANNOTATIONS.
func hoverAnnotation(tokens []lex.Token, idx int, contents []rune) *protocol.Hover {
	nameTok, ok := annotationNameToken(tokens, idx)
	if !ok {
		return nil
	}

	name := string(contents[nameTok.Span.Start():nameTok.Span.End()])

	for _, ann := range decl.ANNOTATIONS {
		if ann.Name() == name {
			value := "```zkc\n#[" + ann.Name() + "]\n```\n\n" + ann.Description()
			//
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: value,
				},
			}
		}
	}

	return nil
}

func annotationNameToken(tokens []lex.Token, idx int) (lex.Token, bool) {
	switch tokens[idx].Kind {
	case parser.HASH:
		if idx+3 < len(tokens) && tokens[idx+1].Kind == parser.LSQUARE &&
			tokens[idx+2].Kind == parser.IDENTIFIER && tokens[idx+3].Kind == parser.RSQUARE {
			return tokens[idx+2], true
		}
	case parser.LSQUARE:
		if idx >= 1 && idx+2 < len(tokens) && tokens[idx-1].Kind == parser.HASH &&
			tokens[idx+1].Kind == parser.IDENTIFIER && tokens[idx+2].Kind == parser.RSQUARE {
			return tokens[idx+1], true
		}
	case parser.IDENTIFIER:
		if idx >= 2 && idx+1 < len(tokens) && tokens[idx-2].Kind == parser.HASH &&
			tokens[idx-1].Kind == parser.LSQUARE && tokens[idx+1].Kind == parser.RSQUARE {
			return tokens[idx], true
		}
	case parser.RSQUARE:
		if idx >= 3 && tokens[idx-3].Kind == parser.HASH && tokens[idx-2].Kind == parser.LSQUARE &&
			tokens[idx-1].Kind == parser.IDENTIFIER {
			return tokens[idx-1], true
		}
	}

	return lex.Token{}, false
}

// hoverLocalVariable searches for a local variable named name inside the
// function whose span in srcfile contains offset. Returns a Hover with the
// variable's type, or nil if no match is found.
func hoverLocalVariable(
	name string,
	offset int,
	program interface{ Components() []decl.Resolved },
	srcmaps source.Maps[any],
	srcfile source.File,
	env data.ResolvedEnvironment,
) *protocol.Hover {
	for _, d := range program.Components() {
		fn, ok := d.(*decl.ResolvedFunction)
		if !ok {
			continue
		}

		// Check whether the cursor falls within this function's span.
		srcFile, span, found := srcmaps.Lookup(d)
		if !found || srcFile.Filename() != srcfile.Filename() {
			continue
		}

		if offset < span.Start() || offset >= span.End() {
			continue
		}

		// Cursor is inside this function — search its variables.
		for _, v := range fn.Variables {
			if v.Name == name {
				return &protocol.Hover{
					Contents: protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: "```zkc\n" + name + ": " + dataTypeToString(v.DataType, env) + "\n```",
					},
				}
			}
		}
	}

	return nil
}

// formatDeclaration formats a top-level declaration as a markdown code block
// suitable for display in a hover popup.
func formatDeclaration(d decl.Resolved, env data.ResolvedEnvironment) string {
	var sig string

	switch d := d.(type) {
	case *decl.ResolvedFunction:
		sig = formatFunctionDecl(d, env)
	case *decl.ResolvedConstant:
		sig = "const " + d.Name() + ": " + dataTypeToString(d.DataType, env)
	case *decl.ResolvedMemory:
		sig = formatMemoryDecl(d, env)
	case *decl.ResolvedTypeAlias:
		sig = "type " + d.Name() + " = " + dataTypeToString(d.DataType, env)
	default:
		sig = d.Name()
	}

	return "```zkc\n" + sig + "\n```"
}

// formatFunctionDecl formats a function declaration as a single-line
// signature, e.g. "fn f<mem>(x: u16, y: u32) -> (r: u16)".
func formatFunctionDecl(fn *decl.ResolvedFunction, env data.ResolvedEnvironment) string {
	var sb strings.Builder

	sb.WriteString("fn ")
	sb.WriteString(fn.Name())

	// Memory effects, e.g. <ram, rom>
	if len(fn.Effects) > 0 {
		sb.WriteString("<")

		for i, e := range fn.Effects {
			if i > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(e.Name)
		}

		sb.WriteString(">")
	}

	// Parameters
	sb.WriteString("(")

	for i, v := range fn.Inputs() {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(v.Name)
		sb.WriteString(": ")
		sb.WriteString(dataTypeToString(v.DataType, env))
	}

	sb.WriteString(")")

	// Return values
	if outs := fn.Outputs(); len(outs) > 0 {
		sb.WriteString(" -> (")

		for i, v := range outs {
			if i > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(v.Name)
			sb.WriteString(": ")
			sb.WriteString(dataTypeToString(v.DataType, env))
		}

		sb.WriteString(")")
	}

	return sb.String()
}

// extractPrecedingComments walks backward from spanStart in srcFile and returns
// any // comment lines directly preceding that position (no blank line between
// the comments and the span). Lines are returned joined by "\n" with the "//"
// prefix and optional following space stripped. Returns "" when none are found.
func extractPrecedingComments(srcFile source.File, spanStart int) string {
	contents := srcFile.Contents()

	// Find the start of the line that contains spanStart.
	lineStart := spanStart
	for lineStart > 0 && contents[lineStart-1] != '\n' {
		lineStart--
	}

	// Walk backward one line at a time from just before lineStart.
	var commentLines []string

	pos := lineStart // points just after the '\n' ending the previous line

	for pos > 0 {
		// pos-1 is the '\n' that terminates the previous line.
		end := pos - 1 // the '\n' character itself — not part of line content

		// Find where that line starts.
		start := end
		for start > 0 && contents[start-1] != '\n' {
			start--
		}

		line := strings.TrimSpace(string(contents[start:end]))

		if line == "" {
			// Blank line — stop collecting.
			break
		}

		if !strings.HasPrefix(line, "//") {
			break
		}

		text := strings.TrimPrefix(line, "//")
		text = strings.TrimPrefix(text, " ")
		commentLines = append(commentLines, text)
		pos = start
	}

	if len(commentLines) == 0 {
		return ""
	}

	// We collected lines newest-first; reverse to chronological order.
	for i, j := 0, len(commentLines)-1; i < j; i, j = i+1, j-1 {
		commentLines[i], commentLines[j] = commentLines[j], commentLines[i]
	}

	return strings.Join(commentLines, "\n")
}

// formatMemoryDecl formats a memory declaration as a single-line signature,
// e.g. "pub input rom(addr: u16) -> (data: u32)".
func formatMemoryDecl(m *decl.ResolvedMemory, env data.ResolvedEnvironment) string {
	var sb strings.Builder

	// Public visibility prefix
	switch m.Kind {
	case decl.PUBLIC_READ_ONLY_MEMORY, decl.PUBLIC_WRITE_ONCE_MEMORY, decl.PUBLIC_STATIC_MEMORY:
		sb.WriteString("pub ")
	}

	// Memory kind keyword
	switch m.Kind {
	case decl.PUBLIC_READ_ONLY_MEMORY, decl.PRIVATE_READ_ONLY_MEMORY:
		sb.WriteString("input ")
	case decl.PUBLIC_WRITE_ONCE_MEMORY, decl.PRIVATE_WRITE_ONCE_MEMORY:
		sb.WriteString("output ")
	case decl.PUBLIC_STATIC_MEMORY, decl.PRIVATE_STATIC_MEMORY:
		sb.WriteString("static ")
	case decl.RANDOM_ACCESS_MEMORY:
		sb.WriteString("memory ")
	}

	sb.WriteString(m.Name())

	// Address bus
	sb.WriteString("(")

	for i, v := range m.Address {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(v.Name)
		sb.WriteString(": ")
		sb.WriteString(dataTypeToString(v.DataType, env))
	}

	sb.WriteString(")")

	// Data bus
	sb.WriteString(" -> (")

	for i, v := range m.Data {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(v.Name)
		sb.WriteString(": ")
		sb.WriteString(dataTypeToString(v.DataType, env))
	}

	sb.WriteString(")")

	return sb.String()
}
