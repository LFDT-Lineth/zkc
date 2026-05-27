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
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// Resolved represents a declaration  where external identifiers are otherwise
// resolved. As such, it should not be possible that such a declaration refers
// to unknown (or otherwise incorrect) external components.
type Resolved = Declaration[symbol.Resolved]

// Unresolved represents a declaration which contains string identifiers for
// external (i.e. unlinked) components.  As such, its possible that such a
// declaration may fail with an error at link time due to an unresolvable
// reference to an external component (e.g. function, RAM, ROM, etc).
type Unresolved = Declaration[symbol.Unresolved]

// Declaration represents something declared within a source file, such as a
// function or constant, etc.
type Declaration[S any] interface {
	// Arity returns the number of inputs/outputs for this declaration.
	Arity() (inputs uint, outputs uint)
	// Return name of this component
	Name() string
	// Determine all reference external symbols
	Externs() []S
	// Annotations returns the annotations associated with this declaration.
	Annotations() []string
	// SetAnnotations sets the annotations associated with this declaration.
	SetAnnotations(annotations []string)
}
