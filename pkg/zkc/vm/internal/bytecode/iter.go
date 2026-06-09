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
package bytecode

// Op8Iter provides a way of iterating u8 operands packed into u32 words without
// allocating memory.
type Op8Iter struct {
	offset uint8
	data   []uint32
}

// NewOp8Iter constructs a new register iterator from a given array of words and
// starting position.
func NewOp8Iter(n uint8, data []uint32) Op8Iter {
	var (
		i = n / 4
		j = n % 4
	)
	//
	return Op8Iter{
		offset: j,
		data:   data[i:],
	}
}

// Next returns the next register in this iterator.
func (p *Op8Iter) Next() (operand uint8) {
	operand = uint8(p.data[0] >> (p.offset * 8))
	//
	p.offset = (p.offset + 1) % 4
	//
	if p.offset == 0 {
		p.data = p.data[1:]
	}
	// Done
	return operand
}

// OpIterToArray extracts n elements from the given iterator into an array.
func OpIterToArray[T uint8 | uint16](n uint, iter Op8Iter) []T {
	var arr = make([]T, n)
	//
	for i := range n {
		arr[i] = T(iter.Next())
	}
	///
	return arr
}
