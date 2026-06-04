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
package register

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
)

// Map provides a generic interface for entities which hold information
// about registers.
type Map interface {
	fmt.Stringer
	// Name returns the name given to the enclosing entity (i.e. module or
	// function), along with its multiplier.
	Name() trace.ModuleName
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns false.
	HasRegister(name string) (Id, bool)
	// Access a given register in this module.
	Register(Id) Register
	// Registers providers access to the underlying registers of this map.
	Registers() []Register
}

// ConstMap is a register map which additionally provides guaranteed access to
// a constant (binary) register.  That is, a register which is always either "0"
// or always either "1".
type ConstMap interface {
	Map
	// ConstRegister returns the ID of a constant register which is either
	// always zero or always one (no other constants are supported at this
	// time).   If such a register does not already exist, then one is created.
	// This ensures constant registers are only included when they are actually
	// needed.
	ConstRegister(constant uint8) Id
}

// MapToString provides a default method for converting a register map
// into a simple string representation.
func MapToString(p Map) string {
	var builder strings.Builder
	//
	builder.WriteString("{")
	builder.WriteString(p.Name().String())
	builder.WriteString(":")
	//
	for i, r := range p.Registers() {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(r.Name())
	}
	//
	builder.WriteString("}")
	//
	return builder.String()
}

// ArrayMap constructs a register map from an array of registers.
func ArrayMap(name trace.ModuleName, regs ...Register) Map {
	return &arrayMap{name, regs}
}

type arrayMap struct {
	name trace.ModuleName
	regs []Register
}

func (p *arrayMap) Name() trace.ModuleName {
	return p.name
}

func (p *arrayMap) HasRegister(name string) (Id, bool) {
	for i, r := range p.regs {
		if r.Name() == name {
			return NewId(uint(i)), true
		}
	}
	//
	return UnusedId(), false
}

func (p *arrayMap) Register(id Id) Register {
	return p.regs[id.Unwrap()]
}

func (p *arrayMap) Registers() []Register {
	return p.regs
}

func (p *arrayMap) String() string {
	return MapToString(p)
}
