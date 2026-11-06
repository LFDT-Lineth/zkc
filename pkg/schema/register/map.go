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
package register

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Map provides a generic interface for entities which hold information
// about registers.
type Map[F field.Element[F]] interface {
	fmt.Stringer
	// Name returns the name given to the enclosing entity (i.e. module or
	// function).
	Name() string
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns false.
	HasRegister(name string) (Id, bool)
	// Access a given register in this module.
	Register(Id) Register[F]
	// Registers providers access to the underlying registers of this map.
	Registers() []Register[F]
}

// MapToString provides a default method for converting a register map
// into a simple string representation.
func MapToString[F field.Element[F]](p Map[F]) string {
	var builder strings.Builder
	//
	builder.WriteString("{")
	builder.WriteString(p.Name())
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
func ArrayMap[F field.Element[F]](name string, regs ...Register[F]) Map[F] {
	return &arrayMap[F]{name, regs}
}

type arrayMap[F field.Element[F]] struct {
	name string
	regs []Register[F]
}

func (p *arrayMap[F]) Name() string {
	return p.name
}

func (p *arrayMap[F]) HasRegister(name string) (Id, bool) {
	for i, r := range p.regs {
		if r.Name() == name {
			return NewId(uint(i)), true
		}
	}
	//
	return UnusedId(), false
}

func (p *arrayMap[F]) Register(id Id) Register[F] {
	return p.regs[id.Unwrap()]
}

func (p *arrayMap[F]) Registers() []Register[F] {
	return p.regs
}

func (p *arrayMap[F]) String() string {
	return MapToString(p)
}
