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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/decl"
	"go.lsp.dev/protocol"
)

// DocumentSymbolsFor returns a protocol.DocumentSymbol for each top-level
// declaration defined in the file identified by uri.  Declarations from
// included files are filtered out by comparing the source file recorded in
// srcmaps against uri.  The caller supplies an already-compiled program and
// its source maps (typically taken from an IncrementalCompiler), so this
// function does no parsing or compilation of its own.
func DocumentSymbolsFor(
	uri protocol.URI, program ast.Program, srcmaps source.Maps[any],
) ([]interface{}, error) {
	filename := uri.Filename()
	env := program.Environment()

	var symbols []interface{}

	for _, d := range program.Components() {
		srcFile, span, ok := srcmaps.Lookup(d)
		if !ok || srcFile.Filename() != filename {
			continue
		}

		rng := spanToRange(srcFile, span)

		symbols = append(symbols, protocol.DocumentSymbol{
			Name:           d.Name(),
			Detail:         declDetail(d, env),
			Kind:           declSymbolKind(d),
			Range:          rng,
			SelectionRange: rng,
		})
	}

	return symbols, nil
}

// declSymbolKind maps a ZkC declaration to the closest LSP SymbolKind.
func declSymbolKind(d decl.Resolved) protocol.SymbolKind {
	switch d.(type) {
	case *decl.ResolvedFunction:
		return protocol.SymbolKindFunction
	case *decl.ResolvedConstant:
		return protocol.SymbolKindConstant
	case *decl.ResolvedMemory:
		return protocol.SymbolKindVariable
	case *decl.ResolvedTypeAlias:
		return protocol.SymbolKindTypeParameter
	default:
		return protocol.SymbolKindNull
	}
}

// declDetail returns a short, single-line description of the declaration shown
// beside the name in the editor outline (the Detail field of DocumentSymbol).
func declDetail(d decl.Resolved, env data.ResolvedEnvironment) string {
	switch d := d.(type) {
	case *decl.ResolvedFunction:
		return funcDetail(d, env)
	case *decl.ResolvedConstant:
		return ": " + dataTypeToString(d.DataType, env)
	case *decl.ResolvedMemory:
		return memDetail(d, env)
	case *decl.ResolvedTypeAlias:
		return "= " + dataTypeToString(d.DataType, env)
	default:
		return ""
	}
}

// funcDetail builds the compact parameter/return summary shown in the outline
// for a function, e.g. "(x: u16, y: u32) -> (r: u16)".
func funcDetail(fn *decl.ResolvedFunction, env data.ResolvedEnvironment) string {
	s := "("

	for i, v := range fn.Inputs() {
		if i > 0 {
			s += ", "
		}

		s += v.Name + ": " + dataTypeToString(v.DataType, env)
	}

	s += ")"

	if outs := fn.Outputs(); len(outs) > 0 {
		s += " -> ("

		for i, v := range outs {
			if i > 0 {
				s += ", "
			}

			s += v.Name + ": " + dataTypeToString(v.DataType, env)
		}

		s += ")"
	}

	return s
}

// memDetail builds the compact signature shown in the outline for a memory
// declaration, e.g. "input (addr: u16) -> (data: u32)".
func memDetail(m *decl.ResolvedMemory, env data.ResolvedEnvironment) string {
	var kind string

	switch m.Kind {
	case decl.PUBLIC_READ_ONLY_MEMORY, decl.PRIVATE_READ_ONLY_MEMORY:
		kind = "input"
	case decl.PUBLIC_WRITE_ONCE_MEMORY, decl.PRIVATE_WRITE_ONCE_MEMORY:
		kind = "output"
	case decl.PUBLIC_STATIC_MEMORY, decl.PRIVATE_STATIC_MEMORY:
		kind = "static"
	case decl.RANDOM_ACCESS_MEMORY:
		kind = "memory"
	}

	s := kind + " ("

	for i, v := range m.Address {
		if i > 0 {
			s += ", "
		}

		s += v.Name + ": " + dataTypeToString(v.DataType, env)
	}

	s += ") -> ("

	for i, v := range m.Data {
		if i > 0 {
			s += ", "
		}

		s += v.Name + ": " + dataTypeToString(v.DataType, env)
	}

	s += ")"

	return s
}
