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
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
	"go.lsp.dev/protocol"
	lspuri "go.lsp.dev/uri"
)

// ReferencesFor returns the locations of every use of the top-level
// declaration named by the identifier under the cursor at pos.  When
// includeDecl is true, the declaration's own site is included in the
// results.  Returns nil when no identifier is under the cursor or when the
// identifier does not name a top-level declaration.
func ReferencesFor(
	uri protocol.URI, text string, pos protocol.Position, includeDecl bool,
	program ast.Program, srcmaps source.Maps[any],
) ([]protocol.Location, error) {
	srcfile := source.NewSourceFile(uri.Filename(), []byte(text))

	// Convert LSP cursor position to a rune offset in the source file.
	offset := posToOffset(*srcfile, pos)

	// Lex the document to find the identifier token under the cursor.
	tokens := parser.Lex(*srcfile, false, false)

	tok, _, ok := tokenAtOffset(tokens, offset)
	if !ok || tok.Kind != parser.IDENTIFIER {
		return nil, nil
	}

	contents := srcfile.Contents()
	name := string(contents[tok.Span.Start():tok.Span.End()])

	// Locate the matching top-level declaration so we can match references by
	// declaration index rather than by name. This lets us be robust to names
	// that happen to coincide with local variables.
	var (
		targetIdx   uint
		targetDecl  decl.Resolved
		targetFound bool
	)

	for i, d := range program.Components() {
		if d.Name() == name {
			targetIdx = uint(i)
			targetDecl = d
			targetFound = true

			break
		}
	}

	if !targetFound {
		return nil, nil
	}

	// Walk every declaration collecting AST nodes that reference the target.
	refs := collectProgramRefs(program, targetIdx)

	// Convert collected nodes to LSP locations, optionally prepending the
	// declaration site itself.
	var locations []protocol.Location

	if includeDecl {
		if defFile, span, found := srcmaps.Lookup(targetDecl); found {
			locations = append(locations, locationOf(defFile, narrowToName(defFile, span, name)))
		}
	}

	for _, n := range refs {
		if file, span, found := srcmaps.Lookup(n); found {
			locations = append(locations, locationOf(file, narrowToName(file, span, name)))
		}
	}

	return locations, nil
}

// locationOf converts a source file and span to an LSP Location.
func locationOf(file source.File, span source.Span) protocol.Location {
	return protocol.Location{
		URI:   lspuri.File(file.Filename()),
		Range: spanToRange(file, span),
	}
}

// narrowToName returns the span of the first IDENTIFIER token within the
// supplied span whose text equals name.  When no such token is found the
// original span is returned unchanged. This gives editors a tight highlight
// over just the symbol name rather than the enclosing call or memory access.
func narrowToName(file source.File, span source.Span, name string) source.Span {
	contents := file.Contents()
	if span.End() > len(contents) {
		return span
	}

	sub := source.NewSourceFile(file.Filename(), []byte(string(contents[span.Start():span.End()])))
	tokens := parser.Lex(*sub, false, false)

	for _, t := range tokens {
		if t.Kind != parser.IDENTIFIER {
			continue
		}

		if string(contents[span.Start()+t.Span.Start():span.Start()+t.Span.End()]) == name {
			return source.NewSpan(span.Start()+t.Span.Start(), span.Start()+t.Span.End())
		}
	}

	return span
}

// collectProgramRefs walks every declaration in the program and returns
// the AST nodes which reference the declaration at index target. Used by
// both find-references and rename to discover the use sites of a top-level
// declaration.
func collectProgramRefs(program ast.Program, target uint) []any {
	var refs []any

	for _, d := range program.Components() {
		switch d := d.(type) {
		case *decl.ResolvedFunction:
			collectFunctionRefs(d, target, &refs)
		case *decl.ResolvedConstant:
			collectExprRefs(d.ConstExpr, target, &refs)
		case *decl.ResolvedMemory:
			for _, c := range d.Contents {
				collectExprRefs(c, target, &refs)
			}
		}
	}

	return refs
}

