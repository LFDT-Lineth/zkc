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
	"fmt"
	"math/big"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/collection/set"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/data"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// Resolved represents an expression whose external identifiers are otherwise
// resolved. As such, it should not be possible that such a declaration refers
// to unknown (or otherwise incorrect) external components.
type Resolved = Expr[symbol.Resolved]

// Unresolved represents an expression whose identifiers for external components
// are unresolved linkage records.  As such, its possible that such an
// expression instruction may fail with an error at link time due to an
// unresolvable reference to an external component (e.g. function, RAM, ROM,
// etc).
type Unresolved = Expr[symbol.Unresolved]

// Expr represents an arbitrary expression used within an instruction.
type Expr[S symbol.Symbol[S]] interface {
	// ExternUses returns the set of non-local declarations accessed by this
	// expression.  For example, external constants or memories used within.
	ExternUses() set.AnySortedSet[S]
	// LocalUses returns the set of variables used (i.e. read) by this expression
	LocalUses() bit.Set
	// String returns a string representation of this expression.
	String(mapping variable.Map[S]) string
	// Type returns the type associated with this expression (or nil if that has
	// not yet been determined).
	Type() data.Type[S]
	// SetType sets the type associated with this expression.
	SetType(data.Type[S])
}

// Uses determines the (unique) set of registers read by any expression
// in the given set of expressions.
func Uses[S symbol.Symbol[S]](exprs ...Expr[S]) []variable.Id {
	var (
		reads []variable.Id
		bits  bit.Set
	)
	// extract all usages
	for _, e := range exprs {
		if e != nil {
			bits.Union(e.LocalUses())
		}
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
func String[S symbol.Symbol[S]](e Expr[S], mapping variable.Map[S]) string {
	var (
		exprs    []Expr[S]
		operator string
		builder  strings.Builder
	)
	//
	switch e := e.(type) {
	case *Cast[S]:
		inner := String(e.Expr, mapping)
		if needsBraces(e.Expr) {
			inner = "(" + inner + ")"
		}

		var env data.Environment[S]

		return inner + " as " + e.CastType.String(env)
	case *Add[S]:
		operator = "+"
		exprs = e.Exprs
	case *ArrayAccess[S]:
		return fmt.Sprintf("%s[%s]", mapping.Variable(e.Id).Name, e.Arg.String(mapping))
	case *BitwiseAnd[S]:
		operator = "&"
		exprs = e.Exprs
	case *BitwiseOr[S]:
		operator = "|"
		exprs = e.Exprs
	case *Concat[S]:
		operator = "::"
		exprs = e.Exprs
	case *Const[S]:
		return stringOfConstant(e.constant, e.base)
	case *Div[S]:
		exprs = e.Exprs
		operator = "/"
	case *LocalAccess[S]:
		return mapping.Variable(e.Variable).Name
	case *Mul[S]:
		exprs = e.Exprs
		operator = "*"
	case *BitwiseNot[S]:
		if needsBraces[S](e.Expr) {
			return "~(" + String[S](e.Expr, mapping) + ")"
		}

		return "~" + String(e.Expr, mapping)
	case *ExternAccess[S]:
		args := stringOfArguments(e.Args, mapping)
		name := e.Name.String()
		//
		switch {
		case e.Name.IsFunction():
			return fmt.Sprintf("%s(%s)", name, args)
		case e.Name.IsMemory():
			return fmt.Sprintf("%s[%s]", name, args)
		default:
			return name
		}

	case *Shl[S]:
		operator = "<<"
		exprs = e.Exprs
	case *Shr[S]:
		operator = ">>"
		exprs = e.Exprs
	case *Sub[S]:
		exprs = e.Exprs
		operator = "-"
	case *TupleInitialiser[S]:
		args := stringOfArguments(e.Exprs, mapping)
		return fmt.Sprintf("(%s)", args)
	case *Rem[S]:
		exprs = e.Exprs
		operator = "%"
	case *Ternary[S]:
		return e.String(mapping)
	case *Xor[S]:
		operator = "^"
		exprs = e.Exprs

	default:
		panic("unreachable")
	}
	//
	for i, e := range exprs {
		if i != 0 {
			builder.WriteString(" ")
			builder.WriteString(operator)
			builder.WriteString(" ")
		}
		//
		if needsBraces(e) {
			builder.WriteString("(")
			builder.WriteString(String(e, mapping))
			builder.WriteString(")")
		} else {
			builder.WriteString(String(e, mapping))
		}
	}
	//
	return builder.String()
}

func stringOfArguments[S symbol.Symbol[S]](args []Expr[S], mapping variable.Map[S]) string {
	var builder strings.Builder
	//
	for i, arg := range args {
		if i != 0 {
			builder.WriteString(",")
		}

		builder.WriteString(String[S](arg, mapping))
	}
	//
	return builder.String()
}

func stringOfConstant(val big.Int, base uint) string {
	switch base {
	case 2:
		return fmt.Sprintf("0b%s", val.Text(2))
	case 16:
		return fmt.Sprintf("0x%s", val.Text(16))
	default:
		return val.String()
	}
}

func needsBraces[S symbol.Symbol[S]](e Expr[S]) bool {
	switch e.(type) {
	case *Cast[S]:
		return false
	case *Const[S]:
		return false
	case *LocalAccess[S]:
		return false
	case *ArrayAccess[S]:
		return false
	case *ExternAccess[S]:
		return false
	default:
		return true
	}
}
