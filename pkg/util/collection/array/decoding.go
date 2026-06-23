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
	"encoding/binary"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array/codec"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Decode a given encoding into a mutable array, using a given heap (which
// should have been preloaded accordingly).
func Decode[T word.DynamicWord[T], P Pool[T]](encoding Encoding, heap P) MutArray[T] {
	switch encoding.OpCode() {
	case ENCODING_STATIC_CONSTANT:
		return decode_static_constant[T](encoding)
	case ENCODING_STATIC_DENSE:
		return decode_static_dense[T](encoding)
	case ENCODING_STATIC_SPARSE8:
		return decode_static_sparse[T](encoding, 8)
	case ENCODING_STATIC_SPARSE16:
		return decode_static_sparse[T](encoding, 16)
	case ENCODING_STATIC_SPARSE32:
		return decode_static_sparse[T](encoding, 32)
	case ENCODING_POOL_CONSTANT:
		return decode_pool_constant[T](encoding, heap)
	case ENCODING_POOL1_DENSE:
		return decode_pool_dense(encoding, heap, 1)
	case ENCODING_POOL2_DENSE:
		return decode_pool_dense(encoding, heap, 2)
	case ENCODING_POOL4_DENSE:
		return decode_pool_dense(encoding, heap, 4)
	case ENCODING_POOL8_DENSE:
		return decode_pool_dense(encoding, heap, 8)
	case ENCODING_POOL16_DENSE:
		return decode_pool_dense(encoding, heap, 16)
	case ENCODING_POOL32_DENSE:
		return decode_pool_dense(encoding, heap, 32)
	case ENCODING_POOL8_SPARSE8:
		return decode_pool_sparse(encoding, heap, 8, 8)
	case ENCODING_POOL8_SPARSE16:
		return decode_pool_sparse(encoding, heap, 8, 16)
	case ENCODING_POOL8_SPARSE32:
		return decode_pool_sparse(encoding, heap, 8, 32)
	case ENCODING_POOL16_SPARSE8:
		return decode_pool_sparse(encoding, heap, 16, 8)
	case ENCODING_POOL16_SPARSE16:
		return decode_pool_sparse(encoding, heap, 16, 16)
	case ENCODING_POOL16_SPARSE32:
		return decode_pool_sparse(encoding, heap, 16, 32)
	case ENCODING_POOL32_SPARSE8:
		return decode_pool_sparse(encoding, heap, 32, 8)
	case ENCODING_POOL32_SPARSE16:
		return decode_pool_sparse(encoding, heap, 32, 16)
	case ENCODING_POOL32_SPARSE32:
		return decode_pool_sparse(encoding, heap, 32, 32)
	default:
		panic(fmt.Sprintf("unsupported encoding (%d)", encoding.OpCode()))
	}
}

// ============================================================================
// Constant Arrays
// ============================================================================

func decode_static_constant[T word.DynamicWord[T]](encoding Encoding) MutArray[T] {
	var (
		value    T
		len      = binary.BigEndian.Uint32(encoding.Bytes)
		constant = encoding.Operand()
		tmp      = constant
		bitwidth uint
	)
	// Determine bitwidth of constant
	for tmp > 0 {
		tmp >>= 1
		bitwidth++
	}
	//
	value = value.SetUint64(uint64(constant))
	//
	return NewConstantArray(uint(len), bitwidth, value)
}

func decode_pool_constant[T word.DynamicWord[T], P Pool[T]](encoding Encoding, heap P) MutArray[T] {
	var (
		len   = binary.BigEndian.Uint32(encoding.Bytes)
		index = encoding.Operand()
		value = heap.Get(index)
	)
	//
	return NewConstantArray(uint(len), 8*value.ByteWidth(), value)
}

// ============================================================================
// Static Arrays
// ============================================================================

func decode_static_dense[T word.DynamicWord[T]](encoding Encoding) MutArray[T] {
	var bitwidth = uint(encoding.Operand())
	//
	switch {
	case bitwidth == 1:
		data, height := codec.DecodeU1(encoding.Bytes)
		return &BitArray[T]{data, height}
	case bitwidth <= 2:
		return &SmallArray[uint8, T]{codec.DecodeU2Dense[uint8](encoding.Bytes), bitwidth}
	case bitwidth <= 4:
		return &SmallArray[uint8, T]{codec.DecodeU4Dense[uint8](encoding.Bytes), bitwidth}
	case bitwidth <= 8:
		// Nothing to decode here!
		return &SmallArray[uint8, T]{encoding.Bytes, bitwidth}
	case bitwidth <= 16:
		return &SmallArray[uint16, T]{codec.DecodeU16Dense[uint16](encoding.Bytes), bitwidth}
	case bitwidth <= 32:
		return &SmallArray[uint32, T]{codec.DecodeU32Dense[uint32](encoding.Bytes), bitwidth}
	default:
		panic(fmt.Sprintf("unsupported static array type (u%d)", bitwidth))
	}
}

func decode_static_sparse[T word.DynamicWord[T]](encoding Encoding, blockSizeWidth uint) MutArray[T] {
	var bitwidth = uint(encoding.Operand())
	//
	switch {
	case bitwidth <= 8:
		return &SmallArray[uint8, T]{codec.DecodeU8Sparse[uint8](encoding.Bytes, blockSizeWidth), bitwidth}
	case bitwidth <= 16:
		return &SmallArray[uint16, T]{codec.DecodeU16Sparse[uint16](encoding.Bytes, blockSizeWidth), bitwidth}
	case bitwidth <= 32:
		return &SmallArray[uint32, T]{codec.DecodeU32Sparse[uint32](encoding.Bytes, blockSizeWidth), bitwidth}
	default:
		panic(fmt.Sprintf("unsupported static array type (u%d,u%d)", bitwidth, blockSizeWidth))
	}
}

// ============================================================================
// Pool Arrays
// ============================================================================

func decode_pool_dense[T word.DynamicWord[T], P Pool[T]](encoding Encoding, heap P, indexWidth uint) MutArray[T] {
	var bitwidth = uint(encoding.Operand())
	//
	switch {
	case indexWidth <= 1:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU1Dense[uint32](encoding.Bytes), bitwidth}
	case indexWidth <= 2:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU2Dense[uint32](encoding.Bytes), bitwidth}
	case indexWidth <= 4:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU4Dense[uint32](encoding.Bytes), bitwidth}
	case indexWidth <= 8:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU8Dense[uint32](encoding.Bytes), bitwidth}
	case indexWidth <= 16:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU16Dense[uint32](encoding.Bytes), bitwidth}
	case indexWidth <= 32:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU32Dense[uint32](encoding.Bytes), bitwidth}
	default:
		panic(fmt.Sprintf("unknown pool array type (u%d)", indexWidth))
	}
}

func decode_pool_sparse[T word.DynamicWord[T], P Pool[T]](encoding Encoding, heap P,
	indexWidth, blockSizeWidth uint) MutArray[T] {
	//
	var bitwidth = uint(encoding.Operand())
	//
	switch {
	case indexWidth <= 8:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU8Sparse[uint32](encoding.Bytes, blockSizeWidth), bitwidth}
	case indexWidth <= 16:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU16Sparse[uint32](encoding.Bytes, blockSizeWidth), bitwidth}
	case indexWidth <= 32:
		return &PoolArray[uint32, T, P]{heap, codec.DecodeU32Sparse[uint32](encoding.Bytes, blockSizeWidth), bitwidth}
	default:
		panic(fmt.Sprintf("unknown pool array type (u%d)", indexWidth))
	}
}
