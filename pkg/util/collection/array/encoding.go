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
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Encode writes the contents of this array into the given buffer, using
// the natural encoding for this array representation.  The encoding is
// self-delimiting given the length of the array (i.e. Decode can determine
// how many bytes to read given the number of encoded elements).
func Encode[F word.Word[F]](array Array[F], buf *bytes.Buffer) {
	switch t := array.(type) {
	case *ConstantArray[F]:
		encodeConstantArray(t, buf)
	case *BitArray[F]:
		encodeBitArray(t, buf)
	case *SmallArray[uint8, F]:
		encodeSmallArray(t, buf)
	case *SmallArray[uint16, F]:
		encodeSmallArray(t, buf)
	case *SmallArray[uint32, F]:
		encodeSmallArray(t, buf)
	case *SmallArray[uint64, F]:
		encodeSmallArray(t, buf)
	case *StaticArray[F]:
		encodeStaticArray(t, buf)
	default:
		panic("unknown array encountered")
	}
}

// Decode reads a given number of elements from the given buffer into this
// array, replacing any existing contents.  The data is expected to be in
// the natural encoding for this array representation (i.e. as produced by
// Encode).
func Decode[F word.Word[F]](bitwidth uint, height uint, buf *bytes.Buffer) (Array[F], error) {
	// Construct column
	switch {
	case bitwidth == 0:
		return decodeConstantArray[F](height, bitwidth, buf)
	case bitwidth == 1:
		return decodeBitArray[F](height, buf)
	case bitwidth <= 8:
		return decodeSmallArray[uint8, F](height, buf)
	case bitwidth <= 16:
		return decodeSmallArray[uint16, F](height, buf)
	case bitwidth <= 32:
		return decodeSmallArray[uint32, F](height, buf)
	case bitwidth <= 64:
		return decodeSmallArray[uint64, F](height, buf)
	default:
		return decodeStaticArray[F](height, buf)
	}
}

// ============================================================================
// Constant Array
// ============================================================================

// Encode implementation for Array interface.  The natural encoding of a
// constant array is simply its constant value, written once as a
// length-prefixed sequence of raw bytes.
func encodeConstantArray[F word.Word[F]](p *ConstantArray[F], buffer *bytes.Buffer) {
	writeWordBytes(buffer, p.value.Bytes())
}

// Decode implementation for MutArray interface.  This reads the constant value
// (as produced by Encode), and sets the array to hold the given number of
// copies of it.
func decodeConstantArray[F word.Word[F]](height uint, bitwidth uint, buffer *bytes.Buffer) (Array[F], error) {
	var (
		data, err = readWordBytes(buffer)
		value     F
	)
	//
	if err != nil {
		return nil, err
	}
	//
	return NewConstantArray(height, bitwidth, value.SetBytes(data)), nil
}

// ============================================================================
// Bit Array
// ============================================================================

// Encode implementation for Array interface.  The natural encoding of a bit
// array is its packed byte representation, where eight bits are packed into
// each byte.
func encodeBitArray[T word.Word[T]](p *BitArray[T], buffer *bytes.Buffer) {
	var (
		bytewidth = word.ByteWidth(p.height)
		data      = make([]byte, bytewidth)
	)
	//
	bit.LittleEndianCopy(p.data, 0, data, 0, p.height)
	buffer.Write(data)
}

// Decode implementation for MutArray interface.  This reads a packed byte
// representation (as produced by Encode) holding the given number of bits.
func decodeBitArray[F word.Word[F]](height uint, buffer *bytes.Buffer) (Array[F], error) {
	var (
		bytewidth = word.ByteWidth(height)
		p         BitArray[F]
	)
	//
	if uint(buffer.Len()) < bytewidth {
		return nil, fmt.Errorf("bit array requires %d bytes, but only %d remain", bytewidth, buffer.Len())
	}
	// Observe bytes must be cloned, since the slice returned by Next is only
	// valid until the next buffer operation.
	p.data = bytes.Clone(buffer.Next(int(bytewidth)))
	p.height = height
	//
	return &p, nil
}

// ============================================================================
// Small Array
// ============================================================================

// Encode implementation for Array interface.  The natural encoding of a small
// array is its elements written as fixed-width, little endian values.
func encodeSmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]](p *SmallArray[K, T], buffer *bytes.Buffer) {
	if err := binary.Write(buffer, binary.LittleEndian, p.data); err != nil {
		// Unreachable, since writes to a bytes.Buffer cannot fail.
		panic(err)
	}
}

// Decode implementation for MutArray interface.  This reads a given number of
// fixed-width, little endian values (as produced by Encode).
func decodeSmallArray[K uint8 | uint16 | uint32 | uint64, T word.Word[T]](height uint, buf *bytes.Buffer,
) (Array[T], error) {
	var (
		data = make([]K, height)
		p    SmallArray[K, T]
	)
	//
	if err := binary.Read(buf, binary.LittleEndian, data); err != nil {
		return nil, err
	}
	//
	p.data = data
	//
	return &p, nil
}

// ============================================================================
// Static Array
// ============================================================================

// Encode implementation for Array interface.  The natural encoding of a static
// array is its elements written as length-prefixed sequences of raw bytes.
func encodeStaticArray[T word.Word[T]](p *StaticArray[T], buffer *bytes.Buffer) {
	for _, w := range p.data {
		writeWordBytes(buffer, w.Bytes())
	}
}

// Decode implementation for MutArray interface.  This reads a given number of
// length-prefixed words (as produced by Encode).
func decodeStaticArray[T word.Word[T]](height uint, buffer *bytes.Buffer) (Array[T], error) {
	var (
		data = make([]T, height)
		p    StaticArray[T]
	)
	//
	for i := range data {
		bs, err := readWordBytes(buffer)
		//
		if err != nil {
			return nil, err
		}
		//
		data[i] = data[i].SetBytes(bs)
	}
	//
	p.data = data
	//
	return &p, nil
}
