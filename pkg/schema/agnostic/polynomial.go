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
package agnostic

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
)

// Environment provides a generic mechanism for associating the bitwidth of a
// register with its ID.
type Environment[T RegisterIdentifier[T]] func(T) uint

// StaticPolynomial represents a polynomial over registers on the current row.
// In other words, a polynomial which cannot refer to a register on a different
// (i.e. relative) row.
type StaticPolynomial = Polynomial[register.Id]

// StaticMonomial defines the type of monomials contained within a given (static) polynomial.
type StaticMonomial = Monomial[register.Id]

// DynamicPolynomial represents a polynomial over "relative registers".  That
// is, it can refer to registers on the current row or on a row relative to the
// current row (e.g. the next row, or the previous row, etc).
type DynamicPolynomial = Polynomial[register.AccessId]

// DynamicMonomial defines the type of monomials contained within a given (relative) polynomial.
type DynamicMonomial = Monomial[register.AccessId]

// Polynomial defines the type of polynomials over which packets (and register
// splitting in general) operate.
type Polynomial[T util.Comparable[T]] = *poly.ArrayPoly[T]

// Monomial defines the type of monomials contained within a given polynomial.
type Monomial[T util.Comparable[T]] = poly.Monomial[T]

// RegisterIdentifier enables functions which are generic over the identifier
// used in a polynomial (either relative or not relative, etc).
type RegisterIdentifier[T any] interface {
	util.Comparable[T]
	// Id returns the underlying register id for this identifier.
	Id() register.Id
}

// DynamicEnvironment provides a simple implementation of Environment for rich
// register accesses.
func DynamicEnvironment() Environment[register.AccessId] {
	return func(rid register.AccessId) uint {
		return rid.MaskWidth()
	}
}

// StaticEnvironment constructs an environment from a register map.
func StaticEnvironment(mapping register.Map) Environment[register.Id] {
	return func(rid register.Id) uint {
		return mapping.Register(rid.Id()).Width()
	}
}

// ArrayEnvironment constructs an environment from a register array.
func ArrayEnvironment(registers []register.Register) Environment[register.Id] {
	return func(rid register.Id) uint {
		return registers[rid.Id().Unwrap()].Width()
	}
}

// WidthOfPolynomial determines the minimum number of bits required to store
// all possible evaluations of this polynomial.  Observe that, in the case of
// negative values, this must include the sign bit as well.  For example, a
// polynomial contained within the range 0..255 has a width of 8 bits. Likewise,
// a polynomial contained within the range -17 .. 15 has a width of 6bits.  To
// understand this, consider that the positive component (0..15) has a width of
// 4 and the negative component (-17..-1) a width of 5.  Since a sign bit is
// needed to distinguish the two cases, we have an overall width of 6 bits
// required for the polynomial.
//
// To determine the bitwidth of a polynomial, this function first determines its
// smallest enclosing integer range.  From this is then determines the required
// widths of the negative and positive components, before combining them to give
// the result.
func WidthOfPolynomial[T RegisterIdentifier[T]](source Polynomial[T], env Environment[T],
) (bitwidth uint, signed bool) {
	bitwidth, signed = RawWidthOfPolynomial(source, env)
	// Adjust to include the sign bit for signed values.
	if signed {
		bitwidth++
	}
	//
	return bitwidth, signed
}

// RawWidthOfPolynomial is essentially the same as WidthOfPolynomial, but does
// not adjust the returned width to include a sign bit.
func RawWidthOfPolynomial[T RegisterIdentifier[T]](source Polynomial[T], env Environment[T],
) (bitwidth uint, signed bool) {
	//
	var (
		intRange  = IntegerRangeOfPolynomial(source, env)
		lower     = intRange.MinIntValue()
		upper     = intRange.MaxIntValue()
		upperBits = uint(upper.BitLen())
	)
	// Check whether negative range in play.
	if lower.Sign() < 0 {
		// NOTE: this accounts for the fact that, on the negative side, we get
		// an extra value.  For example, with signed 8bit values the range is
		// -128 upto 127.
		lowerBits := uint(lower.Add(&lower, &one).BitLen())
		// Yes, we have negative value but we don't adjust to include a sign bit
		// (in this case).
		return max(lowerBits, upperBits), true
	}
	// No sign bit required.
	return upperBits, false
}

