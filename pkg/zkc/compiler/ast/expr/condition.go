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
package expr

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// ResolvedCondition represents a condition whose external identifiers are otherwise
// resolved. As such, it should not be possible that such a declaration refers
// to unknown (or otherwise incorrect) external components.
type ResolvedCondition = Condition[symbol.Resolved]

// UnresolvedCondition represents a condition whose identifiers for external
// components are unresolved linkage records.  As such, its possible that such
// an expression instruction may fail with an error at link time due to an
// unresolvable reference to an external component (e.g. function, RAM, ROM,
// etc).
type UnresolvedCondition = Condition[symbol.Unresolved]

// Condition describes a logical condition which can be used as branch
// conditions (e.g. for if/while, etc).
type Condition[S symbol.Symbol[S]] interface {
	// Negate a given condition to produce an equivalent (but negated)
	// condition.
	Negate() Condition[S]
	// ExternUses returns the set of non-local declarations accessed by this
	// condition.  For example, external constants or memories used within.
	ExternUses() set.AnySortedSet[S]
	// RegistersRead returns the set of variables used (i.e. read) by this condition
	LocalUses() bit.Set
	// String returns a string representation of this condition.
	String(mapping variable.Map[S]) string
}
