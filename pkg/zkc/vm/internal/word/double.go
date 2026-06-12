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
package word

import "fmt"

// Double represents a "double word".
type Double[W Word[W]] struct {
	hi, lo W
}

// AsDouble constructs a double word from a word by assigning it as the lo
// word.
func AsDouble[W Word[W]](lo W) Double[W] {
	var zero W
	return Double[W]{zero, lo}
}

// Cmp returns 1 if x > y, 0 if x = y, and -1 if x < y.
func (p Double[W]) Cmp(o Double[W]) int {
	if c := p.hi.Cmp(o.hi); c != 0 {
		return c
	}
	//
	return p.lo.Cmp(o.lo)
}

// IsZero checks whether this double word is zero, or not.
func (p Double[W]) IsZero() bool {
	return p.lo.Cmp64(0) == 0 && p.hi.Cmp64(0) == 0
}

// HalfAdd adds one word onto this double word, producing a double word (and
// overflow bit).
func (p Double[W]) HalfAdd(rhs W) (res Double[W], overflow bool) {
	if p.lo, overflow = p.lo.Add(rhs); overflow {
		p.hi, overflow = p.hi.Add64(1)
	}
	//
	return p, overflow
}

// HalfMul multiplies this double word by a given word, producing a double word
// (and overflow bit).
func (p Double[W]) HalfMul(rhs W) (res Double[W], overflow bool) {
	var v W
	//
	p.lo, v = p.lo.DwMul(rhs)
	p.hi, overflow = p.hi.Mul(v)
	//
	return p, overflow
}

// HalfSub subtracts a word from this double word by a given word, producing a
// double word (and underflow bit).
func (p Double[W]) HalfSub(rhs Word[W]) (res Double[W], overflow bool) {
	panic("todo")
}

// HiWord returns the most significant word of this double word.
func (p Double[W]) HiWord() W {
	return p.hi
}

// IsWord returns true if this double words fits into a single word (i.e. the hi
// word is zero).
func (p Double[W]) IsWord() bool {
	return p.hi.Cmp64(0) == 0
}

// LoWord returns the least significant word of this double word.
func (p Double[W]) LoWord() W {
	return p.lo
}

// Sub subtracts a word from this double word by a given word, producing a
// double word (and underflow bit).
func (p Double[W]) Sub(rhs Double[W]) (res Double[W], underflow bool) {
	var b0, b1 bool
	//
	if p.lo, b0 = p.lo.Sub(rhs.lo); b0 {
		p.hi, b0 = p.hi.Sub64(1)
	}
	//
	p.hi, b1 = p.hi.Sub(rhs.hi)
	//
	return p, b0 || b1
}

// Sbb implements "subtract with borrow" semantics.  Specifically, on underflow,
// it adds a value 2^n.  Observe that, if n > the bitwidth of a double word,
// then this operation is undefined.
func (p Double[W]) Sbb(n uint64, rhs Double[W]) Double[W] {
	var (
		tmp W
		dw  Double[W]
	)
	//
	if p.Cmp(rhs) >= 0 {
		// NOTE: underflow impossible
		dw, _ = p.Sub(rhs)
	} else {
		var hi, lo = tmp.SetUint64(1).DwShl64(n)
		// NOTE: underflow impossible
		dw, _ = rhs.Sub(p)
		// NOTE: underflow could arise here (i.e. if hi::lo==0), in which case
		// result in undefined.
		dw, _ = Double[W]{hi, lo}.Sub(dw)
	}
	//
	return dw
}

// Shr64 performs an (unsigned) shift right of this word.
func (p Double[W]) Shr64(n uint64) Double[W] {
	var w = p.hi.Slice(uint(n))
	//
	p.lo = p.lo.Shr64(n).Or(w)
	p.hi = p.hi.Shr64(n)
	//
	return p
}

// Text returns the given word formated in the given base
func (p Double[W]) Text(base int) string {
	return fmt.Sprintf("%s::%s", p.hi.Text(base), p.lo.Text(base))
}
