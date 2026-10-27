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
	"slices"
	"testing"
)

// ===================================================================
// Constructors
// ===================================================================

func Test_BitArray_Zero_00(t *testing.T) {
	// A zero-length array must still hold a non-nil slice (see #2299), so that
	// callers comparing Bytes() against []byte{} are not surprised.
	arr := ZeroArray(0)
	//
	if arr.Bytes() == nil {
		t.Errorf("expected non-nil slice")
	}
	//
	checkArray(t, arr, 0, []byte{}, "")
}

func Test_BitArray_Zero_01(t *testing.T) {
	checkArray(t, ZeroArray(1), 1, []byte{0}, "00:1")
}

func Test_BitArray_Zero_02(t *testing.T) {
	checkArray(t, ZeroArray(7), 7, []byte{0}, "00:7")
}

func Test_BitArray_Zero_03(t *testing.T) {
	checkArray(t, ZeroArray(8), 8, []byte{0}, "00")
}

func Test_BitArray_Zero_04(t *testing.T) {
	checkArray(t, ZeroArray(9), 9, []byte{0, 0}, "0000:1")
}

func Test_BitArray_Zero_05(t *testing.T) {
	checkArray(t, ZeroArray(17), 17, []byte{0, 0, 0}, "000000:1")
}

func Test_BitArray_FromBytes_00(t *testing.T) {
	checkArray(t, ArrayFromBytes(), 0, []byte{}, "")
}

func Test_BitArray_FromBytes_01(t *testing.T) {
	checkArray(t, ArrayFromBytes(0x31, 0xf0, 0x0e, 0x1d), 32, []byte{0x31, 0xf0, 0x0e, 0x1d}, "31f00e1d")
}

// Construction from a longer slice retains only the required prefix.
func Test_BitArray_New_00(t *testing.T) {
	checkArray(t, NewArray([]byte{1, 2, 3, 4}, 16, BigEndian), 16, []byte{1, 2}, "0102")
}

// Construction clears any unused bits in the final byte (big endian).
func Test_BitArray_New_01(t *testing.T) {
	checkArray(t, NewArray([]byte{0xff, 0xff}, 11, BigEndian), 11, []byte{0xff, 0xe0}, "ffe0:3")
}

// Construction clears any unused bits in the final byte (little endian).
func Test_BitArray_New_02(t *testing.T) {
	checkArray(t, NewArray([]byte{0xff, 0xff}, 11, LittleEndian), 11, []byte{0xff, 0x07}, "ff07:3")
}

// ===================================================================
// Parsing
// ===================================================================

func Test_BitArray_Parse_00(t *testing.T) {
	checkParseArray(t, "", []byte{}, 0)
}

func Test_BitArray_Parse_01(t *testing.T) {
	checkParseArray(t, "31f00e1d", []byte{0x31, 0xf0, 0x0e, 0x1d}, 32)
}

func Test_BitArray_Parse_02(t *testing.T) {
	checkParseArray(t, "bf1230:5", []byte{0xbf, 0x12, 0x30}, 21)
}

func Test_BitArray_Parse_03(t *testing.T) {
	checkParseArray(t, "f8:5", []byte{0xf8}, 5)
}

func Test_BitArray_Parse_Invalid(t *testing.T) {
	for _, text := range []string{
		"zz",     // not hex
		"ff:0",   // mask out of range
		"ff:8",   // mask out of range
		"ff:1:2", // too many parts
		":3",     // mask without data
		"ff:3",   // unused bits are set
		"f8:4",   // unused bits are set
	} {
		if _, err := ParseArray(text); err == nil {
			t.Errorf("expected error parsing %q", text)
		}
	}
}

// ===================================================================
// Equality
// ===================================================================

func Test_BitArray_Equals_00(t *testing.T) {
	// Equality is by content, not by nil-ness of the underlying slice.
	if !ZeroArray(0).Equals(Array{}) || !(Array{}).Equals(ZeroArray(0)) {
		t.Errorf("expected zero-length arrays to be equal")
	}
}

func Test_BitArray_Equals_01(t *testing.T) {
	if !ArrayFromBytes(0x12).Equals(NewArray([]byte{0x12}, 8, BigEndian)) {
		t.Errorf("expected arrays to be equal")
	}
}

func Test_BitArray_Equals_02(t *testing.T) {
	// Same bytes, different number of bits.
	if NewArray([]byte{0x80}, 1, BigEndian).Equals(NewArray([]byte{0x80}, 2, BigEndian)) {
		t.Errorf("expected arrays to differ")
	}
}

// ===================================================================
// Append / Read
// ===================================================================

func Test_BitArray_Append_00(t *testing.T) {
	var arr Array
	// 101
	arr.Append([]byte{0xa0}, 3, BigEndian)
	checkArray(t, arr, 3, []byte{0xa0}, "a0:3")
	// 101 11111
	arr.Append([]byte{0xff}, 5, BigEndian)
	checkArray(t, arr, 8, []byte{0xbf}, "bf")
	// 101 11111 00010010 00110
	arr.Append([]byte{0x12, 0x34}, 13, BigEndian)
	checkArray(t, arr, 21, []byte{0xbf, 0x12, 0x30}, "bf1230:5")
	// Read chunks back
	checkRead(t, arr, 3, 5, BigEndian, []byte{0xf8})
	checkRead(t, arr, 8, 13, BigEndian, []byte{0x12, 0x30})
	// Cross check against parser
	if parsed, err := ParseArray("bf1230:5"); err != nil || !parsed.Equals(arr) {
		t.Errorf("expected %v, received %v (%v)", arr, parsed, err)
	}
}

