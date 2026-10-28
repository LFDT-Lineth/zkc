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

func u4_decoder(bytes []byte) uint {
	return uint(bytes[0] >> 4)
}

func u5_decoder(bytes []byte) uint {
	return uint(bytes[0] >> 3)
}

func Test_BitDecoder_00(t *testing.T) {
	checkDecoder(t, 4, []byte{}, []uint{}, u4_decoder)
}

func Test_BitDecoder_01(t *testing.T) {
	checkDecoder(t, 4, []byte{0xf}, []uint{0x0, 0xf}, u4_decoder)
}

func Test_BitDecoder_02(t *testing.T) {
	checkDecoder(t, 4, []byte{0xef}, []uint{0xe, 0xf}, u4_decoder)
}

func Test_BitDecoder_03(t *testing.T) {
	checkDecoder(t, 5, []byte{0xef}, []uint{0x1d}, u5_decoder)
}

func Test_BitDecoder_04(t *testing.T) {
	checkDecoder(t, 5, []byte{0xef, 0x40}, []uint{0x1d, 0x1d, 0x00}, u5_decoder)
}

func checkDecoder[T comparable](t *testing.T, width uint, input []byte, output []T, decoder func([]byte) T) {
	var (
		n = uint(len(input) * 8)
		// construct array of given size
		bits = NewArray(input, n, BigEndian)
		// Determine expected residue (i.e. bits left)
		residue = n % width
	)
	//
	t.Parallel()
	//
	actual, remainder := bits.Decode(0, width, BigEndian, decoder)
	// Check correct residue
	if residue != remainder {
		t.Errorf("incorrect number of residual bits (expected %v, received %v)", residue, remainder)
	}
	// Check correct output
	if !slices.Equal(actual, output) {
		t.Errorf("incorrect output (expected %v, received %v)", output, actual)
	}
}
