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
package lookup

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Vector encapsulates all columns on one side of a lookup (i.e. it
// represents all source columns or all target columns).
type Vector[F any, E term.Evaluable[F]] struct {
	// Module in which all terms are evaluated.
	Module schema.ModuleId
	// Selector for this vector (optional)
	Selector util.Option[E]
	// Terms making up this vector.
	Terms []E
}

// NewVector constructs a new vector in a given context with an optional selector.
func NewVector[F any, E term.Evaluable[F]](mid schema.ModuleId, selector util.Option[E], terms ...E) Vector[F, E] {
	if selector.HasValue() {
		return FilteredVector(mid, selector.Unwrap(), terms...)
	}
	//
	return UnfilteredVector(mid, terms...)
}

// UnfilteredVector constructs a new vector in a given context which has no selector.
func UnfilteredVector[F any, E term.Evaluable[F]](mid schema.ModuleId, terms ...E) Vector[F, E] {
	return Vector[F, E]{
		mid,
		util.None[E](),
		terms,
	}
}

// FilteredVector constructs a new vector in a given context which has a selector.
func FilteredVector[F any, E term.Evaluable[F]](mid schema.ModuleId, selector E, terms ...E) Vector[F, E] {
	return Vector[F, E]{
		mid,
		util.Some(selector),
		terms,
	}
}

// Bounds determines the well-definedness bounds for all terms within this vector.
//
//nolint:revive
func (p *Vector[F, E]) Bounds(module uint) util.Bounds {
	var bound util.Bounds
	//
	if module == p.Module {
		// Include bounds for selector (if applicable)
		if p.HasSelector() {
			sel := p.Selector.Unwrap().Bounds()
			bound.Union(&sel)
		}
		// Include bounds for all terms
		for _, e := range p.Terms {
			eth := e.Bounds()
			bound.Union(&eth)
		}
	}
	//
	return bound
}

// Context returns the conterxt in which all terms of this vector must be
// evaluated.
func (p *Vector[F, E]) Context() schema.ModuleId {
	return p.Module
}

// HasSelector determines whether or not this lookup vector has a selector or
// not.
func (p *Vector[F, E]) HasSelector() bool {
	return p.Selector.HasValue()
}

// Ith returns the ith term in this vector.
func (p *Vector[F, E]) Ith(index uint) E {
	return p.Terms[index]
}

// Len returns the number of items in this lookup vector.  Note this doesn't
// include the selector (since this is optional anyway).
func (p *Vector[F, E]) Len() uint {
	return uint(len(p.Terms))
}

// Lisp returns a textual representation of this vector.
func (p *Vector[F, E]) Lisp(mapping schema.AnySchema[F]) sexp.SExp {
	var (
		module = mapping.Module(p.Module)
		terms  = sexp.EmptyList()
	)
	//
	if p.HasSelector() {
		terms.Append(p.Selector.Unwrap().Lisp(true, module))
	} else {
		terms.Append(sexp.NewSymbol("_"))
	}
	// Iterate source expressions
	for i := range p.Len() {
		terms.Append(p.Ith(i).Lisp(true, module))
	}
	// Done
	return terms
}

// Substitute any matchined labelled constants within this vector
func (p *Vector[F, E]) Substitute(mapping map[string]F) {
	for _, ith := range p.Terms {
		ith.Substitute(mapping)
	}
	// Substitute through selector (if applicable)
	if p.HasSelector() {
		p.Selector.Unwrap().Substitute(mapping)
	}
}
