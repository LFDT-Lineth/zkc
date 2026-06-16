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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// InlineFunctions checks that every function marked with the #[inline]
// annotation can actually be inlined at its call sites (and subsequently
// removed).  Specifically, an inlined function must not be:
//
// (1) the entry function "main", since this must remain to boot the machine;
//
// (2) marked #[native], since native functions have no body to inline;
//
// (3) (mutually) recursive with other inlined functions, since inlining such
// functions can never terminate.
//
// Observe that (3) only rejects recursive cycles consisting entirely of
// inlined functions.  Recursion through a non-inlined function is fine, since
// the residual call to that function simply remains in the inlined body.
func InlineFunctions(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		// Indices of declarations subject to cycle checking.
		remaining []uint
	)
	// Sanity check individual inlined functions.
	for i, d := range program.Components() {
		fn, ok := d.(*decl.ResolvedFunction)
		//
		if !ok || !slices.Contains(fn.Annotations(), "inline") {
			continue
		}
		//
		switch {
		case fn.Name() == "main":
			errors = append(errors, srcmaps.SyntaxErrors(fn, "cannot inline entry function")...)
		case slices.Contains(fn.Annotations(), "native"):
			errors = append(errors, srcmaps.SyntaxErrors(fn, "cannot inline native function")...)
		default:
			remaining = append(remaining, uint(i))
		}
	}
	// Check for recursion amongst the inlined functions by repeatedly
	// discharging any function which calls no other remaining inlined function
	// (mirroring the callee-first order in which they are eventually inlined).
	// Functions left over are part of (or call into) a recursive cycle.
	for stuck := false; !stuck && len(remaining) > 0; {
		stuck = true
		//
		for i := 0; i < len(remaining); i++ {
			var fn = program.Component(remaining[i]).(*decl.ResolvedFunction)
			//
			if !callsAnyOf(fn, remaining) {
				remaining = slices.Delete(remaining, i, i+1)
				stuck = false
				i = i - 1
			}
		}
	}
	//
	for _, index := range remaining {
		var fn = program.Component(index).(*decl.ResolvedFunction)
		//
		errors = append(errors, srcmaps.SyntaxErrors(fn, "cannot inline recursive function")...)
	}
	//
	return errors
}

// callsAnyOf checks whether the body of a given function calls any of the
// given declarations.
func callsAnyOf(fn *decl.ResolvedFunction, indices []uint) bool {
	for _, s := range fn.Code {
		for _, use := range externUsesOfStmt(s) {
			if !use.IsUnknown() && use.IsFunction() && slices.Contains(indices, use.Index) {
				return true
			}
		}
	}
	//
	return false
}

// externUsesOfStmt returns all external symbols used by a given statement.
// Note that, since validation happens after flattening, block-level constructs
// (if/else, while, etc) do not arise here.
func externUsesOfStmt(s stmt.Resolved) set.AnySortedSet[symbol.Resolved] {
	switch s := s.(type) {
	case *stmt.Assign[symbol.Resolved]:
		var uses = s.Source.ExternUses()
		//
		for _, target := range s.Targets {
			targetUses := target.ExternUses()
			uses.InsertSorted(&targetUses)
		}
		//
		return uses
	case *stmt.Fail[symbol.Resolved]:
		return *set.UnionAnySortedSets(s.Arguments, externUsesOf)
	case *stmt.IfGoto[symbol.Resolved]:
		return s.Cond.ExternUses()
	case *stmt.Printf[symbol.Resolved]:
		return *set.UnionAnySortedSets(s.Arguments, externUsesOf)
	case *stmt.VarDecl[symbol.Resolved]:
		if s.Init.HasValue() {
			return s.Init.Unwrap().ExternUses()
		}
	}
	//
	return nil
}
