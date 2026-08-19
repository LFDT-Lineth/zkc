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
package bit

// BigEndianCopy copies n bits starting a given bit offset from a given byte
// array source into a given destination (at a given offset) assuming a big
// endian layout of bytes.  For example, consider the array [0x90,0x7] which is
// [0b10010000,0b00000111].  Then, the bit offsets can be viewed as follows:
//
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 |  | 0 | 0 | 0 | 0 | 0 | 1 | 1 | 1 |
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 00| 01| 02| 03| 04| 05| 06| 07|  | 08| 09| 10| 11| 12| 13| 14| 15|
//
// Now, consider copying 8 bits starting at offset 3.  This represents the
// following bits:
//
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// |   |   |   | X | X | X | X | X |  | X | X | X |   |   |   |   |   |
// +---+---+---+---+---+---+---+---+  +---+---+---+---+---+---+---+---+
// | 00| 01| 02| 03| 04| 05| 06| 07|  | 08| 09| 10| 11| 12| 13| 14| 15|
//
// As such, we see how the big end treatment of bytes impacts the bits which are
// copied.
func BigEndianCopy(src []byte, srcOffset uint, dst []byte, dstOffset uint, nbits uint) {
	// Check for aligned read / write
	if srcOffset%8 == 0 && dstOffset%8 == 0 {
		var (
			srcByteOffset = srcOffset / 8
			dstByteOffset = dstOffset / 8
			nBytes        = nbits / 8
		)
		// Copy bytes
		copy(dst[dstByteOffset:dstByteOffset+nBytes], src[srcByteOffset:srcByteOffset+nBytes])
		// Calculate residue
		nbits = nbits % 8
		srcOffset += nBytes * 8
		dstOffset += nBytes * 8
	}
	// Continue with any remaining
	for i := range nbits {
		ith := BigEndianRead(src, srcOffset+i)
		BigEndianWrite(ith, dst, dstOffset+i)
	}
}

// BigEndianRead reads the bit at a given bit offset out of an array of bytes
// arranged in big endian format.  So, for example, reading bit 0 from the byte
// 0b0111_11111 returns 0, but reading bit 7 returns 1.
func BigEndianRead(src []byte, bitoffset uint) bool {
	var (
		byte = bitoffset / 8
		bit  = bitoffset % 8
		mask = uint8(1) << (7 - bit)
	)
	//
	return src[byte]&mask != 0
}

// BigEndianWrite writes a bit to a given bit offset in an array of bytes
// arranged in big endian format.  So, for example, writing 1 at offset 15 into
// an array [0x00,0x00] yields [0x00,0x01].
func BigEndianWrite(val bool, src []byte, bitoffset uint) {
	var (
		byte = bitoffset / 8
		bit  = bitoffset % 8
		mask = uint8(1) << (7 - bit)
	)
	//
	if val {
		// set bit
		src[byte] = src[byte] | mask
	} else {
		// Clear bit
		src[byte] = src[byte] & ^mask
	}
}
