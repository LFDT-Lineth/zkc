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
	"fmt"

	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
	"go.lsp.dev/protocol"
	lspuri "go.lsp.dev/uri"
)

// PrepareRenameFor returns the range of the identifier under the cursor when
// it names something the server knows how to rename: a top-level declaration
// (function, constant, memory, type alias) or a local variable in the
// enclosing function. Returns nil for any other token, signalling the editor
// not to start a rename session.
func PrepareRenameFor(
	uri protocol.URI, text string, pos protocol.Position,
	program ast.Program, srcmaps source.Maps[any],
) (*protocol.Range, error) {
	srcfile := source.NewSourceFile(uri.Filename(), []byte(text))
	offset := posToOffset(*srcfile, pos)
	tokens := parser.Lex(*srcfile, false, false)

	tok, _, ok := tokenAtOffset(tokens, offset)
	if !ok || tok.Kind != parser.IDENTIFIER {
		return nil, nil
	}

	contents := srcfile.Contents()
	name := string(contents[tok.Span.Start():tok.Span.End()])

	if isTopLevelDecl(name, program) || isLocalVariable(name, offset, *srcfile, program, srcmaps) {
		rng := spanToRange(*srcfile, tok.Span)
		return &rng, nil
	}

	return nil, nil
}

// RenameFor produces a workspace edit which renames the identifier under the
// cursor to newName.  Top-level declarations are renamed across every file in
// the program; local variables (parameters, returns, and `var` bindings) are
// renamed only within the enclosing function.  Returns nil when no rename is
// possible — the cursor is not on a renameable identifier, newName is not a
// valid identifier, or newName equals the existing name.
func RenameFor(
	uri protocol.URI, text string, pos protocol.Position, newName string,
	program ast.Program, srcmaps source.Maps[any],
) (*protocol.WorkspaceEdit, error) {
	if !parser.IsValidIdentifier(newName) {
		return nil, fmt.Errorf("invalid identifier: %q", newName)
	}

	srcfile := source.NewSourceFile(uri.Filename(), []byte(text))
	offset := posToOffset(*srcfile, pos)
	tokens := parser.Lex(*srcfile, false, false)

	tok, idx, ok := tokenAtOffset(tokens, offset)
	if !ok || tok.Kind != parser.IDENTIFIER {
		return nil, nil
	}

	contents := srcfile.Contents()
	oldName := string(contents[tok.Span.Start():tok.Span.End()])

	if oldName == newName {
		return nil, nil
	}

	// An identifier directly followed by `(` or `[` is always an extern
	// access — a function call or memory read/write — and so refers to a
	// top-level declaration regardless of any same-named local in scope.
	externAccess := isExternAccess(tokens, idx)

	// Inside a function, a local of the same name shadows any top-level
	// declaration — so prefer the local unless we're on an extern access.
	if !externAccess {
		if fn, fnFile, fnSpan := enclosingFunction(offset, *srcfile, program, srcmaps); fn != nil {
			if hasLocal(fn, oldName) {
				return renameLocal(fnFile, fnSpan, oldName, newName), nil
			}
		}
	}

	// Otherwise the identifier names a top-level declaration.
	for i, d := range program.Components() {
		if d.Name() == oldName {
			return renameTopLevel(d, uint(i), oldName, newName, program, srcmaps), nil
		}
	}

	return nil, nil
}

// renameTopLevel constructs a workspace edit which renames every reference to
// the given top-level declaration across all files in the program, plus the
// declaration's own name.
func renameTopLevel(
	target decl.Resolved, targetIdx uint, oldName, newName string,
	program ast.Program, srcmaps source.Maps[any],
) *protocol.WorkspaceEdit {
	// Collect AST nodes that reference the target — the declaration site
	// itself is added separately below.
	refs := collectProgramRefs(program, targetIdx)

	changes := make(map[protocol.DocumentURI][]protocol.TextEdit)

	addEdit := func(file source.File, span source.Span) {
		uri := lspuri.File(file.Filename())
		changes[uri] = append(changes[uri], protocol.TextEdit{
			Range:   spanToRange(file, span),
			NewText: newName,
		})
	}

	if defFile, span, found := srcmaps.Lookup(target); found {
		addEdit(defFile, narrowToName(defFile, span, oldName))
	}

	for _, n := range refs {
		if file, span, found := srcmaps.Lookup(n); found {
			addEdit(file, narrowToName(file, span, oldName))
		}
	}

	if len(changes) == 0 {
		return nil
	}

	return &protocol.WorkspaceEdit{Changes: changes}
}

