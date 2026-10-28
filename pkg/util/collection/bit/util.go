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

// Width determines the smallest bitwidth which can hold all values below a
// given bound.  Basically, the bound is raised to the nearest power of 2.  For
// example, given 4 this should return 2bits, whilst 5 should return 3bits, etc.
func Width(bound uint) uint {
	// Determine actual bound
	bitwidth := uint(1)
	acc := uint(2)
	//
	for ; acc < bound; acc = acc * 2 {
		bitwidth++
	}
	// Done
	return bitwidth
}

// BytesRequiredFor returns the minimum number of bytes required to hold the
// given bitwidth.  For example, the number of bytes to hold a u16 is 2 bytes,
// whilst the minimum required to hold a u17 is 3 bytes.
func BytesRequiredFor(bitwidth uint) uint {
	var (
		nbytes = bitwidth / 8
	)
	// round up (if necessary)
	if bitwidth%8 != 0 {
		nbytes++
	}
	//
	return nbytes
}

// NewBuffer allocates a byte array which is large enough to hold values of the
// given bitwidth, as needed for the Reader.
func NewBuffer(bitwidth uint) []byte {
	return make([]byte, BytesRequiredFor(bitwidth))
}
