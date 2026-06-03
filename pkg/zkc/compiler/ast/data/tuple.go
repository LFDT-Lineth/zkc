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
package data

import (
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// Tuple represents the composition of zero or more types together.  For
// example, (u8,u16) is a tuple type consisting of two elements: the first being
// a u8, and the second being a u16.  The overall width of the tuple is
// therefore the sum of the widths of the elements.
type Tuple[S symbol.Symbol[S]] struct {
	elements []Type[S]
}

// AsTuple implementation for Type interface
func (p *Tuple[S]) AsTuple(Environment[S]) *Tuple[S] {
	return p
}

// AsUint implementation for Type interface
func (p *Tuple[S]) AsUint(Environment[S]) *UnsignedInt[S] {
	return nil
}

// AsAlias implementation for Type interface
func (p *Tuple[S]) AsAlias(Environment[S]) *Alias[S] {
	return nil
}

// AsField implementation for Type interface
func (p *Tuple[S]) AsField(Environment[S]) *FieldElement[S] {
	return nil
}

// AsFixedArray implementation for Type interface
func (p *Tuple[S]) AsFixedArray(Environment[S]) *FixedArray[S] {
	return nil
}

// Ith returns the ith element in this tuple
func (p *Tuple[S]) Ith(index uint) Type[S] {
	return p.elements[index]
}

// Width returns the number of elements in this tuple.
func (p *Tuple[S]) Width() uint {
	return uint(len(p.elements))
}

// Types provides access to the underlying array of types.  This has been
// defensively cloned before returning.
func (p *Tuple[S]) Types() []Type[S] {
	return slices.Clone(p.elements)
}

// Flattern implementation for Type interface
func (p *Tuple[S]) Flattern(prefix string, env Environment[S], constructor func(name string, bitwidth uint)) {

}

func (p *Tuple[S]) String(env Environment[S]) string {
	var builder strings.Builder
	//
	builder.WriteString("(")
	//
	for i, element := range p.elements {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(element.String(env))
	}
	//
	builder.WriteString(")")
	//
	return builder.String()
}
