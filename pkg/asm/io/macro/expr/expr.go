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
	"encoding/gob"
	"fmt"
	"math/big"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
)

var (
	biZERO big.Int = *big.NewInt(0)
	biONE  big.Int = *big.NewInt(1)
)

// Expr represents an arbitrary expression used within an instruction.
type Expr interface {
	// Check whether two expressions are equivalent
	Equals(e Expr) bool
	// Evaluate this expression in a given environment producing a given value.
	Eval([]big.Int) big.Int
	// Polynomial returns this expression flatterned into a polynomial form.
	Polynomial() agnostic.StaticPolynomial
	// RegistersRead returns the set of registers read by this expression
	RegistersRead() bit.Set
	// String returns a string representation of this expression in a given base.
	String(mapping register.Map) string
	// ValueRange returns the interval of values that this term can evaluate to.
	// For terms accessing registers, this is determined by the declared width of
	// the register.
	ValueRange(mapping register.Map) math.Interval
}

// AtomicExpr represents either a register access or a constant access.
type AtomicExpr interface {
	Expr
	// Convert this (atomic) expression into a micro expression.
	ToMicroExpr() micro.Expr
}

// EqualsAll determines whether all of the expressions on the left-hand side
// match those on the right-hand side.  The number of expressions on both sides
// must also match.
func EqualsAll(lhs []Expr, rhs []Expr) bool {
	if len(lhs) == len(rhs) {
		for i := range len(lhs) {
			if !lhs[i].Equals(rhs[i]) {
				return false
			}
		}
		//
		return true
	}
	//
	return false
}

// BitWidth returns the minimum number of bits required to store any evaluation
// of this expression.  In addition, it provides an indicator as to whether or
// not any evaluation could result in a negative value.
func BitWidth(e Expr, mapping register.Map) (uint, bool) {
	var (
		// Determine set of all values that right-hand side can evaluate to
		values = e.ValueRange(mapping)
		// Determine bitwidth required to contain all values
		bitwidth, signed = values.BitWidth()
	)
	// For signed arithmetic, we need a specific sign bit.
	if signed {
		bitwidth++
	}
	//
	return bitwidth, signed
}

// Eval evaluates a set of zero or more expressions producing a set of zero or
// more values.
func Eval(state []big.Int, exprs []Expr) []big.Int {
	var res = make([]big.Int, len(exprs))
	//
	for i, e := range exprs {
		res[i] = e.Eval(state)
	}
	//
	return res
}

// RegistersRead determines the (unique) set of registers read by any expression
// in the given set of expressions.
func RegistersRead(exprs ...Expr) []register.Id {
	var (
		reads []register.Id
		bits  bit.Set
	)
	// extract all usages
	for _, e := range exprs {
		bits.Union(e.RegistersRead())
	}
	// Collect them all up
	for iter := bits.Iter(); iter.HasNext(); {
		next := iter.Next()
		//
		reads = append(reads, register.NewId(next))
	}
	//
	return reads
}

// String provides a generic facility for converting an expression into a
// suitable string.
func String(e Expr, mapping register.Map) string {
	var (
		exprs    []Expr
		operator string
		builder  strings.Builder
	)
	//
	switch e := e.(type) {
	case *Add:
		operator = "+"
		exprs = e.Exprs
	case *Const:
		return stringOfConstant(e.Constant, e.Base)
	case *Mul:
		exprs = e.Exprs
		operator = "*"
	case *RegAccess:
		return mapping.Register(e.Register).Name()
	case *Sub:
		exprs = e.Exprs
		operator = "-"
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

func needsBraces(e Expr) bool {
	switch e.(type) {
	case *Const:
		return false
	case *RegAccess:
		return false
	default:
		return true
	}
}

func init() {
	gob.Register(Expr(&Add{}))
	gob.Register(Expr(&Const{}))
	gob.Register(Expr(&Mul{}))
	gob.Register(Expr(&RegAccess{}))
	gob.Register(Expr(&Sub{}))
}
