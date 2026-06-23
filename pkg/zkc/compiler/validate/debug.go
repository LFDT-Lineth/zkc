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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// DebugFunctions checks that every function marked with the #[debug]
// annotation is safe to elide.  Calls to debug functions are dropped entirely
// during code generation in quiet mode (like printf statements), so eliding
// them must not change the observable state of the program.  Specifically, a
// debug function must not:
//
// (1) declare any return values;
//
// (2) write to a writable memory (i.e. a RAM or write-once memory);
//
// (3) call a function which is not itself marked #[debug].
//
// Observe that (3) is (for now) deliberately conservative: it rejects calls to
// any non-debug function, even one which cannot access memory.  Combined with
// (1) and (2), this ensures nothing reachable from a debug call can affect the
// observable state of the program.
func DebugFunctions(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		errors  []source.SyntaxError
		checker = debugChecker{program, srcmaps}
	)
	//
	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok && slices.Contains(fn.Annotations(), "debug") {
			errors = append(errors, checker.checkFunction(fn)...)
		}
	}
	//
	return errors
}

// debugChecker embodies information needed for checking debug functions within
// a given program.
type debugChecker struct {
	program ast.Program
	srcmaps source.Maps[any]
}

func (p *debugChecker) checkFunction(fn *decl.ResolvedFunction) []source.SyntaxError {
	var errors []source.SyntaxError
	// Check (1): no return values.
	if fn.NumOutputs > 0 {
		errors = append(errors, p.srcmaps.SyntaxErrors(fn, "debug function cannot return values")...)
	}
	// Check the function body.  At this point, the body has been flattened
	// into the flat if-goto form.
	for _, s := range fn.Code {
		switch s := s.(type) {
		case *stmt.Assign[symbol.Resolved]:
			errors = append(errors, p.checkAssign(s)...)
		case *stmt.Fail[symbol.Resolved]:
			errors = append(errors, p.checkCallees(s, *set.UnionAnySortedSets(s.Arguments, externUsesOf))...)
		case *stmt.IfGoto[symbol.Resolved]:
			errors = append(errors, p.checkCallees(s, s.Cond.ExternUses())...)
		case *stmt.Printf[symbol.Resolved]:
			errors = append(errors, p.checkCallees(s, *set.UnionAnySortedSets(s.Arguments, externUsesOf))...)
		}
	}
	//
	return errors
}

func (p *debugChecker) checkAssign(s *stmt.Assign[symbol.Resolved]) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		exprs  = []expr.Resolved{s.Source}
	)
	// Check (2): no writes to writable memories.
	for _, target := range s.Targets {
		if target, ok := target.(*lval.MemAccess[symbol.Resolved]); ok && !target.Name.IsUnknown() {
			if mem, ok := p.program.Component(target.Name.Index).(*decl.ResolvedMemory); ok && mem.IsWriteable() {
				errors = append(errors, p.srcmaps.SyntaxErrors(target, "debug function cannot write memory")...)
			}
			// Address expressions may themselves contain calls.
			exprs = append(exprs, target.Args...)
		}
	}
	//
	return append(errors, p.checkCallees(s, *set.UnionAnySortedSets(exprs, externUsesOf))...)
}

// checkCallees applies check (3) to every function called from the given
// statement: a callee must itself be marked #[debug].  Errors are reported
// against the enclosing statement.
func (p *debugChecker) checkCallees(s stmt.Resolved, uses set.AnySortedSet[symbol.Resolved],
) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, use := range uses {
		if use.IsUnknown() || !use.IsFunction() {
			continue
		}
		//
		fn, ok := p.program.Component(use.Index).(*decl.ResolvedFunction)
		//
		if ok && !slices.Contains(fn.Annotations(), "debug") {
			msg := fmt.Sprintf("debug function cannot call non-debug function \"%s\"", fn.Name())
			errors = append(errors, p.srcmaps.SyntaxErrors(s, msg)...)
		}
	}
	//
	return errors
}

func externUsesOf(e expr.Resolved) *set.AnySortedSet[symbol.Resolved] {
	uses := e.ExternUses()
	//
	return &uses
}
