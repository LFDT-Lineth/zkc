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
package stmt

import (
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Resolved represents a macro instruction  where external identifiers
// are otherwise resolved. As such, it should not be possible that such a
// declaration refers to unknown (or otherwise incorrect) external components.
type Resolved = Stmt[symbol.Resolved]

// Unresolved represents a statement whose identifiers for external components
// are unresolved linkage records.  As such, its possible that such a
// instruction may fail with an error at link time due to an unresolvable
// reference to an external component (e.g. function, RAM, ROM, etc).
type Unresolved = Stmt[symbol.Unresolved]

// Stmt provides an abstract notion of a macro "machine instruction".
// Here, macro is intended to imply that the instruction may break down into
// multiple underlying "micro instructions".
type Stmt[S symbol.Symbol[S]] interface {
	// Uses returns the set of variables used (i.e. read) by this instruction.
	Uses() []variable.Id
	// Definitions returns the set of variables registers defined (i.e. written)
	// by this instruction.
	Definitions() []variable.Id
	// Provide human readable form of instruction
	String(env variable.Map[S]) string
}
