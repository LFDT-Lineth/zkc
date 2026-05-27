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
	"slices"
	"strings"
)

// Vector represents a sequence of one or more registers which are imagined to
// be concatenated together, with the least significant having the least index
// in the vector.  For example, we might have a vector hi::lo where lo has index
// 0, and hi has index 1.
type Vector struct {
	regs []Id
}

// NewVector constructs a new vector from a given sequence of registers, where
// the least significant register has the lowest index.
func NewVector(regs ...Id) Vector {
	return Vector{regs}
}

// AsRegister returns this vector as a single register.  Observe that this will
// panic if the vector contains more than one register.
func (p Vector) AsRegister() Id {
	if len(p.regs) != 1 {
		panic("cannot coerce vector into single register")
	}
	//
	return p.regs[0]
}

// Clone this vector producing an identical but physically disjoint vector.
func (p Vector) Clone() Vector {
	return Vector{slices.Clone(p.regs)}
}

// BitWidth returns the bitwidth of this vector in a given context.
func (p Vector) BitWidth(fn Map) uint {
	var bitwidth uint
	//
	for _, r := range p.regs {
		bitwidth += fn.Register(r).Width()
	}
	//
	return bitwidth
}

// Len returns the length of this vector
func (p Vector) Len() uint {
	return uint(len(p.regs))
}

// Registers provides raw access to the underlying register array wrapped in
// this vector.
func (p Vector) Registers() []Id {
	return p.regs
}

// Split this vector according to a given limbs mapping.
func (p Vector) Split(mapping LimbsMap) Vector {
	var limbs []Id
	//
	for _, r := range p.regs {
		limbs = append(limbs, mapping.LimbIds(r)...)
	}
	//
	return Vector{limbs}
}

func (p Vector) String(fn Map) string {
	var builder strings.Builder
	//
	for i := len(p.regs); i > 0; i-- {
		ith := p.regs[i-1]
		if i != len(p.regs) {
			builder.WriteString("::")
		}
		//
		if fn == nil {
			fmt.Fprintf(&builder, "?%d", ith.Unwrap())
		} else {
			builder.WriteString(fn.Register(ith).Name())
		}
	}
	//
	return builder.String()
}
