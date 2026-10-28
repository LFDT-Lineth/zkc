// Copyright Consensys Incorporated
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
package inspector

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
)

const qROW = 0
const qVAR = 1
const qNUM = 2
const qOR = 3
const qAND = 4
const qEQ = 5
const qNEQ = 6
const qLT = 7
const qLTEQ = 8
const qADD = 9
const qMUL = 10
const qSUB = 11

// QueryEnv abstracts the notion of an environment for evaluating a query.
// Specifically, the environment provides a mapping from variable names to their
// values on a given row.
type QueryEnv = func(string, uint) (big.Int, bool)

var biZero *big.Int = big.NewInt(0)
var biOne *big.Int = big.NewInt(1)

// Query represents a boolean expression which can be evaluated over a
// given set of columns.
type Query struct {
	// operation
	op int
	// arguments (if applicable)
	args []Query
	// constant value (if applicable)
	number big.Int
	// variable name (if applicable)
	name string
}

// Variable constructs a variable of the given name.
func (p *Query) Variable(name string) *Query {
	var query Query
	//
	if name == "$" {
		query.op = qROW
		query.name = name
	} else {
		query.op = qVAR
		query.name = name
	}
	//
	return &query
}

// Number constructs a number with the given value
func (p *Query) Number(number big.Int) *Query {
	var (
		val   big.Int
		query Query
	)
	//
	val.SetBytes(number.Bytes())
	//
	query.op = qNUM
	query.number = val
	//
	return &query
}

// Or constructs a disjunction of queries.
func (p *Query) Or(queries ...*Query) *Query {
	args := build_args(p, queries)
	//
	return &Query{qOR, args, *biZero, ""}
}

// And constructs a conjunction of queries.
func (p *Query) And(queries ...*Query) *Query {
	args := build_args(p, queries)
	//
	return &Query{qAND, args, *biZero, ""}
}

// Truth constructs a logical truth
func (p *Query) Truth(val bool) *Query {
	panic("unsupported operation")
}

// Equals constructs an equality between two queries.
func (p *Query) Equals(rhs *Query) *Query {
	return &Query{qEQ, []Query{*p, *rhs}, *biZero, ""}
}

// NotEquals constructs a non-equality between two queries.
func (p *Query) NotEquals(rhs *Query) *Query {
	return &Query{qNEQ, []Query{*p, *rhs}, *biZero, ""}
}

// LessThan constructs a (strict) inequality between two queries.
func (p *Query) LessThan(rhs *Query) *Query {
	return &Query{qLT, []Query{*p, *rhs}, *biZero, ""}
}

// LessThanEquals constructs a (non-strict) inequality between two queries.
func (p *Query) LessThanEquals(rhs *Query) *Query {
	return &Query{qLTEQ, []Query{*p, *rhs}, *biZero, ""}
}

// Add constructs the sum of one or more queries
func (p *Query) Add(queries ...*Query) *Query {
	args := build_args(p, queries)
	//
	return &Query{qADD, args, *biZero, ""}
}

// Mul constructs the product of one or more queries
func (p *Query) Mul(queries ...*Query) *Query {
	args := build_args(p, queries)
	//
	return &Query{qMUL, args, *biZero, ""}
}

// Sub constructs the subtraction of one or more queries
func (p *Query) Sub(queries ...*Query) *Query {
	args := build_args(p, queries)
	//
	return &Query{qSUB, args, *biZero, ""}
}

func build_args(q *Query, queries []*Query) []Query {
	args := make([]Query, 1+len(queries))
	args[0] = *q
	//
	for i := range queries {
		args[i+1] = *queries[i]
	}
	//
	return args
}

// String produces a parseable string from this query.
func (p *Query) String() string {
	return query_string(*p)
}

// Eval evaluates the given query in the given environment.
func (p *Query) Eval(row uint, env QueryEnv) (big.Int, bool) {
	switch p.op {
	case qROW:
		return *big.NewInt(int64(row)), true
	case qVAR:
		return env(p.name, row)
	case qNUM:
		return p.number, true
	case qEQ:
		return eval_binary(row, env, p.args[0], p.args[1], eval_eq)
	case qNEQ:
		return eval_binary(row, env, p.args[0], p.args[1], eval_neq)
	case qLT:
		return eval_binary(row, env, p.args[0], p.args[1], eval_lt)
	case qLTEQ:
		return eval_binary(row, env, p.args[0], p.args[1], eval_lteq)
	case qOR:
		return eval_nary(row, env, p.args, eval_or)
	case qAND:
		return eval_nary(row, env, p.args, eval_and)
	case qADD:
		return eval_nary(row, env, p.args, eval_add)
	case qMUL:
		return eval_nary(row, env, p.args, eval_mul)
	case qSUB:
		return eval_nary(row, env, p.args, eval_sub)
	default:
		panic(fmt.Sprintf("unknown operator (%d)", p.op))
	}
}

