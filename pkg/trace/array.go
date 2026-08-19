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
package trace

import (
	"math"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Array provides an implementation of Trace which stores rows as an array.
type Array[T any, M ModuleBuilder[T, M]] struct {
	// Holds the set of modules in this trace.  The index of each module in this
	// array uniquely identifies it, and is referred to as the "module index".
	modules []M
}

// NewArray constructs a row-major trace from a given set of modules.
func NewArray[T any, M ModuleBuilder[T, M]](modules []M) *Array[T, M] {
	return &Array[T, M]{modules}
}

// HasModule determines whether this trace has a module with the given name and,
// if so, what its module index is.
func (p *Array[T, M]) HasModule(name string) (uint, bool) {
	for mid, mod := range p.modules {
		if mod.Name() == name {
			return uint(mid), true
		}
	}
	//
	return math.MaxUint, false
}

// Module returns a specific module in this trace.
func (p *Array[T, M]) Module(module uint) Module[T] {
	return p.modules[module]
}

// RawModule returns a specific (raw) module in this trace.
func (p *Array[T, M]) RawModule(module uint) M {
	return p.modules[module]
}

// SetRawModule replaces a specific (raw) module in this trace.  This is
// necessary, for example, after an operation (such as Pad) which returns a
// new module rather than updating the original in place.
func (p *Array[T, M]) SetRawModule(module uint, m M) {
	p.modules[module] = m
}

// Modules returns an iterator over the modules in this trace.
func (p *Array[T, M]) Modules() iter.Iterator[Module[T]] {
	it := iter.NewArrayIterator(p.modules)
	//
	return iter.NewCastIterator[M, Module[T]](it)
}

// Width returns the number of modules in this trace.
func (p *Array[T, M]) Width() uint {
	return uint(len(p.modules))
}

func (p *Array[T, M]) String() string {
	var id strings.Builder

	id.WriteString("{")
	//
	for i, m := range p.modules {
		if i != 0 {
			id.WriteString(", ")
		}
		//
		id.WriteString(m.String())
	}
	//
	id.WriteString("}")
	//
	return id.String()
}
