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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// NON_FUNCTION is used to signal a symbol which does not represent a function.
var NON_FUNCTION util.Option[uint] = util.None[uint]()

// Symbol represents a variable or function access within a declaration.
// Initially, such the proper interpretation of such accesses is unclear and it
// is only later when we can distinguish them (e.g. whether its a column access,
// a constant access, etc).
type Symbol interface {
	Node
	// Path returns the given path of this symbol.
	Path() *file.Path
	// Indicates whether or not this is a function and, if so, what arity (i.e.
	// how many arguments) the function has.
	Arity() util.Option[uint]
	// Checks whether this symbol has been resolved already, or not.
	IsResolved() bool
	// Get binding associated with this interface.  This will panic if this
	// symbol is not yet resolved.
	Binding() Binding
	// Resolve this symbol by associating it with the binding associated with
	// the definition of the symbol to which this refers.  Observe that
	// resolution can fail if we cannot bind the symbol to the given binding
	// (e.g. a function binding was provided, but we're expecting a column
	// binding).
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
	// Indicates whether or not this is a function and, if so, what arity (i.e.
	// how many arguments) the function has.
	Arity() util.Option[uint]
	// Allocated binding for the symbol which may or may not be finalised.
	Binding() Binding
}

// FunctionName represents a name used in a position where it can only be
// resolved as a function.
type FunctionName = Name[*DefunBinding]

// NewFunctionName construct a new column name which is (initially) unresolved.
func NewFunctionName(path file.Path, binding *DefunBinding) *FunctionName {
	arity := uint(len(binding.ParamTypes))
	return &FunctionName{path, util.Some(arity), binding, true}
}

// PerspectiveName represents a name used in a position where it can only be
// resolved as a perspective.
type PerspectiveName = Name[*PerspectiveBinding]

// NewPerspectiveName construct a new column name which is (initially) unresolved.
func NewPerspectiveName(path file.Path, binding *PerspectiveBinding) *PerspectiveName {
	return &PerspectiveName{path, NON_FUNCTION, binding, true}
}

// Name represents a name within some syntactic item.  Essentially this wraps a
// string and provides a mechanism for it to be associated with source line
// information.
type Name[T Binding] struct {
	// Name of symbol
	path file.Path
	// Indicates whether represents function or something else.
	arity util.Option[uint]
	// Binding constructed for symbol.
	binding T
	// Indicates whether resolved.
	resolved bool
}

// NewUnboundName construct a new name which is (initially) unresolved.
func NewUnboundName[T Binding](path file.Path, arity util.Option[uint]) *Name[T] {
	// Default value for type T
	var empty T
	// Construct the name
	return &Name[T]{path, arity, empty, false}
}

// NewBoundName construct a new name which is already unresolved.
func NewBoundName[T Binding](path file.Path, arity util.Option[uint], binding T) *Name[T] {
	// Construct the name
	return &Name[T]{path, arity, binding, false}
}

// Name returns the (unqualified) name of this symbol.  For example, "X" for
// a column X defined in a module m1.
func (e *Name[T]) Name() string {
	return e.path.Tail()
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (e *Name[T]) Path() *file.Path {
	return &e.path
}

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (e *Name[T]) Arity() util.Option[uint] {
	return e.arity
}

// IsResolved checks whether this symbol has been resolved already, or not.
func (e *Name[T]) IsResolved() bool {
	return e.resolved
}

// Binding gets binding associated with this interface.  This will panic if this
// symbol is not yet resolved.
func (e *Name[T]) Binding() Binding {
	if !e.resolved {
		panic("name not yet resolved")
	}
	//
	return e.binding
}

// InnerBinding returns the concrete binding type used within this name.
func (e *Name[T]) InnerBinding() T {
	if !e.resolved {
		panic("name not yet resolved")
	}
	//
	return e.binding
}

// Resolve this symbol by associating it with the binding associated with
// the definition of the symbol to which this refers.
func (e *Name[T]) Resolve(binding Binding) bool {
	var ok bool
	//
	if e.resolved {
		panic("name already resolved")
	}
	// Attempt to assign binding.
	e.binding, ok = binding.(T)
	e.resolved = ok
	//
	return ok
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (e *Name[T]) Lisp() sexp.SExp {
	return sexp.NewSymbol(e.path.String())
}
