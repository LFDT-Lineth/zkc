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

import (
	"math/big"
)

// Word abstracts the data type (a.k.a the "machine word") used for holding
// values within the machine.  The reason for abstracting this concept is to
// allow a machine compiled for a larger word size to be automatically lowered
// to a machine for a smaller word size.  For example, our source program might
// be written for a 64bit machine and we wish to executed it on 16bit machine
// (i.e. because our target field configuration has a maximum register size of
// 16bits).
type Word[W any] interface {
	// Add two words together, producing another (along with an overflow bit).
	Add(W) (W, bool)
	// AddMod adds two words together modulus a third.
	AddMod(W, W) W
	// Bitwise AND of two words.
	And(W) W
	// Returns the maximum number of bits of information a word this type can
	// hold.
	Bandwidth() uint
	// Return the value of this word as a big integer.
	BigInt() *big.Int
	// Cmp returns 1 if x > y, 0 if x = y, and -1 if x < y.
	Cmp(y W) int
	// Cmp returns 1 if x > y, 0 if x = y, and -1 if x < y.
	Cmp64(y uint64) int
	// Div divides this word by another.  Panics on division by zero.
	Div(W) W
	// Check whether this value fits within the given bitwidth.
	FitsWithin(uint) bool
	// Multiply two words together, producing another (along with an overflow bit).
	Mul(W) (W, bool)
	// MulMod multiplies two words together modulus a third.
	MulMod(W, W) W
	// Bitwise NOT of this word within the given bit width.
	Not(uint) W
	// Bitwise OR of two words.
	Or(W) W
	// Rem computes the remainder of dividing this word by another.  Panics on
	// division by zero.
	Rem(W) W
	// Shift left word by the amount given in another word, masking to width bits.
	Shl(uint, W) W
	// Shift left word by the amount given in another word, masking to width bits.
	Shl64(uint, uint64) W
	// Shift right word by the amount given in another word.
	Shr(W) W
	// Shift right word by a given number of bits.
	Shr64(uint64) W
	// Slice number of bits from this word.
	Slice(uint) W
	// Initialise this word from a given big integer (which cannot be negative).
	SetBigInt(*big.Int) W
	// Construct a fresh word with the given uint64 value, or panic (if the
	// value does not fit).
	SetUint64(uint64) W
	// Sub two words together, producing another (along with an underflow bit).
	Sub(W) (W, bool)
	// SubMod subtracts two words together modulus a third.
	SubMod(W, W) W
	// Returns value of word as an unsigned integer and will panic if the value
	// does not fit.
	Uint64() uint64
	// Bitwise XOR of two words.
	Xor(W) W
	// Text returns the given word formated in the given base
	Text(base int) string
}

// Const64 initialises a given word with a 64bit value.  This will panic if
// the given value exceeds the available bandwidth of the word in question.
func Const64[W Word[W]](val uint64) W {
	var w W
	return w.SetUint64(val)
}
