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
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// ENCODING_STATIC_CONSTANT is for arrays which hold constant values.
const ENCODING_STATIC_CONSTANT = 0

// ENCODING_STATIC_DENSE is for arrays which hold their values explicitly.
const ENCODING_STATIC_DENSE = 1

// ENCODING_STATIC_SPARSE8 is for arrays which hold their values explicitly, and
// are stored in a sparse representation (assuming u8 block lengths).
const ENCODING_STATIC_SPARSE8 = 2

// ENCODING_STATIC_SPARSE16 is for arrays which hold their values explicitly, and
// are stored in a sparse representation (assuming u16 block lengths).
const ENCODING_STATIC_SPARSE16 = 3

// ENCODING_STATIC_SPARSE24 is currently not supported.
const ENCODING_STATIC_SPARSE24 = 4

// ENCODING_STATIC_SPARSE32 is for arrays which hold their values explicitly, and
// are stored in a sparse representation (assuming u32 block lengths).
const ENCODING_STATIC_SPARSE32 = 5

// ENCODING_POOL_CONSTANT is for arrays holding a constant index into a pool.
const ENCODING_POOL_CONSTANT = 6

// ENCODING_POOL1_DENSE is for arrays holding u1 indictes into a pool.
const ENCODING_POOL1_DENSE = 7

// ENCODING_POOL2_DENSE is for arrays holding u2 indictes into a pool.
const ENCODING_POOL2_DENSE = 8

// ENCODING_POOL4_DENSE is for arrays holding u4 indictes into a pool.
const ENCODING_POOL4_DENSE = 9

// ENCODING_POOL8_DENSE is for arrays holding u8 indictes into a pool.
const ENCODING_POOL8_DENSE = 10

// ENCODING_POOL16_DENSE is for arrays holding u16 indictes into a pool.
const ENCODING_POOL16_DENSE = 11

// ENCODING_POOL32_DENSE is for arrays holding u32 indictes into a pool.
const ENCODING_POOL32_DENSE = 12

// ENCODING_POOL8_SPARSE8 is for arrays holding u8 indictes into a pool, and are
// stored in a sparse representation (assuming u8 block lengths).
const ENCODING_POOL8_SPARSE8 = 13

// ENCODING_POOL8_SPARSE16 is for arrays holding u8 indictes into a pool, and are
// stored in a sparse representation (assuming u16 block lengths).
const ENCODING_POOL8_SPARSE16 = 14

// ENCODING_POOL8_SPARSE24 is currently not supported.
const ENCODING_POOL8_SPARSE24 = 15

// ENCODING_POOL8_SPARSE32 is for arrays holding u8 indictes into a pool, and are
// stored in a sparse representation (assuming u8 block lengths).
const ENCODING_POOL8_SPARSE32 = 16

// ENCODING_POOL16_SPARSE8 is for arrays holding u16 indictes into a pool, and are
// stored in a sparse representation (assuming u8 block lengths).
const ENCODING_POOL16_SPARSE8 = 17

// ENCODING_POOL16_SPARSE16 is for arrays holding u16 indictes into a pool, and are
// stored in a sparse representation (assuming u16 block lengths).
const ENCODING_POOL16_SPARSE16 = 18

// ENCODING_POOL16_SPARSE24 is currently not supported.
const ENCODING_POOL16_SPARSE24 = 19

// ENCODING_POOL16_SPARSE32 is for arrays holding u16 indictes into a pool, and are
// stored in a sparse representation (assuming u32 block lengths).
const ENCODING_POOL16_SPARSE32 = 20

// ENCODING_POOL32_SPARSE8 is for arrays holding u32 indictes into a pool, and are
// stored in a sparse representation (assuming u8 block lengths).
const ENCODING_POOL32_SPARSE8 = 21

// ENCODING_POOL32_SPARSE16 is for arrays holding u32 indictes into a pool, and are
// stored in a sparse representation (assuming u16 block lengths).
const ENCODING_POOL32_SPARSE16 = 22

// ENCODING_POOL32_SPARSE24 is currently not supported.
const ENCODING_POOL32_SPARSE24 = 23

// ENCODING_POOL32_SPARSE32 is for arrays holding u32 indictes into a pool, and are
// stored in a sparse representation (assuming u32 block lengths).
const ENCODING_POOL32_SPARSE32 = 24

// Pool provides a convenient alias
type Pool[T any] = pool.Pool[uint32, T]

// Encoding represents an encoded form of a word array useful for long term
// storage (e.g. in a file).
type Encoding struct {
	// Indicates what encoding method is used for this encoding.
	Encoding uint32
	// Bytes of the encoding itself
	Bytes []byte
}

// OpCode returns the instruction opcode for this encoding.
func (p *Encoding) OpCode() uint8 {
	return uint8(p.Encoding >> 24)
}

