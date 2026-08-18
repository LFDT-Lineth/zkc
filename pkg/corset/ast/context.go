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
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
)

// Context represents the evaluation context in which an expression can be
// evaluated.  Every expression must have a single enclosing module (i.e. all
// columns accessed by the expression are in that module).  Constant
// expressions are something of an anomily here since they have no enclosing
// module.  Instead, we consider that constant expressions are evaluated in the
// empty --- or void --- context.  This is something like a bottom type which is
// contained within all other contexts.
type Context struct {
	// Identifies the module in which this evaluation context exists.  This is
	// only meaningful when the context is neither void nor conflicted.
	ModuleId string
	// Distinguishes the void, normal and conflicted states of this context.
	kind contextKind
}

// contextKind distinguishes the three states a Context can be in.
type contextKind uint8

const (
	voidKind contextKind = iota
	normalKind
	conflictedKind
)

// VoidContext returns the void (or empty) context.  This is the bottom type in
// the lattice, and is the context contained in all other contexts.  It is
// needed, for example, as the context for constant expressions.
func VoidContext() Context {
	return Context{"", voidKind}
}

// ConflictingContext represents the case where multiple different contexts have
// been joined together.  For example, when determining the context of an
// expression, the conflicting context is returned when no single context can be
// deteremed.  This value is generally considered to indicate an error.
func ConflictingContext() Context {
	return Context{"", conflictedKind}
}

// NewContext returns a (normal) context representing the given module.
func NewContext(module string) Context {
	return Context{module, normalKind}
}

// Module returns the module for this context.  Note, however, that this is
// nonsensical in the case of either the void or the conflicted  context.  In
// this cases, this method will panic.
func (p Context) Module() string {
	if !p.IsVoid() && !p.IsConflicted() {
		return p.ModuleId
	} else if p.IsVoid() {
		panic("void context has no module")
	}

	panic("conflicted context")
}

// ModuleName returns the name of the module represented by this context.
func (p Context) ModuleName() module.Name {
	return p.ModuleId
}

// IsVoid checks whether this context is the void context (or not).  This is the
// bottom element in the lattice.
func (p Context) IsVoid() bool {
	return p.kind == voidKind
}

// IsConflicted checks whether this context represents the conflicted context.
// This is the top element in the lattice, and is used to represent the case
// where e.g. an expression has multiple conflicting contexts.
func (p Context) IsConflicted() bool {
	return p.kind == conflictedKind
}

// Join returns the least upper bound of the two contexts, or false if this does
// not exist (i.e. the two context's are in conflict).
func (p Context) Join(other Context) Context {
	if p.IsVoid() {
		return other
	} else if other.IsVoid() {
		return p
	} else if p != other || p.IsConflicted() || other.IsConflicted() {
		// Conflicting contexts
		return ConflictingContext()
	}
	// Matching contexts
	return p
}

func (p Context) String() string {
	if p.IsVoid() {
		return "⊥"
	} else if p.IsConflicted() {
		return "⊤"
	}

	return p.ModuleId
}
