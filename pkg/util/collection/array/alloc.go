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
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Alloc allocates a new array suitable for holding elements upto the given
// bitwidth, and initialises it with default values upto the given height.
func Alloc[F word.Word[F]](bitwidth uint, height uint) MutArray[F] {
	var zero F
	// Construct column
	switch {
	case bitwidth == 0:
		return NewConstantArray(height, 0, zero)
	case bitwidth == 1:
		return NewBitArray[F](height)
	case bitwidth <= 8:
		return NewSmallArray[uint8, F](height, bitwidth)
	case bitwidth <= 16:
		return NewSmallArray[uint16, F](height, bitwidth)
	case bitwidth <= 32:
		return NewSmallArray[uint32, F](height, bitwidth)
	case bitwidth <= 64:
		return NewSmallArray[uint64, F](height, bitwidth)
	default:
		return NewStaticArray[F](height, bitwidth)
	}
}

// AppendOnto attempts to efficiently append the contents of the right array
// onto the left array.  This currently assumes that the type and bitwidth of
// the two arrays matches exactly, and will panic otherwise.
func AppendOnto[F word.Word[F]](left MutArray[F], right Array[F]) {
	switch left := left.(type) {
	case *ConstantArray[F]:
		var right = right.(*ConstantArray[F])
		left.AppendAll(*right)
	case *BitArray[F]:
		var right = right.(*BitArray[F])
		left.AppendAll(*right)
	case *SmallArray[uint8, F]:
		var right = right.(*SmallArray[uint8, F])
		left.AppendAll(*right)
	case *SmallArray[uint16, F]:
		var right = right.(*SmallArray[uint16, F])
		left.AppendAll(*right)
	case *SmallArray[uint32, F]:
		var right = right.(*SmallArray[uint32, F])
		left.AppendAll(*right)
	case *SmallArray[uint64, F]:
		var right = right.(*SmallArray[uint64, F])
		left.AppendAll(*right)
	case *StaticArray[F]:
		var right = right.(*StaticArray[F])
		left.AppendAll(*right)
	default:
		panic("unknown array")
	}
}
