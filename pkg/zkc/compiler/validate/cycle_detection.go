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
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// dependencies returns the declaration indices that the given declaration
// depends on. Returns nil for declarations that are not checked for cycles.
func dependencies(d decl.Resolved) []uint {
	switch d := d.(type) {
	case *decl.ResolvedTypeAlias:
		return typeAliasDependencies(d)
	case *decl.ResolvedConstant:
		return constantDependencies(d)
	default:
		return nil
	}
}

func typeAliasDependencies(d *decl.ResolvedTypeAlias) []uint {
	alias, ok := d.DataType.(*data.ResolvedAlias)
	if !ok {
		return nil
	}

	return []uint{alias.Name.Index}
}

func constantDependencies(d *decl.ResolvedConstant) []uint {
	symSet := d.ConstExpr.ExternUses()
	if symSet == nil {
		return nil
	}

	var deps []uint

	for sym := symSet.Iter(); sym.HasNext(); {
		s := sym.Next()
		if s.Kind == symbol.CONSTANT {
			deps = append(deps, s.Index)
		}
	}

	return deps
}

// findCycle performs DFS from start. It returns the declaration involved in a
// cycle if one is found, else nil.
func findCycle(start uint, program ast.Program, path, visited map[uint]bool) decl.Resolved {
	if visited[start] {
		// no cycle
		return nil
	}

	// (1) we check we haven't met this node on the path
	if path[start] {
		// we are in the presence of a cycle
		// we mark all the nodes on the path as visited
		for j := range path {
			visited[j] = true
		}
		// we return a cycle detection on the node
		return program.Components()[start]
	}

	// (2) we check dependencies
	d := program.Components()[start]
	deps := dependencies(d)

	// we mark the node on the path
	// we detect cycle on the dependencies if any
	path[start] = true
	for _, k := range deps {
		if d := findCycle(k, program, path, visited); d != nil {
			// we are in the presence of a cycle
			return d
		}
	}

	visited[start] = true

	return nil
}

// CycleDetection traverses the program and detects cyclic definitions in
// type constants and aliases.
func CycleDetection(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var (
		errors  []source.SyntaxError
		visited = make(map[uint]bool)
	)

	for i := range program.Components() {
		path := make(map[uint]bool) // path reset
		if d := findCycle(uint(i), program, path, visited); d != nil {
			errors = append(errors, srcmaps.SyntaxErrors(d, "cyclic definition for "+d.Name())...)
		}
	}

	return errors
}
