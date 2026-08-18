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
package narray

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// AppendOnto attempts to efficiently append the contents of the right array
// onto the left array.  This currently assumes that the type and bitwidth of
// the two arrays matches exactly, and will panic otherwise.
func AppendOnto[F field.Element[F]](left MutArray[F], right Array[F]) {
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