// WidthOfMonomial determines the minimum number of bits required to store all
// possible evaluations of this mononomial.  To determine the bitwidth of a
// monomial, this function first determines its smallest enclosing integer
// range.
func WidthOfMonomial[T RegisterIdentifier[T]](source Monomial[T], env Environment[T],
) (bitwidth uint, signed bool) {
	//
	var (
		intRange  = IntegerRangeOfMonomial(source, env)
		lower     = intRange.MinIntValue()
		upper     = intRange.MaxIntValue()
		upperBits = uint(upper.BitLen())
	)
	// Check whether negative mononial
	if lower.Sign() < 0 {
		// NOTE: this accounts for the fact that, on the negative side, we get
		// an extra value.  For example, with signed 8bit values the range is
		// -128 upto 127.
		lowerBits := uint(lower.Add(&lower, &one).BitLen())
		// Yes, we have negative value but we don't adjust to include a sign bit
		// (in this case).
		return max(lowerBits, upperBits), true
	}
	// Positive mononial
	return upperBits, false
}

// SplitWidthOfPolynomial resturns the number of bits required for all positive
// values, along with the number of bits required for all negative values.
// Observe that, unlike WidthOfPolynomial, this does not account for an
// additional sign bit.
func SplitWidthOfPolynomial(source StaticPolynomial, env Environment[register.Id]) (poswidth uint, negwidth uint) {
	var (
		intRange  = IntegerRangeOfPolynomial(source, env)
		lower     = intRange.MinIntValue()
		upper     = intRange.MaxIntValue()
		upperBits = uint(upper.BitLen())
	)
	// Check whether negative range in play.
	if lower.Sign() < 0 {
		// NOTE: this accounts for the fact that, on the negative side, we get
		// an extra value.  For example, with signed 8bit values the range is
		// -128 upto 127.
		lowerBits := uint(lower.Add(&lower, &one).BitLen())
		// Yes, we have negative values.  This mandates the need for an
		// additional signbit.
		return upperBits, lowerBits
	}
	//
	return upperBits, 0
}

// IntegerRangeOfPolynomial determines the smallest integer range in which all
// evaluations of this polynomial lie.  For example, consider "2*X + 1" where X
// is an 8bit register.  Then, the smallest integer range which includes this
// polynomial is "0..511".
func IntegerRangeOfPolynomial[T RegisterIdentifier[T]](poly Polynomial[T], env Environment[T]) util_math.Interval {
	var intRange util_math.Interval
	//
	for i := range poly.Len() {
		intRange.Add(IntegerRangeOfMonomial(poly.Term(i), env))
	}
	//
	return intRange
}

// IntegerRangeOfMonomial determines the smallest integer range in which all
// evaluations of the monomial lie.  For example, consider the monomial "3*X*Y"
// where X and are 8bit and 16bit registers respectively.  Then, the smallest
// enclosing integer range is 0 .. 3*255*65535.
func IntegerRangeOfMonomial[T RegisterIdentifier[T]](mono Monomial[T], env Environment[T]) util_math.Interval {
	var (
		coeff    = mono.Coefficient()
		intRange = util_math.NewInterval(coeff, coeff)
	)
	//
	for i := range mono.Len() {
		intRange.Mul(IntegerRangeOfRegister(mono.Nth(i), env))
	}
	//
	return intRange
}

// IntegerRangeOfRegister determines the smallest integer range enclosing all possible
// values for a given register.  For example, a register of width 16 has an
// integer range of 0..65535 (inclusive).
func IntegerRangeOfRegister[T RegisterIdentifier[T]](id T, env Environment[T]) util_math.Interval {
	var (
		val   = big.NewInt(2)
		width = env(id)
	)
	// NOTE: following is safe since the width of any registers must sure be
	// less than 65536 bits :)
	val.Exp(val, big.NewInt(int64(width)), nil)
	// Subtract one since the interval is inclusive.
	return util_math.NewInterval(zero, *val.Sub(val, &one))
}

