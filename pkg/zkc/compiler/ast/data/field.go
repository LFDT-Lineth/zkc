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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// ResolvedFieldElement represents a field element type with only resolved identifiers.
type ResolvedFieldElement = FieldElement[symbol.Resolved]

// UnresolvedFieldElement represents a field element type which may contain unresolved identifiers.
type UnresolvedFieldElement = FieldElement[symbol.Unresolved]

// FieldElement captures the native field element type. A field element is any
// element within the prime field used by the verifier. Unlike unsigned integer
// types, the range of valid values depends on the choice of prime field.
type FieldElement[S symbol.Symbol[S]] struct{}

// NewFieldElement constructs a field element type.
func NewFieldElement[S symbol.Symbol[S]]() *FieldElement[S] {
	return &FieldElement[S]{}
}

// AsUint implementation for Type interface
func (p *FieldElement[S]) AsUint(Environment[S]) *UnsignedInt[S] { return nil }

// AsTuple implementation for Type interface
func (p *FieldElement[S]) AsTuple(Environment[S]) *Tuple[S] { return nil }

// AsAlias implementation for Type interface
func (p *FieldElement[S]) AsAlias(Environment[S]) *Alias[S] { return nil }

// AsFixedArray implementation for Type interface
func (p *FieldElement[I]) AsFixedArray(Environment[I]) *FixedArray[I] {
	return nil
}

// AsField implementation for Type interface
func (p *FieldElement[S]) AsField(Environment[S]) *FieldElement[S] { return p }

func (p *FieldElement[S]) String(_ Environment[S]) string { return "𝔽" }
