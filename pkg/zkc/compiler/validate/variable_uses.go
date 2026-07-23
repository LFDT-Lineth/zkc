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
package validate

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// VariableDeclarations records, for each function, the source node (i.e. the
// "var" declaration) which introduced each of its local variables.  This must
// be captured before block-level constructs are flattened away, since
// flattening discards the declaration nodes; it allows an unused-variable error
// to be reported against the original declaration rather than the enclosing
// function.
type VariableDeclarations = map[*decl.ResolvedFunction]map[variable.Id]stmt.Resolved

// CollectVariableDeclarations records the declaration node for every local
// variable in the program.  It must be called before the program is flattened,
// as flattening discards the (block-structured) declaration nodes.
func CollectVariableDeclarations(program ast.Program) VariableDeclarations {
	var decls = make(VariableDeclarations)
	//
	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok {
			var m = make(map[variable.Id]stmt.Resolved)
			//
			collectDeclarations(fn.Code, m)
			//
			decls[fn] = m
		}
	}
	//
	return decls
}

// collectDeclarations records the declaration node for every variable
// introduced by a "var" declaration within the given statements, descending
// into any block-level constructs.
func collectDeclarations(stmts []stmt.Resolved, decls map[variable.Id]stmt.Resolved) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *stmt.VarDecl[symbol.Resolved]:
			for _, id := range s.Variables {
				decls[id] = s
			}
		case *stmt.IfElse[symbol.Resolved]:
			collectDeclarations(s.TrueBranch, decls)
			collectDeclarations(s.FalseBranch, decls)
		case *stmt.While[symbol.Resolved]:
			collectDeclarations(s.Body, decls)
		case *stmt.For[symbol.Resolved]:
			collectDeclarations([]stmt.Resolved{s.Init, s.Post}, decls)
			collectDeclarations(s.Body, decls)
		case *stmt.Switch[symbol.Resolved]:
			for _, b := range s.Branches {
				collectDeclarations(b.Body, decls)
			}
		}
	}
}

// VariableUses performs a check that every variable in a program is used at least once and, if not, reports a
// syntax error that the variable is unused.  For now, a variable which is only assigned (i.e. written but never
// read) is still considered used.  The declaration nodes gathered by
// CollectVariableDeclarations (before flattening) are used to anchor each error
// on the offending "var" declaration.
func VariableUses(program ast.Program, _ field.Config, srcmaps source.Maps[any],
	decls VariableDeclarations) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok {
			errors = append(errors, validateFunctionUses(fn, decls[fn], srcmaps)...)
		}
	}
	//
	return errors
}

// validateFunctionUses checks that every variable declared within a given
// function is used at least once.  A variable is considered used if it is read
// or assigned by some instruction in the function body.
func validateFunctionUses(fn *decl.ResolvedFunction, decls map[variable.Id]stmt.Resolved,
	srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		used   bit.Set
	)
	// Record every variable which is read or written by some instruction.
	for _, insn := range fn.Code {
		if insn == nil {
			continue
		}
		//
		for _, r := range insn.Uses() {
			used.Insert(r)
		}
		//
		for _, r := range insn.Definitions() {
			used.Insert(r)
		}
	}
	// Report any local variable which was never used.  Parameters and returns
	// form the function's interface (they are implicitly used by the calling
	// convention), whilst an unassigned return is caught separately as a
	// control-flow error; hence, neither is considered here.
	for i, v := range fn.Variables {
		if v.IsLocal() && !used.Contains(uint(i)) {
			msg := fmt.Sprintf("unused variable %s", v.Name)
			errors = append(errors, unusedError(fn, decls[uint(i)], msg, srcmaps))
		}
	}
	//
	return errors
}

// unusedError constructs the syntax error for an unused variable.  It is
// anchored on the variable's "var" declaration where available, falling back to
// the enclosing function otherwise.
func unusedError(fn *decl.ResolvedFunction, declaration stmt.Resolved, msg string,
	srcmaps source.Maps[any]) source.SyntaxError {
	// Prefer the declaration node, provided it has a known source location.
	if declaration != nil {
		if _, _, ok := srcmaps.Lookup(declaration); ok {
			return *srcmaps.SyntaxError(declaration, msg)
		}
	}
	// Otherwise, fall back to the enclosing function.
	return *srcmaps.SyntaxError(fn, msg)
}