// RegistersRead returns the set of registers read by this instruction.
func RegistersRead[T RegisterIdentifier[T]](p Polynomial[T]) []register.Id {
	var (
		regs bit.Set
		read []register.Id
	)
	//
	for i := range p.Len() {
		for _, ident := range p.Term(i).Vars() {
			rid := ident.Id()
			//
			if !regs.Contains(rid.Unwrap()) {
				regs.Insert(rid.Unwrap())
				read = append(read, rid)
			}
		}
	}
	//
	return read
}

// RegisterReadSet returns the set of registers read by this instruction.
func RegisterReadSet[T RegisterIdentifier[T]](p Polynomial[T]) bit.Set {
	var regs bit.Set
	//
	for i := range p.Len() {
		for _, ident := range p.Term(i).Vars() {
			rid := ident.Id()
			regs.Insert(rid.Unwrap())
		}
	}
	//
	return regs
}

// SubstitutePolynomial replaces all occurrences of a given variable with a set
// of (zero or more) variables (e.g. typically used for substituting limbs).
func SubstitutePolynomial[T RegisterIdentifier[T]](p Polynomial[T], mapping func(T) Polynomial[T]) (r Polynomial[T]) {
	//
	for i := range p.Len() {
		ith := SubstituteMonomial(p.Term(i), mapping)
		//
		if i == 0 {
			r = ith
		} else {
			r = r.Add(ith)
		}
	}
	// Done
	return r
}

// SubstituteMonomial replaces all occurrences of a given variable with a set of
// (zero or more) variables (e.g. typically used for substituting limbs).
func SubstituteMonomial[T RegisterIdentifier[T]](t Monomial[T], mapping func(T) Polynomial[T]) Polynomial[T] {
	var (
		r Polynomial[T]
	)
	// Initialise
	r = r.Set(poly.NewMonomial[T](t.Coefficient()))
	// Initially, attempt to avoid substitution altgoether.  This ensures we
	// only allocate memory when an actual subistition happens.
	for _, v := range t.Vars() {
		tmp := mapping(v)
		// Sanity check what happened
		if tmp == nil {
			tmp = tmp.Set(poly.NewMonomial(one, v))
		}
		//
		r = r.Mul(tmp)
	}
	// No substitution required
	return r
}

// LimbPolynomial constructs a polynomial representing the combined value of all
// limbs according to their given bitwidths.  For example, given [l0, l1, l2]
// with limbs widths [8,8,2] the resulting polynomial is: l0 + 2^8*l1 + 2^8*l2.
func LimbPolynomial(bitwidth uint, shift int, limbs []register.Id, widths []uint) (p DynamicPolynomial) {
	var (
		terms []DynamicMonomial
		width uint
	)
	//
	for i, limb := range limbs {
		var (
			c         = util_math.Pow2(width)
			limbWidth = min(bitwidth, widths[i])
		)
		//
		if limbWidth > 0 {
			terms = append(terms, poly.NewMonomial(*c, limb.AccessOf(widths[i]).Shift(shift).Mask(limbWidth)))
			width += limbWidth
		}
		//
		bitwidth -= limbWidth
	}
	//
	return p.Set(terms...)
}

// StaticLimbPolynomial constructs a polynomial representing the combined value of all
// limbs according to their given bitwidths.  For example, given [l0, l1, l2]
// with limbs widths [8,8,2] the resulting polynomial is: l0 + 2^8*l1 + 2^8*l2.
func StaticLimbPolynomial(bitwidth uint, limbs []register.Id, widths []uint) (p StaticPolynomial) {
	var (
		terms []StaticMonomial
		width uint
	)
	//
	for i, limb := range limbs {
		var (
			c         = util_math.Pow2(width)
			limbWidth = min(bitwidth, widths[i])
		)
		//
		if limbWidth > 0 {
			terms = append(terms, poly.NewMonomial(*c, limb))
			width += limbWidth
		}
		//
		bitwidth -= limbWidth
	}
	//
	return p.Set(terms...)
}
