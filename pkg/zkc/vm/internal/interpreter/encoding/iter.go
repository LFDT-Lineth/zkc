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
package encoding

// Operands provides a way of iterating operands packed into u32 words without
// allocating memory.  Operands are either u8 (four per word, as used by narrow
// instruction forms) or u16 (two per word, as used by wide instruction forms);
// the element width is fixed at construction.
type Operands struct {
	count  uint
	offset uint
	// Number of operands packed into each u32 word (4 for u8 operands, 2 for
	// u16 operands).
	perWord uint
	data    []uint32
}

// NewOperands constructs an iterator over u8 operands (packed four per word)
// from a given array of words and starting position.
func NewOperands(n, len uint, data []uint32) Operands {
	return Operands{
		offset:  n % 4,
		count:   len,
		perWord: 4,
		data:    data[n/4:],
	}
}

// NewWideOperands constructs an iterator over u16 operands (packed two per word)
// from a given array of words and starting position.
func NewWideOperands(n, len uint, data []uint32) Operands {
	return Operands{
		offset:  n % 2,
		count:   len,
		perWord: 2,
		data:    data[n/2:],
	}
}

// HasNext determines whether there are any more operands in this iterator.
func (p *Operands) HasNext() bool {
	return p.count != 0
}

// Next returns the next operand in this iterator.
func (p *Operands) Next() (operand uint16) {
	var (
		bits = 32 / p.perWord
		mask = (uint32(1) << bits) - 1
	)
	//
	operand = uint16((p.data[0] >> (p.offset * bits)) & mask)
	//
	p.count--
	p.offset = (p.offset + 1) % p.perWord
	//
	if p.offset == 0 {
		p.data = p.data[1:]
	}
	// Done
	return operand
}

// ConstFlag returns the bit which, when set in the length half of a packed
// (base, len) operand pair, marks that pair as denoting a run of constant pool
// entries (base = pool index) rather than a register vector.  The flag depends
// on the element width of this iterator (u8 for narrow forms, u16 for wide).
func (p *Operands) ConstFlag() uint16 {
	if p.perWord == 2 {
		return 0x8000
	}
	//
	return 0x80
}

// NextOperand reads the next (base, n) pair from this iterator, reporting
// whether it denotes a run of n constant pool entries starting at pool index
// base (isConst true) or a register vector of n registers starting at register
// base (isConst false).
func (p *Operands) NextOperand() (base, n uint16, isConst bool) {
	var (
		flag = p.ConstFlag()
		l    uint16
	)
	//
	base = p.Next()
	l = p.Next()
	//
	return base, l &^ flag, l&flag != 0
}

// OpIterToArray extracts n elements from the given iterator into an array.
func OpIterToArray[T uint8 | uint16](iter Operands) []T {
	var arr = make([]T, iter.count)
	//
	for i := range iter.count {
		arr[i] = T(iter.Next())
	}
	///
	return arr
}
