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
package bit

// Reader provides a mechanism for reading bits from a given array of bytes,
// where the least significant bits are read first.  For example, consider
// sequence of bytes [0x9f,0x05] can be views as the following bit sequence:
//
// | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 || 8 | 9 | A | B | C | D | E | F |
// +===+===+===+===+===+===+===+===++===+===+===+===+===+===+===+===+
// | 1 | 1 | 1 | 1 | 1 | 0 | 0 | 1 || 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 |
// |   |   |   |   |
// | 1 | 1 | 1 | 1 | 1 | 0 | 0 |
//
// The above illustrates the outcome from reading 7 bits.  In such
// case, the value 0b0011111 is written into the target buffer.
type Reader struct {
	bitoffset uint
	bytes     []byte
}

// NewReader constructs a new bit reader.
func NewReader(bytes []byte) Reader {
	return Reader{0, bytes}
}

// Remaining returns the remaining number of bits which can be read.
func (p *Reader) Remaining() uint {
	var n = uint(len(p.bytes) * 8)
	//
	return n - p.bitoffset
}

// LittleEndianReadInto reads the n least significant bits from the underlying
// array into a given target array, returning the total number of bytes
// affected.  This assume a little endian encoding of bytes.
func (p *Reader) LittleEndianReadInto(nbits uint, buf []byte) uint {
	var nread = nbits / 8
	// Determine how many bytes affected.
	if nbits%8 != 0 {
		// Clear final byte
		buf[nread] = 0
		nread++
	}
	//
	LittleEndianCopy(p.bytes, p.bitoffset, buf, 0, nbits)
	//
	p.bitoffset += nbits
	//
	return nread
}

// BigEndianReadInto reads the n least significant bits from the underlying
// array into a given target array, returning the total number of bytes
// affected.  This assume a big endian encoding of bytes.
func (p *Reader) BigEndianReadInto(nbits uint, buf []byte) uint {
	var nread = nbits / 8
	// Determine how many bytes affected.
	if nbits%8 != 0 {
		// Clear final byte
		buf[nread] = 0
		nread++
	}
	//
	BigEndianCopy(p.bytes, p.bitoffset, buf, 0, nbits)
	//
	p.bitoffset += nbits
	//
	return nread
}
