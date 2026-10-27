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

// Fully aligned reads

func Test_LsReader_Aligned_00(t *testing.T) {
	checkLsReader(t, 0, 8, []byte{159, 0}, []byte{159})
}

func Test_LsReader_Aligned_01(t *testing.T) {
	checkLsReader(t, 8, 8, []byte{223, 159, 0}, []byte{159})
}

func Test_LsReader_Aligned_02(t *testing.T) {
	checkLsReader(t, 8, 16, []byte{223, 159, 123, 0, 0}, []byte{159, 123})
}
func Test_LsReader_Aligned_03(t *testing.T) {
	checkLsReader(t, 16, 16, []byte{1, 223, 159, 123, 0, 0}, []byte{159, 123})
}

// Partially aligned reads
func Test_LsReader_Partial_00(t *testing.T) {
	checkLsReader(t, 0, 7, []byte{159, 0}, []byte{31})
}
func Test_LsReader_Partial_01(t *testing.T) {
	checkLsReader(t, 0, 13, []byte{159, 99, 0}, []byte{159, 3})
}

func Test_LsReader_Partial_02(t *testing.T) {
	checkLsReader(t, 8, 7, []byte{223, 159, 0}, []byte{31})
}
func Test_LsReader_Partial_03(t *testing.T) {
	checkLsReader(t, 8, 13, []byte{223, 159, 99, 0}, []byte{159, 3})
}

func Test_LsReader_Partial_04(t *testing.T) {
	checkLsReader(t, 8, 19, []byte{223, 159, 89, 99, 0}, []byte{159, 89, 3})
}

// Fully unaligned aligned reads
func Test_LsReader_Unaligned_00(t *testing.T) {
	checkLsReader(t, 1, 7, []byte{159, 0}, []byte{79})
}

func Test_LsReader_Unaligned_01(t *testing.T) {
	checkLsReader(t, 3, 19, []byte{159, 99, 35, 0}, []byte{115, 108, 4})
}

func checkLsReader(t *testing.T, offset uint, nbits uint, src []byte, expected []byte) {
	var (
		reader = NewReader(src)
		buf    = make([]byte, len(src))
	)
	// Set junk in buf.  This is useful to check that the final byte is cleared
	// for all bits.
	for i := range buf {
		buf[i] = 0xaa
	}
	// Initial (discarded) read
	_ = reader.LittleEndianReadInto(offset, buf)
	// Actual read
	n := reader.LittleEndianReadInto(nbits, buf)
	// Extract read bytes
	actual := buf[:n]
	//
	if !slices.Equal(expected, actual) {
		t.Errorf("expected %v, received %v", expected, actual)
	}
}