// renameLocal constructs a workspace edit which renames every occurrence of
// oldName within the supplied function span. Identifier tokens immediately
// followed by `(` or `[` are skipped, since those are extern accesses (calls
// and memory reads/writes) which cannot refer to a local.
func renameLocal(
	fnFile source.File, fnSpan source.Span, oldName, newName string,
) *protocol.WorkspaceEdit {
	tokens := parser.Lex(fnFile, false, false)

	var edits []protocol.TextEdit

	for i, t := range tokens {
		if t.Kind != parser.IDENTIFIER {
			continue
		}

		if t.Span.Start() < fnSpan.Start() || t.Span.End() > fnSpan.End() {
			continue
		}

		text := string(fnFile.Contents()[t.Span.Start():t.Span.End()])
		if text != oldName {
			continue
		}

		// Skip extern accesses — name(...) and name[...] are always
		// resolved against top-level declarations even when a local of
		// the same name is in scope.
		if isExternAccess(tokens, i) {
			continue
		}

		edits = append(edits, protocol.TextEdit{
			Range:   spanToRange(fnFile, t.Span),
			NewText: newName,
		})
	}

	if len(edits) == 0 {
		return nil
	}

	uri := lspuri.File(fnFile.Filename())

	return &protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			uri: edits,
		},
	}
}

// isTopLevelDecl reports whether name matches a top-level declaration in the
// program.
func isTopLevelDecl(name string, program ast.Program) bool {
	for _, d := range program.Components() {
		if d.Name() == name {
			return true
		}
	}

	return false
}

// isLocalVariable reports whether name is a local variable (parameter,
// return, or `var` binding) in the function whose source span contains
// offset.
func isLocalVariable(
	name string, offset int, srcfile source.File,
	program ast.Program, srcmaps source.Maps[any],
) bool {
	fn, _, _ := enclosingFunction(offset, srcfile, program, srcmaps)
	return fn != nil && hasLocal(fn, name)
}

// enclosingFunction returns the function whose source span contains offset
// in srcfile, along with the file and the span covering both its signature
// and body. The parser only records the signature in the source map, so we
// extend the recorded span by brace-matching forward to the close of the
// body. Returns nil for fn when no function encloses the cursor.
func enclosingFunction(
	offset int, srcfile source.File, program ast.Program, srcmaps source.Maps[any],
) (*decl.ResolvedFunction, source.File, source.Span) {
	for _, d := range program.Components() {
		fn, ok := d.(*decl.ResolvedFunction)
		if !ok {
			continue
		}

		fnFile, fnSpan, found := srcmaps.Lookup(d)
		if !found || fnFile.Filename() != srcfile.Filename() {
			continue
		}

		full := extendToBody(fnFile, fnSpan)

		if offset < full.Start() || offset >= full.End() {
			continue
		}

		return fn, fnFile, full
	}

	return nil, source.File{}, source.Span{}
}

// extendToBody returns a span that covers both the signature span passed in
// and the function body's `{ ... }` block which immediately follows it. When
// no balanced body block can be located the signature span is returned
// unchanged.
func extendToBody(file source.File, signature source.Span) source.Span {
	contents := file.Contents()

	if signature.End() > len(contents) {
		return signature
	}

	sub := source.NewSourceFile(file.Filename(), []byte(string(contents[signature.End():])))
	tokens := parser.Lex(*sub, false, false)

	var depth int

	for _, t := range tokens {
		switch t.Kind {
		case parser.LCURLY:
			depth++
		case parser.RCURLY:
			depth--

			if depth == 0 {
				return source.NewSpan(signature.Start(), signature.End()+t.Span.End())
			}
		}
	}

	return signature
}

// hasLocal reports whether the function declares a local variable
// (parameter, return, or `var` binding) named name.
func hasLocal(fn *decl.ResolvedFunction, name string) bool {
	for _, v := range fn.Variables {
		if v.Name == name {
			return true
		}
	}

	return false
}

// isExternAccess reports whether the token at index i in tokens is followed
// by `(` or `[` — the syntactic shapes for a function call and a memory
// read/write respectively. Such a use always resolves to a top-level
// declaration even when a same-named local is in scope.
func isExternAccess(tokens []lex.Token, i int) bool {
	if i < 0 || i+1 >= len(tokens) {
		return false
	}

	next := tokens[i+1].Kind

	return next == parser.LBRACE || next == parser.LSQUARE
}
