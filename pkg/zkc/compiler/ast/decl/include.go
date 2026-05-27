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

import "github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"

// ResolvedInclude is simply a useful alias.
type ResolvedInclude = Include[symbol.Resolved]

// UnresolvedInclude is simply a useful alias.
type UnresolvedInclude = Include[symbol.Unresolved]

// Include corresponds with a specific include statement used within a source
// file.
type Include[S any] struct {
	path string
}

// NewInclude constructs a new include representing the given path.
func NewInclude[S any](path string) *Include[S] {
	return &Include[S]{path}
}

// Arity implementation for Decl interface.
func (p *Include[S]) Arity() (inputs uint, outputs uint) {
	return 0, 0
}

// Name implementation for Decl interface.
func (p *Include[S]) Name() string {
	return ""
}

// Externs implementation for Decl interface.
func (p *Include[S]) Externs() []S {
	return nil
}

// Annotations implementation for Decl interface.
func (p *Include[S]) Annotations() []string {
	return nil
}

// SetAnnotations implementation for Decl interface.
func (p *Include[S]) SetAnnotations(_ []string) {
	// NOTE: we don't do anything here on the understanding that this will be
	// reported as an error during parsing, since no annotation can be used on
	// an include statement.
}

// Pattern returns the include pattern; observe that this may include a wildcard
// match, or it might identify a single file.
func (p *Include[S]) Pattern() string {
	return p.path
}
