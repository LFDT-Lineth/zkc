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
package math

import (
	"fmt"
	"math/big"
)

// INFINITY represents the interval which encloses all other intervals.
var INFINITY Interval = Interval{NegInfinity, PosInfinity}

// Interval provides a discrete range of integers, such as 0..1, 1..18, etc.  An
// interval can be used to approximate the possible values that a given
// expression could evaluate to.  An interval can additionally represent three
// different forms of infinity: negative infinity, positive infinity and plain
// infinity.  The latter contains both negative and positive infinities.  For
// more information on this system, see the following paper:
//
// Integer Range Analysis for Whiley on Embedded Systems, David J. Pearce.  In
// Proceedings of the IEEE/IFIP Workshop on Software Technologies for Future
// Embedded and Ubiquitous Systems (SEUS), pages 26--33, 2015.
type Interval struct {
	min InfInt
	max InfInt
}

// NewInterval creates an interval representing a given range.
func NewInterval(lower big.Int, upper big.Int) Interval {
	var (
		min InfInt
		max InfInt
	)
	// sanity check
	if lower.Cmp(&upper) > 0 {
		panic("invalid interval")
	}
	//
	min.SetInt(lower)
	max.SetInt(upper)
	//
	return Interval{min, max}
}

// NewInterval64 creates an interval representing a given range.
func NewInterval64(lower int64, upper int64) Interval {
	return NewInterval(*big.NewInt(lower), *big.NewInt(upper))
}

// Constant checks whether this interval represents a constant value and, if so,
// returns it.  Otherwise, it returns nil.
func (p *Interval) Constant() *big.Int {
	if p.IsFinite() {
		minv, maxv := p.min.IntVal(), p.max.IntVal()
		// Check whether constant
		if minv.Cmp(&maxv) == 0 {
			// Yes
			return &minv
		}
	}
	//
	return nil
}

// IsFinite determines whether or not this interval represents an a finite value
// (i.e. not an infinity).
func (p *Interval) IsFinite() bool {
	return p.min.IsNotAnInfinity() && p.max.IsNotAnInfinity()
}

// IsInfinite determines whether or not this interval represents an infinity.
func (p *Interval) IsInfinite() bool {
	return !p.IsFinite()
}

// MinValue returns the minimum value that this interval includes.
func (p *Interval) MinValue() InfInt {
	return p.min
}

// MinIntValue returns the minimum value that this interval includes.  Note this
// will panic if the interval is infinite.
func (p *Interval) MinIntValue() big.Int {
	return p.min.IntVal()
}

// MaxValue returns the maximum value that this interval includes.
func (p *Interval) MaxValue() InfInt {
	return p.max
}

// MaxIntValue returns the maximum value that this interval includes.  Note this
// will panic if the interval is infinite.
func (p *Interval) MaxIntValue() big.Int {
	return p.max.IntVal()
}

// BitWidth returns the minimum number of bits required to store all elements in
// this interval.  Observe that, if the interval can contain negative numbers
// then it is considered to be "signed", and the bitwidth returned the maximum
// of either the positive or negative sides.  Note: this method will panic if
// called with the infinite interval.
func (p *Interval) BitWidth() (width uint, signed bool) {
	if p.IsInfinite() {
		panic("cannot determine bitwidth of infinite interval")
	}
	//
	pMin := p.min.IntVal()
	pMax := p.max.IntVal()
	// Determine whether signed or not
	signed = pMin.Sign() < 0
	//
	if signed {
		// In signed arithmetic, we can represent e.g. -16..15 in 4bits of
		// magnitude plus the sign bit.
		pMin.Add(&pMin, big.NewInt(1))
	}
	// Done
	return uint(max(pMin.BitLen(), pMax.BitLen())), signed
}

// Set assigns a given value to this interval.  Note: this method will panic if
// called with the infinite interval.
func (p *Interval) Set(val Interval) {
	p.min.Set(val.min)
	p.max.Set(val.max)
}

// Contains checks whether a given value is contained with this interval
func (p *Interval) Contains(val big.Int) bool {
	return p.min.CmpInt(val) <= 0 && p.max.CmpInt(val) >= 0
}

// Within checks whether this interval is contained within the given bounds.
func (p *Interval) Within(val Interval) bool {
	return p.min.Cmp(val.min) >= 0 && p.max.Cmp(val.max) <= 0
}

// Insert a given value into this interval
func (p *Interval) Insert(val Interval) {
	// Lower bound
	p.min = p.min.Min(val.min)
	// Upper bound
	p.max = p.max.Max(val.max)
}

// Add two intervals together
func (p *Interval) Add(q Interval) {
	// lower bound
	p.min = p.min.Add(q.min)
	// upper bound
	p.max = p.max.Add(q.max)
	// normalise bounds
	p.normalise()
}

// Sub subtracts another interval from this.
func (p *Interval) Sub(q Interval) {
	// lower bound
	p.min = p.min.Sub(q.max)
	// upper bound
	p.max = p.max.Sub(q.min)
	// normalise bounds
	p.normalise()
}

// Mul multiplies this interval by another.
func (p *Interval) Mul(q Interval) {
	x1 := p.min.Mul(q.min)
	x2 := p.min.Mul(q.max)
	x3 := p.max.Mul(q.min)
	x4 := p.max.Mul(q.max)
	//
	x1_m_x2 := x1.Min(x2)
	x3_m_x4 := x3.Min(x4)
	x1_x_x2 := x1.Max(x2)
	x3_x_x4 := x3.Max(x4)
	// Compute min / max
	min := x1_m_x2.Min(x3_m_x4)
	max := x1_x_x2.Max(x3_x_x4)
	//
	p.min.Set(min)
	p.max.Set(max)
}

// Union returns the set union of two intervals.
func (p *Interval) Union(other Interval) Interval {
	return Interval{p.min.Min(other.min), p.max.Max(other.max)}
}

func (p *Interval) String() string {
	return fmt.Sprintf("(%s..%s)", p.min.String(), p.max.String())
}

func (p *Interval) normalise() {
	if p.min.sign == infinity {
		p.min = NegInfinity
	}
	// normalise upper bound
	if p.max.sign == infinity {
		p.max = PosInfinity
	}
}
