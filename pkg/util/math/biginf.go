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
package math

import (
	"fmt"
	"math/big"
)

const notAnInfinity = 0
const negativeInfinity = 1
const positiveInfinity = 2
const infinity = 3

// PosInfinity represents positive infinity
var PosInfinity = InfInt{big.Int{}, positiveInfinity}

// NegInfinity represents negative infinity
var NegInfinity = InfInt{big.Int{}, negativeInfinity}

// Infinity represents plain infinity
var Infinity = InfInt{big.Int{}, infinity}

// InfInt represents an unbound (i.e. big) integer value which can,
// additionally, be either negative infinity, positive infinity or just infinity
// (i.e. which covers all negative and positive values).
type InfInt struct {
	// value of this integer, or nil to signal a form of infinity.
	val big.Int
	// sign indicates whether we are not an infinity, or are negative infinity,
	// positive infinity or just plain infinity.
	sign uint8
}

// Add two (potentially infinite) integers together.
func (p *InfInt) Add(other InfInt) InfInt {
	var val big.Int
	//
	switch {
	case p.sign == notAnInfinity && other.sign == notAnInfinity:
		val.Set(&p.val)
		val.Add(&p.val, &other.val)
		//
		return InfInt{val, notAnInfinity}
	case p.sign == other.sign:
		return *p
	default:
		return Infinity
	}
}

// Cmp performs a comparison of two (potentially infinite) integer values.  This
// will panic if either value is plain infinity.
func (p *InfInt) Cmp(o InfInt) int {
	switch {
	case p.sign == infinity || o.sign == infinity:
		panic("cannot compare against infinity")
	case p.sign == notAnInfinity && o.sign == notAnInfinity:
		return p.val.Cmp(&o.val)
	case p.sign == o.sign:
		return 0
	case p.sign == negativeInfinity || o.sign == positiveInfinity:
		return -1
	case p.sign == positiveInfinity || o.sign == negativeInfinity:
		return 1
	default:
		panic(fmt.Sprintf("unreachable (%s ~ %s)", p.String(), o.String()))
	}
}

// CmpInt compares a potentially infinite integer value against a finite integer
// value.  This will panic if the first value is plain infinity.
func (p *InfInt) CmpInt(other big.Int) int {
	switch p.sign {
	case infinity:
		panic("cannot compare against infinity")
	case notAnInfinity:
		return p.val.Cmp(&other)
	case negativeInfinity:
		return -1
	case positiveInfinity:
		return 1
	default:
		panic(fmt.Sprintf("unreachable (%s ~ %s)", p.String(), other.String()))
	}
}

// IntVal converts a potentially infinite integer into a finite value.  This
// will panic if this value is an infinity.
func (p *InfInt) IntVal() big.Int {
	if p.sign != notAnInfinity {
		panic("cannot cast infinity into a big integer")
	}
	//
	return p.val
}

// IsNotAnInfinity returns true if this represents a finite integer value.
func (p *InfInt) IsNotAnInfinity() bool {
	return p.sign == notAnInfinity
}

// Min determines the least of two values.  Note the semantics here are odd, as
// the minimum of plain infinity and anything is negative infinity!
func (p *InfInt) Min(o InfInt) InfInt {
	switch {
	case p.sign == notAnInfinity && o.sign == notAnInfinity:
		if p.val.Cmp(&o.val) <= 0 {
			return *p
		}
		//
		return o
	case p.sign == positiveInfinity && o.sign == positiveInfinity:
		return PosInfinity
	default:
		return NegInfinity
	}
}

// Max determines the greatest of two values.  Note the semantics here are odd, as
// the maximum of plain infinity and anything is positive infinity!
func (p *InfInt) Max(o InfInt) InfInt {
	switch {
	case p.sign == notAnInfinity && o.sign == notAnInfinity:
		if p.val.Cmp(&o.val) >= 0 {
			return *p
		}
		//
		return o
	case p.sign == negativeInfinity && o.sign == negativeInfinity:
		return NegInfinity
	default:
		return PosInfinity
	}
}

// Mul multiplies a (potentially infinite) value against this (potentially
// infinite) value.  If either operand is an infinity, then some kind of
// infinity is always returned.
func (p *InfInt) Mul(o InfInt) InfInt {
	var val big.Int
	//
	switch {
	case p.sign == infinity || o.sign == infinity:
		return Infinity
	case p.sign == negativeInfinity && o.sign == negativeInfinity:
		return PosInfinity
	case p.sign == negativeInfinity || o.sign == negativeInfinity:
		return NegInfinity
	case p.sign == positiveInfinity || o.sign == positiveInfinity:
		return PosInfinity
	default:
		val.Set(&p.val)
		val.Mul(&p.val, &o.val)
		// Done
		return InfInt{val, notAnInfinity}
	}
}

// Negate this (potentially infinite) integer.
func (p *InfInt) Negate() InfInt {
	switch p.sign {
	case positiveInfinity:
		return NegInfinity
	case negativeInfinity:
		return PosInfinity
	case infinity:
		return Infinity
	default:
		var val big.Int
		//
		val.Neg(&p.val)
		//
		return InfInt{val, notAnInfinity}
	}
}

// Sign returns an indication whether this value is negative, zero or positive.
func (p *InfInt) Sign() int {
	if p.sign != notAnInfinity {
		panic("cannot cast infinity into a big integer")
	}
	//
	return p.val.Sign()
}

// Set this to match some (potentially infinite) integer.  Observe this will
// clone the underlying big integer if the value is finite.
func (p *InfInt) Set(other InfInt) {
	var val big.Int
	// Clone big int
	val.Set(&other.val)
	//
	p.val = val
	p.sign = other.sign
}

// SetInt sets this to match a big integer.  Observe this will clone the
// underlying big integer.
func (p *InfInt) SetInt(other big.Int) {
	var val big.Int
	// Clone big int
	val.Set(&other)
	//
	p.val = val
	p.sign = notAnInfinity
}

// Sub subtracts a (potentially infinite) value from this (potentially infinite)
// value.
func (p *InfInt) Sub(other InfInt) InfInt {
	return p.Add(other.Negate())
}

func (p *InfInt) String() string {
	switch p.sign {
	case negativeInfinity:
		return "-∞"
	case positiveInfinity:
		return "+∞"
	case infinity:
		return "∞"
	default:
		return p.val.String()
	}
}
