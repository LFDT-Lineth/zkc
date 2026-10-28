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
package word

import (
	"math"
)

// ByteWidth returns the least number of bytes required to store an element of
// the given width.
func ByteWidth(bitwidth uint) uint {
	var n = bitwidth / 8
	//
	if bitwidth%8 == 0 {
		return n
	}
	//
	return n + 1
}

// ByteWidth64 returns the bytewidth of the given uint64 value.
func ByteWidth64(value uint64) uint {
	if value > math.MaxUint32 {
		return 4 + ByteWidth32(uint32(value>>32))
	}
	//
	return ByteWidth32(uint32(value))
}

// ByteWidth32 returns the bytewidth of the given uint64 value.
func ByteWidth32(value uint32) uint {
	if value > math.MaxUint16 {
		return 2 + ByteWidth16(uint16(value>>16))
	}
	//
	return ByteWidth16(uint16(value))
}

// ByteWidth16 returns the bytewidth of the given uint16 value.
func ByteWidth16(value uint16) uint {
	if value > math.MaxUint8 {
		return 2
	} else if value > 0 {
		return 1
	}
	//
	return 0
}

// TrimLeadingZeros any leading zeros from this array
func TrimLeadingZeros(bytes []byte) []byte {
	// trim any leading zeros to ensure words are in a canonical form.
	for len(bytes) > 0 && bytes[0] == 0 {
		bytes = bytes[1:]
	}
	//
	return bytes
}
