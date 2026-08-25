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
package bus

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Port describes where a module plugs into a bus: the module, a selector
// choosing the active rows, and the registers making up the message.  Its
// direction (send or receive) is given by the enclosing constraint.  The
// selector is mandatory so that all-zero padding rows stay off the bus.
type Port struct {
	// Module in which all registers are located.
	Module schema.ModuleId
	// Selector for this port; rows where it is zero are inactive.
	Selector register.Id
	// Registers making up the message.
	Registers []register.Id
}

// NewPort constructs a new port.
func NewPort(mid schema.ModuleId, selector register.Id, registers ...register.Id) Port {
	return Port{mid, selector, registers}
}

// Context returns the module in which this port's registers are located.
func (p *Port) Context() schema.ModuleId {
	return p.Module
}

// Ith returns the ith message register in this port.
func (p *Port) Ith(index uint) register.Id {
	return p.Registers[index]
}

// Len returns the number of message registers (excluding the selector).
func (p *Port) Len() uint {
	return uint(len(p.Registers))
}

// Lisp returns a textual representation of this port.
func (p *Port) Lisp(mapping register.Map) sexp.SExp {
	var terms = sexp.EmptyList()
	//
	terms.Append(lispOfRegister(p.Selector, mapping))
	//
	for i := range p.Len() {
		terms.Append(lispOfRegister(p.Ith(i), mapping))
	}
	//
	return terms
}

func lispOfRegister(rid register.Id, mapping register.Map) sexp.SExp {
	return sexp.NewSymbol(mapping.Register(rid).QualifiedName(mapping))
}
