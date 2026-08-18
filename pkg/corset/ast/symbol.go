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
package ast

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// Symbol represents a variable access within a declaration.  Initially, such
// the proper interpretation of such accesses is unclear and it is only later
// when we can distinguish them (e.g. whether its a column access, a constant
// access, etc).
type Symbol interface {
	Node
	// Path returns the given path of this symbol.
	Path() *file.Path
	// Checks whether this symbol has been resolved already, or not.
	IsResolved() bool
	// Get binding associated with this interface.  This will panic if this
	// symbol is not yet resolved.
	Binding() Binding
	// Resolve this symbol by associating it with the binding associated with
	// the definition of the symbol to which this refers.
	Resolve(Binding) bool
}

// TypedSymbol is an extended form of symbol which contains additional
// information about a given column access.
type TypedSymbol interface {
	Symbol
	// Type returns the type associated with this symbol.  If the type cannot be
	// determined, then nil is returned.
	Type() Type
}

// ContextOfSymbol returns the context of a given symbol.  For a column access,
// this is the context of the enclosing (non-virtual) module.  Anything else
// (e.g. a constant) has no context, and the void context is returned.
func ContextOfSymbol(symbol TypedSymbol) Context {
	if binding, ok := symbol.Binding().(*ColumnBinding); ok {
		return binding.Context()
	}
	// Anything else has no context.
	return VoidContext()
}

// ContextOfSymbols returns the context for a set of zero or more symbols, along
// with the index of the first symbol whose context conflicts with those seen
// before it (or the number of symbols, if there is no conflict).  Observe that,
// if the symbols have no context (e.g. they are all constants) then the void
// context is returned.
func ContextOfSymbols(symbols ...TypedSymbol) (Context, uint) {
	context := VoidContext()
	//
	for i, s := range symbols {
		context = context.Join(ContextOfSymbol(s))
		//
		if context.IsConflicted() {
			return context, uint(i)
		}
	}
	//
	return context, uint(len(symbols))
}

// SymbolDefinition represents a declaration (or part thereof) which defines a
// particular symbol.  For example, "defcolumns" will define one or more symbols
// representing columns, etc.
type SymbolDefinition interface {
	Node
	// Name returns the (unqualified) name of this symbol.  For example, "X" for
	// a column X defined in a module m1.
	Name() string
	// Path returns the qualified name (i.e. absolute path) of this symbol.  For
	// example, "m1.X" for a column X defined in module m1.
	Path() *file.Path
	// Allocated binding for the symbol which may or may not be finalised.
	Binding() Binding
}
