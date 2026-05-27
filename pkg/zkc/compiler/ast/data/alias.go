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

// ResolvedAlias represents an alias which contains only resolved identifiers.
type ResolvedAlias = Alias[symbol.Resolved]

// UnresolvedAlias represents an alias which contains only unresolved identifiers.
type UnresolvedAlias = Alias[symbol.Unresolved]

// Alias captures the alias of a language type.
// Ref points to the symbol for the type-alias declaration if resolved.
type Alias[I symbol.Symbol[I]] struct {
	Name I
}

// NewAlias constructs an alias for a given Type.
func NewAlias[I symbol.Symbol[I]](name I) *Alias[I] {
	return &Alias[I]{name}
}

// AsUint implementation for Type interface
func (p *Alias[I]) AsUint(env Environment[I]) *UnsignedInt[I] {
	var t Type[I] = p
	//
	for t.AsAlias(env) != nil {
		// cast type to Alias to resolve
		a, _ := t.(*Alias[I])
		r := a.Resolve(env)
		// back to Type
		t = r
	}

	return t.AsUint(env)
}

// AsTuple implementation for Type interface
func (p *Alias[I]) AsTuple(Environment[I]) *Tuple[I] {
	return nil
}

// AsAlias implementation for Type interface
func (p *Alias[I]) AsAlias(Environment[I]) *Alias[I] {
	return p
}

// AsField implementation for Type interface
func (p *Alias[I]) AsField(env Environment[I]) *FieldElement[I] {
	var t Type[I] = p
	//
	for t.AsAlias(env) != nil {
		// cast type to Alias to resolve
		a, _ := t.(*Alias[I])
		r := a.Resolve(env)
		// back to Type
		t = r
	}
	//
	return t.AsField(env)
}

// AsFixedArray implementation for Type interface
func (p *Alias[I]) AsFixedArray(env Environment[I]) *FixedArray[I] {
	var t Type[I] = p
	//
	for t.AsAlias(env) != nil {
		// cast type to Alias to resolve
		a, _ := t.(*Alias[I])
		r := a.Resolve(env)
		// back to Type
		t = r
	}
	//
	return t.AsFixedArray(env)
}

func (p *Alias[I]) String(Environment[I]) string {
	return p.Name.String()
}

// Resolve returns the type that this alias refers to in the given environment.
func (p *Alias[I]) Resolve(env Environment[I]) Type[I] {
	return env.TypeOf(p.Name)
}
