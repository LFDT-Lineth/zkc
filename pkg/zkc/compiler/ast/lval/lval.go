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
package lval

import (
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Resolved represents an lval whose external identifiers are otherwise
// resolved. As such, it should not be possible that such a declaration refers
// to unknown (or otherwise incorrect) external components.
type Resolved = LVal[symbol.Resolved]

// Unresolved represents an expression whose identifiers for external components
// are unresolved linkage records.  As such, its possible that such an
// expression instruction may fail with an error at link time due to an
// unresolvable reference to an external component (e.g. function, RAM, ROM,
// etc).
type Unresolved = LVal[symbol.Unresolved]

// LVal represents an arbitrary expression used within an instruction.
type LVal[S symbol.Symbol[S]] interface {
	// ExternUses returns the set of non-local declarations accessed by this
	// expression.  For example, external constants or memories used within.
	ExternUses() set.AnySortedSet[S]
	// RegistersRead returns the set of variables used (i.e. read) by this expression
	LocalUses() bit.Set
	// LocalDefs returns the set of local variables which assigned (either
	// fully or in part) by this expression.
	LocalDefs() bit.Set
	// String returns a string representation of this expression.
	String(mapping variable.Map[S]) string
}

// Uses determines the (unique) set of registers read by any expression
// in the given set of expressions.
func Uses[S symbol.Symbol[S]](exprs ...LVal[S]) []variable.Id {
	var (
		reads []variable.Id
		bits  bit.Set
	)
	// extract all usages
	for _, e := range exprs {
		bits.Union(e.LocalUses())
	}
	// Collect them all up
	for iter := bits.Iter(); iter.HasNext(); {
		next := iter.Next()
		//
		reads = append(reads, next)
	}
	//
	return reads
}

// Definitions determines the (unique) set of registers written by a given set
// of lvals.
func Definitions[S symbol.Symbol[S]](lvals ...LVal[S]) []variable.Id {
	var (
		reads []variable.Id
		bits  bit.Set
	)
	// extract all usages
	for _, lv := range lvals {
		bits.Union(lv.LocalDefs())
	}
	// Collect them all up
	for iter := bits.Iter(); iter.HasNext(); {
		next := iter.Next()
		//
		reads = append(reads, next)
	}
	//
	return reads
}

// String provides a generic facility for converting an expression into a
// suitable string.
func String[S symbol.Symbol[S]](e LVal[S], mapping variable.Map[S]) string {
	switch e := e.(type) {
	case *Variable[S]:
		return e.String(mapping)
	case *MemAccess[S]:
		var builder strings.Builder
		//
		for i, arg := range e.Args {
			if i != 0 {
				builder.WriteString(",")
			}
			//
			builder.WriteString(arg.String(mapping))
		}
		//
		return fmt.Sprintf("%s[%s]", e.Name.String(), builder.String())
	case *Array[S]:
		return fmt.Sprintf("%s[%s]", mapping.Variable(e.Id).Name, e.Arg.String(mapping))
	case *BitDestruct[S]:
		return e.String(mapping)
	default:
		panic("unknown lval")
	}
}
