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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// ResolvedEnvironment represents a typing environment which contains only
// resolved identifiers.
type ResolvedEnvironment = Environment[symbol.Resolved]

// UnresolvedEnvironment represents a typing environment which may contain unresolved
// identifiers.
type UnresolvedEnvironment = Environment[symbol.Unresolved]

// Environment provides a typing environment for externally defined types, such
// as for type aliases.
type Environment[S symbol.Symbol[S]] interface {
	// TypeOf resolves the type of an external identifier.
	TypeOf(S) Type[S]
	// WellFormed is used to check whether a given type is well-formed under the
	// given environment.  This is necessary to protected against various problems
	// in methods such as Bitwidth, SubtypeOf and EquiTypes.
	WellFormed(t Type[S]) bool
}