func Test_BitArray_Append_01(t *testing.T) {
	var arr Array
	// 101
	arr.Append([]byte{0x05}, 3, LittleEndian)
	checkArray(t, arr, 3, []byte{0x05}, "05:3")
	// 11111 101
	arr.Append([]byte{0x1f}, 5, LittleEndian)
	checkArray(t, arr, 8, []byte{0xfd}, "fd")
	// 11111 101 | 00010010 | 10100
	arr.Append([]byte{0x12, 0x34}, 13, LittleEndian)
	checkArray(t, arr, 21, []byte{0xfd, 0x12, 0x14}, "fd1214:5")
	// Read chunks back
	checkRead(t, arr, 3, 5, LittleEndian, []byte{0x1f})
	checkRead(t, arr, 8, 13, LittleEndian, []byte{0x12, 0x14})
}

// ===================================================================
// Write / Read
// ===================================================================

func Test_BitArray_Write_00(t *testing.T) {
	arr := ZeroArray(16)
	// bits 3..7 := 10101
	arr.Write(3, []byte{0xa8}, 5, BigEndian)
	checkArray(t, arr, 16, []byte{0x15, 0x00}, "1500")
	checkRead(t, arr, 3, 5, BigEndian, []byte{0xa8})
}

func Test_BitArray_Write_01(t *testing.T) {
	arr := ZeroArray(16)
	// bits 3..7 := 10101
	arr.Write(3, []byte{0x15}, 5, LittleEndian)
	checkArray(t, arr, 16, []byte{0xa8, 0x00}, "a800")
	checkRead(t, arr, 3, 5, LittleEndian, []byte{0x15})
}

// ===================================================================
// Decode
// ===================================================================

func Test_BitArray_Decode_00(t *testing.T) {
	arr := ArrayFromBytes(0x31, 0xf0, 0x0e, 0x1d)
	// u4 nibbles, most significant first
	values, rest := arr.Decode(0, 4, BigEndian, func(b []byte) byte { return b[0] >> 4 })
	//
	checkDecoded(t, values, rest, []byte{0x3, 0x1, 0xf, 0x0, 0x0, 0xe, 0x1, 0xd}, 0)
}

func Test_BitArray_Decode_01(t *testing.T) {
	arr := ArrayFromBytes(0x31, 0xf0, 0x0e, 0x1d)
	// u4 nibbles, least significant first
	values, rest := arr.Decode(0, 4, LittleEndian, func(b []byte) byte { return b[0] & 0xf })
	//
	checkDecoded(t, values, rest, []byte{0x1, 0x3, 0x0, 0xf, 0xe, 0x0, 0xd, 0x1}, 0)
}

func Test_BitArray_Decode_02(t *testing.T) {
	arr := ArrayFromBytes(0x31, 0xf0, 0x0e, 0x1d)
	// u7 values do not divide 32 bits evenly, leaving 4 bits over.
	values, rest := arr.Decode(0, 7, BigEndian, func(b []byte) byte { return b[0] >> 1 })
	//
	checkDecoded(t, values, rest, []byte{0b0011000, 0b1111100, 0b0000001, 0b1100001}, 4)
}

func Test_BitArray_Decode_03(t *testing.T) {
	arr := ArrayFromBytes(0x31, 0xf0, 0x0e, 0x1d)
	// Decoding from a non-zero offset
	values, rest := arr.Decode(8, 8, BigEndian, func(b []byte) byte { return b[0] })
	//
	checkDecoded(t, values, rest, []byte{0xf0, 0x0e, 0x1d}, 0)
}

func Test_BitArray_Decode_04(t *testing.T) {
	values, rest := ZeroArray(0).Decode(0, 4, BigEndian, func(b []byte) byte { return b[0] })
	//
	checkDecoded(t, values, rest, []byte{}, 0)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkArray(t *testing.T, arr Array, expectedLen uint, expectedBytes []byte, expectedString string) {
	if actual := arr.Len(); actual != expectedLen {
		t.Errorf("expected %d bits, received %d", expectedLen, actual)
	}
	//
	if !slices.Equal(expectedBytes, arr.Bytes()) {
		t.Errorf("expected bytes %v, received %v", expectedBytes, arr.Bytes())
	}
	//
	if actual := arr.String(); actual != expectedString {
		t.Errorf("expected string %q, received %q", expectedString, actual)
	}
}

func checkParseArray(t *testing.T, text string, expectedBytes []byte, nbits uint) {
	arr, err := ParseArray(text)
	//
	if err != nil {
		t.Fatalf("unexpected error parsing %q: %v", text, err)
	}
	// Parsing and printing should roundtrip.
	checkArray(t, arr, nbits, expectedBytes, text)
}

func checkRead(t *testing.T, arr Array, offset, nbits uint, order Order, expected []byte) {
	buf := NewBuffer(nbits)
	//
	arr.Read(offset, buf, nbits, order)
	//
	if !slices.Equal(expected, buf) {
		t.Errorf("expected read %v, received %v", expected, buf)
	}
}

func checkDecoded(t *testing.T, values []byte, rest uint, expected []byte, expectedRest uint) {
	if !slices.Equal(expected, values) {
		t.Errorf("expected values %v, received %v", expected, values)
	}
	//
	if rest != expectedRest {
		t.Errorf("expected %d remaining bits, received %d", expectedRest, rest)
	}
}
