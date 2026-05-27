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
package decl

import (
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// ResolvedTypeAlias represents a type alias whose contents uses only external
// identifiers which are resolved. As such, it should not be possible that such
// a declaration refers to unknown (or otherwise incorrect) external components.
type ResolvedTypeAlias = TypeAlias[symbol.Resolved]

// UnresolvedTypeAlias represents a type alias whose contents may contain string
// identifiers for external (i.e. unlinked) components.  As such, its possible
// that such an expression may fail with an error at link time due to an
// unresolvable reference to an external component (e.g. function, RAM, ROM,
// etc).
type UnresolvedTypeAlias = TypeAlias[symbol.Unresolved]

// TypeAlias represents an alias for either a type at the source level or for
// another alias.
type TypeAlias[I symbol.Symbol[I]] struct {
	name        string
	annotations []string
	DataType    data.Type[I]
}

// NewTypeAlias creates a new type alias for a fundamental type
func NewTypeAlias[I symbol.Symbol[I]](name string, datatype data.Type[I]) *TypeAlias[I] {
	return &TypeAlias[I]{name: name, DataType: datatype}
}

// Arity implementation for Declaration interface
func (p *TypeAlias[I]) Arity() (nInputs, nOutputs uint) {
	return 0, 0
}

// Annotations implementation for Declaration interface
func (p *TypeAlias[I]) Annotations() []string {
	return p.annotations
}

// SetAnnotations implementation for Declaration interface
func (p *TypeAlias[I]) SetAnnotations(annotations []string) {
	p.annotations = annotations
}

// Name implementation for AssemblyComponent interface
func (p *TypeAlias[I]) Name() string {
	return p.name
}

// Externs implementation for Declaration interface.
func (p *TypeAlias[I]) Externs() []I {
	return nil
}
