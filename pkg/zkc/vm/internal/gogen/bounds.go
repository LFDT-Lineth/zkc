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

package gogen

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// operand is a value read rendered as Go expression(s), together with what the
// generator knows statically about its runtime value.  Bounds derive from the
// frame invariant that every register always satisfies its declared bit width
// (stores are checked), sharpened by the interval analysis.
//
// A value up to 64 bits is a single expression (expr); a wider value (a
// two-limb register, or an intermediate that may exceed 64 bits) additionally
// carries its high limb in hi.  Bounds drive which representation a result
// needs and whether a store-width check can be omitted.
type operand struct {
	expr string // the value, or its low 64 bits when hi is set
	hi   string // high limb expression ("" for narrow values)
	// max is an inclusive upper bound on the runtime value.
	max *big.Int
	// val is the exact value when known at generation time, and nil otherwise.
	val *big.Int
}

// exact constructs an operand with a generation-time known value (< 2^64).
// The expression carries an explicit uint64 conversion: a bare literal in a
// short variable declaration (`lo, hi := 1, uint64(0)`) would type as int and
// break the bits.Add64/Sub64 chains it feeds.
func exact(v *big.Int) operand {
	return operand{expr: "uint64(" + v.String() + ")", max: v, val: v}
}

// isZero reports whether the operand is the constant zero.
func (o operand) isZero() bool { return o.val != nil && o.val.Sign() == 0 }

// wide reports whether the operand is represented as a lo/hi pair.
func (o operand) wide() bool { return o.hi != "" }

// hiOr0 is the high-limb expression, "0" for narrow operands.
func (o operand) hiOr0() string {
	if o.hi == "" {
		return "0"
	}

	return o.hi
}

// operand returns an operand reading a source register, bounded by the
// interval analysis (which never exceeds the declared width).  A zero bound
// means the register provably holds 0 — the value is then exact, which lets
// emitters drop dead terms.  A two-limb register reads as a pair UNLESS its
// bound proves the high limb zero, in which case it collapses to its low limb
// (common for the wide-typed-but-small temporaries the lowering passes
// introduce).  Note that "const" (zero/one) registers read as plain frame
// values — the reference machine zero-initialises frames and never installs
// their nominal constant, so they are NOT folded to literals here (their
// interval does that, soundly, instead).
func (g *generator) operand(fn *wordFunction, id register.Id) (operand, error) {
	w, err := g.regWidth(fn, id)
	if err != nil {
		return operand{}, err
	}

	bound := bigMin(g.iv.boundOf(id), widthMax(w))

	op := operand{expr: reg(id), max: bound}
	if w > 64 {
		op.expr = reg(id) + "_0"
		if !fitsU64(bound) {
			op.hi = reg(id) + "_1"
		}
	}

	if bound.Sign() == 0 {
		op.val = bound
	}

	return op, nil
}

func (g *generator) operands(fn *wordFunction, ids []register.Id) ([]operand, error) {
	out := make([]operand, len(ids))

	for i, id := range ids {
		o, err := g.operand(fn, id)
		if err != nil {
			return nil, err
		}

		out[i] = o
	}

	return out, nil
}

// allExact reports whether every operand has a generation-time known value.
func allExact(ops []operand) bool {
	for _, o := range ops {
		if o.val == nil {
			return false
		}
	}

	return true
}

func exprsOf(ops []operand) []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.expr
	}

	return out
}

// widthMax returns 2^w - 1, the largest value a w-bit register can hold.
func widthMax(w uint) *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), w)

	return max.Sub(max, big.NewInt(1))
}

// fits reports whether the bound stays within w bits (i.e. a width-w store of
// a value with this bound can never fail).
func fits(bound *big.Int, w uint) bool {
	return bound.BitLen() <= int(w)
}

// fitsU64 / fitsU128 report whether values with this bound fit the single- and
// pair-word representations respectively.
func fitsU64(bound *big.Int) bool  { return bound.BitLen() <= 64 }
func fitsU128(bound *big.Int) bool { return bound.BitLen() <= 128 }