type binary_op func(big.Int, big.Int) big.Int
type nary_op func([]big.Int) big.Int

func eval_binary(row uint, env QueryEnv, lhs Query, rhs Query,
	fn binary_op) (big.Int, bool) {
	// Evaluate left-hand side
	lv, lb := lhs.Eval(row, env)
	// Evaluate right-hand side
	rv, rb := rhs.Eval(row, env)
	// Performan binary operation
	return fn(lv, rv), lb && rb
}

func eval_nary(row uint, env QueryEnv, args []Query, fn nary_op) (big.Int, bool) {
	var (
		vals      = make([]big.Int, len(args))
		ok   bool = true
	)
	// Evaluate arguments
	for i, arg := range args {
		var iok bool

		vals[i], iok = arg.Eval(row, env)
		ok = ok && iok
	}
	//
	return fn(vals), ok
}

func eval_eq(lhs big.Int, rhs big.Int) big.Int {
	// Perform comparison
	if lhs.Cmp(&rhs) == 0 {
		return *big.NewInt(0)
	}
	//
	return *big.NewInt(1)
}

func eval_neq(lhs big.Int, rhs big.Int) big.Int {
	// Perform comparison
	if lhs.Cmp(&rhs) != 0 {
		return *biZero
	}
	//
	return *biOne
}

func eval_lt(lhs big.Int, rhs big.Int) big.Int {
	// Perform comparison
	if lhs.Cmp(&rhs) < 0 {
		return *biZero
	}
	//
	return *biOne
}

func eval_lteq(lhs big.Int, rhs big.Int) big.Int {
	// Perform comparison
	if lhs.Cmp(&rhs) <= 0 {
		return *biZero
	}
	//
	return *biOne
}

func eval_or(vals []big.Int) big.Int {
	for _, v := range vals {
		if v.Cmp(biZero) == 0 {
			// Success
			return v
		}
	}
	// Fail
	return *biOne
}

func eval_and(vals []big.Int) big.Int {
	//
	for _, v := range vals {
		if v.Cmp(biZero) != 0 {
			// Fail
			return v
		}
	}
	// Success
	return *biZero
}

func eval_add(vals []big.Int) big.Int {
	var val *big.Int = big.NewInt(0)
	//
	for _, v := range vals {
		val = val.Add(val, &v)
	}
	//
	return *val
}

func eval_mul(vals []big.Int) big.Int {
	var val *big.Int = big.NewInt(1)
	//
	for _, v := range vals {
		val = val.Mul(val, &v)
	}
	//
	return *val
}

func eval_sub(vals []big.Int) big.Int {
	var val *big.Int = big.NewInt(0)
	// Clone first element
	val.SetBytes(vals[0].Bytes())
	//
	for _, v := range vals[1:] {
		val = val.Sub(val, &v)
	}
	//
	return *val
}

func query_string(p Query, braces ...int) string {
	var str string
	//
	switch p.op {
	case qVAR, qROW:
		return p.name
	case qNUM:
		return p.number.String()
	case qEQ:
		str = query_strings("==", false, p.args)
	case qNEQ:
		str = query_strings("!=", false, p.args)
	case qLT:
		str = query_strings("<", false, p.args)
	case qLTEQ:
		str = query_strings("<=", false, p.args)
	case qOR:
		str = query_strings("∨", true, p.args, qOR, qAND)
	case qAND:
		str = query_strings("∧", true, p.args, qOR, qAND)
	case qADD:
		str = query_strings("+", false, p.args, qADD, qMUL, qSUB)
	case qMUL:
		str = query_strings("*", false, p.args, qADD, qMUL, qSUB)
	case qSUB:
		str = query_strings("-", false, p.args, qADD, qMUL, qSUB)
	default:
		panic(fmt.Sprintf("unknown operator (%d)", p.op))
	}
	// Check whether braces required
	if slices.Contains(braces, p.op) {
		return fmt.Sprintf("(%s)", str)
	}
	// nope
	return str
}

func query_strings(op string, spacing bool, queries []Query, braces ...int) string {
	var builder strings.Builder
	//
	for i, q := range queries {
		if i != 0 && spacing {
			builder.WriteString(" ")
			builder.WriteString(op)
			builder.WriteString(" ")
		} else if i != 0 {
			builder.WriteString(op)
		}

		builder.WriteString(query_string(q, braces...))
	}
	//
	return builder.String()
}
