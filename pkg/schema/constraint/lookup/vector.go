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
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Vector encapsulates all registers on one side of a lookup (i.e. it
// represents all source registers or all target registers).  Observe that a
// vector can only be made up of registers (rather than arbitrary expressions).
// Thus, anything else must be expanded into a register beforehand.
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

// Context returns the conterxt in which all registers of this vector are
// located.
func (p *Vector) Context() schema.ModuleId {
	return p.Module
}

// HasSelector determines whether or not this lookup vector has a selector or
// not.
func (p *Vector) HasSelector() bool {
	return p.Selector.HasValue()
}

// Ith returns the ith register in this vector.
func (p *Vector) Ith(index uint) register.Id {
	return p.Registers[index]
}

// Len returns the number of items in this lookup vector.  Note this doesn't
// include the selector (since this is optional anyway).
func (p *Vector) Len() uint {
	return uint(len(p.Registers))
}

// Lisp returns a textual representation of this vector.
func (p *Vector) Lisp(mapping register.Map) sexp.SExp {
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
func lispOfRegister(rid register.Id, mapping register.Map) sexp.SExp {
	return sexp.NewSymbol(mapping.Register(rid).QualifiedName(mapping))
}
