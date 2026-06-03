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
package poly

import (
	"cmp"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/bexp"
)

// Parse a given input string representing a polynomial, or produce one or more
// errors.
func Parse(input string) (*ArrayPoly[Var], []source.SyntaxError) {
	var env = func(string) bool { return true }
	// Parse input
	term, errs := bexp.Parse[polyTerm](input, env)
	// Sanity check errors
	if len(errs) > 0 {
		return nil, errs
	}
	//
	return &term.poly, nil
}

// Var is a wrapper around a string
type Var struct {
	name string
}

// Cmp implementation for Comparable interface
func (p Var) Cmp(o Var) int {
	return cmp.Compare(p.name, o.name)
}

func (p Var) String(func(string) string) string {
	return p.name
}

// =========================================================================================

type polyTerm struct {
	poly ArrayPoly[Var]
}

func (p polyTerm) Variable(v string) polyTerm {
	var (
		poly ArrayPoly[Var]
		one  = big.NewInt(1)
	)
	//
	poly.AddTerm(NewMonomial(*one, Var{v}))
	//
	return polyTerm{poly}
}

func (p polyTerm) Number(v big.Int) polyTerm {
	var poly ArrayPoly[Var]
	//
	poly.AddTerm(NewMonomial[Var](v))
	//
	return polyTerm{poly}
}

// Arithmetic
func (p polyTerm) Add(terms ...polyTerm) polyTerm {
	var poly = &p.poly
	//
	for _, q := range terms {
		poly = poly.Add(&q.poly)
	}
	//
	return polyTerm{*poly}
}

func (p polyTerm) Mul(terms ...polyTerm) polyTerm {
	var poly = &p.poly
	//
	for _, q := range terms {
		poly = poly.Mul(&q.poly)
	}
	//
	return polyTerm{*poly}
}

func (p polyTerm) Sub(terms ...polyTerm) polyTerm {
	var poly = &p.poly
	//
	for _, q := range terms {
		poly = poly.Sub(&q.poly)
	}
	//
	return polyTerm{*poly}
}

func (p polyTerm) Or(terms ...polyTerm) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) And(terms ...polyTerm) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) Truth(val bool) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) Equals(o polyTerm) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) NotEquals(o polyTerm) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) LessThan(polyTerm) polyTerm {
	panic("unsupported operation")
}

func (p polyTerm) LessThanEquals(polyTerm) polyTerm {
	panic("unsupported operation")
}
