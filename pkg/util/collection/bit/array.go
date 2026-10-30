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

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// Order determines the order in which the bits of a byte are indexed.  For
// example, consider the array [0x90,0x7] which is [0b10010000,0b00000111].
// Then, under a BigEndian ordering the bit offsets can be viewed as follows:
//
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 |  | 0 | 0 | 0 | 0 | 0 | 1 | 1 | 1 |
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 00| 01| 02| 03| 04| 05| 06| 07|  | 08| 09| 10| 11| 12| 13| 14| 15|
//
// In contrast, under a LittleEndian ordering, the bit offsets can be viewed as:
//
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 |  | 0 | 0 | 0 | 0 | 0 | 1 | 1 | 1 |
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 07| 06| 05| 04| 03| 02| 01| 00|  | 15| 14| 13| 12| 11| 10| 09| 08|
//
// The LittleEndian ordering is perhaps the most "natural" way to encode data
// (i.e. since bit 0 maps to index 0, etc).  Despite this, the BigEndian
// ordering is useful for data intended to be human-readable (e.g. which
// originates from a hex string). To better understand this, consider an array
// holding 4-bit nibbles.  Given the hex string "0xfedc", which nibble should
// come first (i.e. have lowest indexed bits)?  Under the BigEndian ordering,
// "0xf" is the first, followed by "0xe" and then "0xd", etc.  In contrast,
// under a LittleEndian ordering, the order is "0xe","0xf","0xc","0xd" --- which
// is somewhat confusing.
type Order bool

const (
	// BigEndian bit ordering reverses the bit indices within a given byte (e.g.
	// where bit 7 has index 0).
	BigEndian Order = true
	// LittleEndian bit ordering follows the "natural" layout of a byte (e.g.
	// where bit 0 has index 0).
	LittleEndian Order = false
)

// Array provides a mechanism for storing data in terms of bits, rather than bytes.
type Array struct {
	// Underlying data of array
	data []byte
	// Size of array (in bits)
	size uint
}

// ArrayFromBytes constructs a bit array from a given byte slice, such that
// every bit of every byte is included in the resulting array.
func ArrayFromBytes(bytes ...byte) Array {
	// NOTE: the ordering actually doesn't matter here since all bits are
	// included in the resulting array.
	return NewArray(bytes, uint(len(bytes))*8, BigEndian)
}

// ZeroArray constructs an array holding a given number of (zeroed) bits.
// Observe that this construct guarantees the result of Bytes() is not nil.
func ZeroArray(nbits uint) Array {
	var (
		// Determine how many bytes required
		nbytes = BytesRequiredFor(nbits)
	)
	//
	return Array{make([]byte, nbytes), nbits}
}

// NewArray constructs a new array containing the first n bits from a given
// slice of bytes.  Observe that this does not clone the slice in question and,
// hence, operations on this array may mutate the slice.
func NewArray(bytes []byte, nbits uint, order Order) Array {
	var (
		// Determine how many bytes required
		nbytes = BytesRequiredFor(nbits)
		// Determine maxBits bits present
		maxBits = nbytes * 8
	)
	//
	util.Assert(nbits <= uint(len(bytes))*8, "slice has insufficient bits")
	// Slice down
	bytes = bytes[:nbytes]
	// clear top bits
	for i := nbits; i < maxBits; i++ {
		if order == BigEndian {
			BigEndianWrite(false, bytes, i)
		} else {
			LittleEndianWrite(false, bytes, i)
		}
	}
	//
	return Array{bytes, nbits}
}

