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
package array

import (
	"math"
	"math/bits"

	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Alloc allocates a new array holding the given elements and which can hold any
// element upto the given bitwidth.  The given array maybe consumed by this
// array.
func Alloc[F word.Word[F]](bitwidth uint) MutArray[F] {
	var zero F
	// Construct column
	switch {
	case bitwidth == 0:
		return NewConstantArray(0, 0, zero)
	case bitwidth == 1:
		return NewBitArray[F](0, false)
	case bitwidth <= 8:
		return NewSmallArray[uint8, F](bitwidth, 0, 0)
	case bitwidth <= 16:
		return NewSmallArray[uint16, F](bitwidth, 0, 0)
	case bitwidth <= 32:
		return NewSmallArray[uint32, F](bitwidth, 0, 0)
	case bitwidth <= 64:
		return NewSmallArray[uint64, F](bitwidth, 0, 0)
	default:
		return NewStaticArray[F](bitwidth)
	}
}

// bitwidth of returns the (approximate) bitwidth of a given value appropriate
// for determine a suitable column width to use.
func bitwidthOf[F word.Word[F]](val F) uint {
	if val.FitsWithin(64) {
		return uint(bits.Len64(val.Uint64()))
	}
	//
	return math.MaxUint
}