// collectFunctionRefs gathers references to the target declaration appearing
// in a function: its memory effects and any usage inside its body.
func collectFunctionRefs(fn *decl.ResolvedFunction, target uint, out *[]any) {
	for _, e := range fn.Effects {
		if e != nil && e.Index == target {
			*out = append(*out, e)
		}
	}

	for _, s := range fn.Code {
		collectStmtRefs(s, target, out)
	}
}

// collectStmtRefs walks a statement and any nested statements/expressions for
// references to the target declaration.
func collectStmtRefs(s stmt.Resolved, target uint, out *[]any) {
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		for _, lv := range s.Targets {
			collectLValRefs(lv, target, out)
		}

		collectExprRefs(s.Source, target, out)
	case *stmt.For[symbol.Resolved]:
		if s.Init != nil {
			collectStmtRefs(s.Init, target, out)
		}

		collectExprRefs(s.Cond, target, out)

		if s.Post != nil {
			collectStmtRefs(s.Post, target, out)
		}

		for _, b := range s.Body {
			collectStmtRefs(b, target, out)
		}
	case *stmt.IfElse[symbol.Resolved]:
		collectExprRefs(s.Cond, target, out)

		for _, b := range s.TrueBranch {
			collectStmtRefs(b, target, out)
		}

		for _, b := range s.FalseBranch {
			collectStmtRefs(b, target, out)
		}
	case *stmt.IfGoto[symbol.Resolved]:
		if cmp, ok := s.Cond.(*expr.Cmp[symbol.Resolved]); ok {
			collectExprRefs(cmp, target, out)
		}
	case *stmt.Printf[symbol.Resolved]:
		for _, a := range s.Arguments {
			collectExprRefs(a, target, out)
		}
	case *stmt.VarDecl[symbol.Resolved]:
		if s.Init.HasValue() {
			collectExprRefs(s.Init.Unwrap(), target, out)
		}
	case *stmt.While[symbol.Resolved]:
		collectExprRefs(s.Cond, target, out)

		for _, b := range s.Body {
			collectStmtRefs(b, target, out)
		}
	}
}

// collectLValRefs walks an lvalue for references to the target declaration.
// MemAccess lvalues both reference the memory by name and contain index
// expressions which may themselves reference further declarations.
func collectLValRefs(lv lval.Resolved, target uint, out *[]any) {
	if mem, ok := lv.(*lval.MemAccess[symbol.Resolved]); ok {
		if mem.Name.Index == target {
			*out = append(*out, mem)
		}

		for _, a := range mem.Args {
			collectExprRefs(a, target, out)
		}
	}
}

// collectExprRefs walks an expression and any sub-expressions for references
// to the target declaration. ExternAccess is the only leaf form that names a
// top-level declaration; the remaining cases simply recurse into their
// operands.
func collectExprRefs(e expr.Resolved, target uint, out *[]any) {
	if e == nil {
		return
	}

	switch e := e.(type) {
	case *expr.ExternAccess[symbol.Resolved]:
		if e.Name.Index == target {
			*out = append(*out, e)
		}

		for _, a := range e.Args {
			collectExprRefs(a, target, out)
		}
	case *expr.Add[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.BitwiseAnd[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.BitwiseOr[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.BitwiseNot[symbol.Resolved]:
		collectExprRefs(e.Expr, target, out)
	case *expr.Cast[symbol.Resolved]:
		collectExprRefs(e.Expr, target, out)
	case *expr.Cmp[symbol.Resolved]:
		collectExprRefs(e.Left, target, out)
		collectExprRefs(e.Right, target, out)
	case *expr.Concat[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Div[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.LogicalAnd[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.LogicalNot[symbol.Resolved]:
		collectExprRefs(e.Expr, target, out)
	case *expr.LogicalOr[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Mul[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Rem[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Shl[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Shr[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Sub[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	case *expr.Ternary[symbol.Resolved]:
		collectExprRefs(e.Cond, target, out)
		collectExprRefs(e.IfTrue, target, out)
		collectExprRefs(e.IfFalse, target, out)
	case *expr.Xor[symbol.Resolved]:
		collectExprListRefs(e.Exprs, target, out)
	}
}

func collectExprListRefs(exprs []expr.Resolved, target uint, out *[]any) {
	for _, e := range exprs {
		collectExprRefs(e, target, out)
	}
}
