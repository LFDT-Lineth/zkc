// Copyright Consensys Incorporated
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
package constraint

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Vector identifies a tuple of registers in one module, optionally filtered
// by a selector. Lookup, permutation and bus constraints use vectors on both
// sides.
type Vector struct {
	// Module in which all registers are located.
	Module schema.ModuleId
	// Selector for this vector (optional)
	Selector util.Option[register.Id]
	// Registers making up this vector.
	Registers []register.Id
}

// NewVector constructs a new vector in a given context with an optional selector.
func NewVector(mid schema.ModuleId, selector util.Option[register.Id], registers ...register.Id) Vector {
	if selector.HasValue() {
		return FilteredVector(mid, selector.Unwrap(), registers...)
	}
	//
	return UnfilteredVector(mid, registers...)
}

// UnfilteredVector constructs a new vector in a given context which has no selector.
func UnfilteredVector(mid schema.ModuleId, registers ...register.Id) Vector {
	return Vector{
		mid,
		util.None[register.Id](),
		registers,
	}
}

// FilteredVector constructs a new vector in a given context which has a selector.
func FilteredVector(mid schema.ModuleId, selector register.Id, registers ...register.Id) Vector {
	return Vector{
		mid,
		util.Some(selector),
		registers,
	}
}

// Context returns the module containing the vector's registers.
func (p Vector) Context() schema.ModuleId {
	return p.Module
}

// Cmp implementation for set.Comparable inteface.
func (p Vector) Cmp(o Vector) int {
	if c := cmp.Compare(p.Module, o.Module); c != 0 {
		return c
	} else if c := array.Compare(p.Registers, o.Registers); c != 0 {
		return c
	}
	//
	return compareSelectors(p.Selector, o.Selector)
}

// HasSelector reports whether the vector has a selector.
func (p Vector) HasSelector() bool {
	return p.Selector.HasValue()
}

// Ith returns the ith register in this vector.
func (p Vector) Ith(index uint) register.Id {
	return p.Registers[index]
}

// Len returns the number of registers in the tuple, excluding the selector.
func (p Vector) Len() uint {
	return uint(len(p.Registers))
}

// String returns a (unique) string representation for this set.
func (p Vector) String() string {
	var builder strings.Builder
	//
	if p.HasSelector() {
		fmt.Fprintf(&builder, "%x:%x", p.Module, p.Selector.Unwrap())
	} else {
		fmt.Fprintf(&builder, "%x", p.Module)
	}
	//
	for _, r := range p.Registers {
		fmt.Fprintf(&builder, ";%x", r.Unwrap())
	}
	//
	return builder.String()
}

// Lisp returns a textual representation of this vector.
func (p Vector) Lisp[F field.Element[F]](mapping register.Map[F]) sexp.SExp {
	var terms = sexp.EmptyList()
	//
	if p.HasSelector() {
		terms.Append(lispOfRegister(p.Selector.Unwrap(), mapping))
	} else {
		terms.Append(sexp.NewSymbol("_"))
	}
	// Iterate source registers
	for i := range p.Len() {
		terms.Append(lispOfRegister(p.Ith(i), mapping))
	}
	// Done
	return terms
}

// lispOfRegister returns a textual representation of a given register in a
// given module.
func lispOfRegister[F field.Element[F]](rid register.Id, mapping register.Map[F]) sexp.SExp {
	return sexp.NewSymbol(mapping.Register(rid).QualifiedName(mapping))
}

func compareSelectors(l, r util.Option[register.Id]) int {
	switch {
	case l.IsEmpty() && r.IsEmpty():
		return 0
	case l.IsEmpty():
		return -1
	case r.IsEmpty():
		return 1
	default:
		return l.Unwrap().Cmp(r.Unwrap())
	}
}