// ParseArray constructs a new array from an (extended) hex representation of
// this array (assuming a BigEndian ordering), or returns an error.
func ParseArray(text string) (Array, error) {
	var (
		splits    = strings.Split(text, "~")
		data, err = hex.DecodeString(splits[0])
		nbits     = uint(len(data) * 8)
	)
	//
	if err != nil {
		return Array{}, err
	} else if len(splits) > 2 {
		return Array{}, fmt.Errorf("malformed extended hex string: \"%s\"", text)
	} else if len(splits) != 1 {
		var mask uint64
		// parse mask
		if mask, err = strconv.ParseUint(splits[1], 10, 3); err != nil {
			return Array{}, err
		} else if len(data) == 0 {
			return Array{}, fmt.Errorf("malformed extended hex string: \"%s\"", text)
		} else if mask > 7 || mask == 0 {
			return Array{}, fmt.Errorf("malformed extended hex string (invalid mask): \"%s\"", text)
		} else if rest := data[len(data)-1] << mask; rest != 0 {
			return Array{}, fmt.Errorf("malformed extended hex string (non-zero masked bits): \"%s\"", text)
		}
		//
		nbits = nbits - 8 + uint(mask)
	}
	//
	return Array{data, nbits}, nil
}

// Append the first n bits of data from the given slice onto the end of this
// array (i.e. starting at bit offset Len()) assuming a given bit ordering,
// whilst expanding this array as required.
func (p *Array) Append(buf []byte, nbits uint, order Order) {
	var (
		offset = p.size
		// Determine how many bytes are required in total
		totalBytes = BytesRequiredFor(p.size + nbits)
		// Determine how many additional bytes required
		extraBytes = int(totalBytes) - len(p.data)
	)
	// Expand underlying slice to required size
	p.data = slices.Grow(p.data, extraBytes)[:totalBytes]
	// Updated recorded size
	p.size += nbits
	// Write data
	p.Write(offset, buf, nbits, order)
}

// Bytes returns a slice to the internal bytes making up this this array.
// Observe that this can return nil if this array was constructed with a nil
// slice.
func (p Array) Bytes() []byte {
	return p.data
}

// Equals checks whether this array matches another array identically.
func (p Array) Equals(o Array) bool {
	return p.size == o.size && bytes.Equal(p.data, o.data)
}

// Len returns the number of bits held in this array.
func (p Array) Len() uint {
	return p.size
}

// String returns an extended hex string representation of this array
// (excluding "0x") assuming a BigEndian ordering.
func (p Array) String() string {
	var (
		tail = p.size % 8
		hex  = hex.EncodeToString(p.data)
	)
	//
	if tail != 0 {
		return fmt.Sprintf("%s~%d", hex, tail)
	}
	//
	return hex
}

// Read n bits of data into the given slice from this array starting at the
// given offset, assuming a given bit ordering.
func (p Array) Read(offset uint, buf []byte, nbits uint, order Order) {
	if order {
		BigEndianCopy(p.data, offset, buf, 0, nbits)
	} else {
		LittleEndianCopy(p.data, offset, buf, 0, nbits)
	}
}

// Write the first n bits of data from the given slice into this array starting
// at the given offset, assuming a given bit ordering.
func (p Array) Write(offset uint, buf []byte, nbits uint, order Order) {
	util.Assert(offset+nbits <= p.size, "write out-of-bounds")
	//
	if order {
		BigEndianCopy(buf, 0, p.data, offset, nbits)
	} else {
		LittleEndianCopy(buf, 0, p.data, offset, nbits)
	}
}

// Decode decodes this array of bits into as slice of arbitrary fixed-width
// values.  Observe that values are assumed to be packed tightly (i.e. without
// any padding).  Consider the following input byte array:
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
//
// Then, decoding this into a u4 array will produce the following:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// Finally, the number of unused (i.e. remaining) bits is returned (which will
// be zero if (len(bytes)*8)%bitwidth == 0).
//
// NOTE: the array parsed into the decoder function is reused across different
// elements and, hence, the decoder function should clone it if necessary.
func (p Array) Decode[T any](offset, bitwidth uint, order Order, decoder func([]byte) T) ([]T, uint) {
	var (
		// Calculate size of final array
		nelems = (p.size - offset) / bitwidth
		// Construct buffer of sufficient width
		buffer = NewBuffer(bitwidth)
		// Preallocate final array
		values = make([]T, nelems)
	)
	// Keep going whilst at least one element left
	for index := 0; offset+bitwidth <= p.size; index++ {
		p.Read(offset, buffer, bitwidth, order)
		values[index] = decoder(buffer)
		offset = offset + bitwidth
	}
	// Done
	return values, p.size - offset
}