// Operand returns the instruction operand for this encoding.
func (p *Encoding) Operand() uint32 {
	// Operand is actually 24bits
	return p.Encoding & 0xFF_FFFF
}

// Set sets the instruction opcode for this encoding.
func (p *Encoding) Set(opcode uint8, operand uint32) {
	// Set new opcode & operand
	p.Encoding = (uint32(opcode) << 24) | (operand & 0xFF_FFFF)
}

// Encode a given array as a sequence of bytes suitable for serialisation.
func Encode[T word.DynamicWord[T], P Pool[T]](array Array[T]) Encoding {
	var (
		encoding Encoding
		bitwidth = uint32(array.BitWidth())
	)
	//
	switch {
	case bitwidth == 0:
		encoding.Bytes = encode_constant(array.(*ConstantArray[T]))
		encoding.Set(ENCODING_STATIC_CONSTANT, 0)
	case bitwidth == 1:
		encoding.Bytes = encode_bits(array.(*BitArray[T]))
		encoding.Set(ENCODING_STATIC_DENSE, bitwidth)
	case bitwidth <= 8:
		encoding.Bytes = encode_small8(array.(*SmallArray[uint8, T]))
		encoding.Set(ENCODING_STATIC_DENSE, bitwidth)
	case bitwidth <= 16:
		encoding.Bytes = encode_small16(array.(*SmallArray[uint16, T]))
		encoding.Set(ENCODING_STATIC_DENSE, bitwidth)
	case bitwidth <= 32:
		encoding.Bytes = encode_small32(array.(*SmallArray[uint32, T]))
		encoding.Set(ENCODING_STATIC_DENSE, bitwidth)
	default:
		switch t := array.(type) {
		// POOL ARRAYS
		case *PoolArray[uint32, T, P]:
			encoding.Bytes = encode_pool(t)
			encoding.Set(ENCODING_POOL32_DENSE, uint32(t.BitWidth()))
		case *PoolArray[uint32, T, *pool.SharedHeap[T]]:
			// FIXME: this use case is only support for legacy reasons whilst the
			// existing legacy trace file format exists.
			encoding.Bytes = encode_pool(t)
			encoding.Set(ENCODING_POOL32_DENSE, uint32(t.bitwidth))
		default:
			panic(fmt.Sprintf("unknown array type: %s", reflect.TypeOf(t).String()))
		}
	}
	//
	return encoding
}

// ============================================================================
// Constant Arrays
// ============================================================================

func encode_constant[T word.DynamicWord[T]](array *ConstantArray[T]) []byte {
	var bytes [4]byte
	//
	binary.BigEndian.PutUint32(bytes[:], uint32(array.Len()))
	//
	return bytes[:]
}

// ============================================================================
// Static Arrays
// ============================================================================

func encode_bits[T word.DynamicWord[T]](array *BitArray[T]) []byte {
	var (
		unused = (len(array.data) * 8) - int(array.height)
	)
	//
	return append(array.data, uint8(unused))
}

func encode_small8[T word.DynamicWord[T]](array *SmallArray[uint8, T]) []byte {
	return array.data
}

func encode_small16[T word.DynamicWord[T]](array *SmallArray[uint16, T]) []byte {
	var bytes = make([]byte, array.Len()*2)
	//
	for i := range array.Len() {
		var (
			ith    = array.data[i]
			offset = i * 2
		)
		// big endian form
		bytes[offset] = uint8(ith >> 8)
		bytes[offset+1] = uint8(ith)
	}
	//
	return bytes
}

// Encode returns the byte encoding of this array.
func encode_small32[T word.DynamicWord[T]](array *SmallArray[uint32, T]) []byte {
	var bytes = make([]byte, array.Len()*4)
	//
	for i := range array.Len() {
		var (
			ith    = array.data[i]
			offset = i * 4
		)
		// big endian form
		bytes[offset] = uint8(ith >> 24)
		bytes[offset+1] = uint8(ith >> 16)
		bytes[offset+2] = uint8(ith >> 8)
		bytes[offset+3] = uint8(ith)
	}
	//
	return bytes
}

// ============================================================================
// Pool Array
// ============================================================================

// Encode returns the byte encoding of this array.
func encode_pool[T word.DynamicWord[T], P Pool[T]](array *PoolArray[uint32, T, P]) []byte {
	var bytes = make([]byte, array.Len()*4)
	//
	for i := range array.Len() {
		var (
			ith    = array.index[i]
			offset = i * 4
		)
		// big endian form
		bytes[offset] = uint8(ith >> 24)
		bytes[offset+1] = uint8(ith >> 16)
		bytes[offset+2] = uint8(ith >> 8)
		bytes[offset+3] = uint8(ith)
	}
	//
	return bytes
}
