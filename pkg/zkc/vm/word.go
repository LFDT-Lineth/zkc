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
package vm

import (
	"encoding/gob"
	"math"
	"math/big"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/memory"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

func init() {
	gob.Register(instruction.Word(&instruction.WordTypeA[Uint]{}))
	gob.Register(instruction.Word(&instruction.WordTypeB{}))
	gob.Register(instruction.Word(&instruction.WordTypeF[Uint]{}))
	gob.Register(instruction.Module(&function.Function[instruction.Word]{}))
	gob.Register(instruction.Module(&memory.RandomAccess[Uint]{}))
	gob.Register(instruction.Module(&memory.ReadOnly[Uint]{}))
	gob.Register(instruction.Module(&memory.WriteOnce[Uint]{}))
	gob.Register(instruction.Module(&memory.StaticReadOnly[Uint]{}))
	gob.Register(instruction.Module(&memory.BiPartiteRandomAccess[Uint]{}))
}

// WordConfig provides a minimal amount of information about a machine word
// type.
type WordConfig struct {
	Name      string
	Bandwidth uint
}

// WORD_UINT64 provides metadata about the Uint64 word type.
var WORD_UINT64 = WordConfig{Name: "Uint64", Bandwidth: 64}

// WORD_UINT provides metadata about the Uint word type.
var WORD_UINT = WordConfig{Name: "Uint", Bandwidth: math.MaxUint}

// Word abstracts the data type (a.k.a the "machine word") used for holding
// values within the machine.  The reason for abstracting this concept is to
// allow a machine compiled for a larger word size to be automatically lowered
// to a machine for a smaller word size.  For example, our source program might
// be written for a 64bit machine and we wish to executed it on 16bit machine
// (i.e. because our target field configuration has a maximum register size of
// 16bits).
type Word[W any] = word.Word[W]

// Uint represents an unbound unsigned integer.
type Uint = word.Uint

// Uint64 represents an 64-bit unsigned integer.
type Uint64 = word.Uint64

// WordVec represents a vector of zero or more words.  Typically, this is used
// for storing constants after register splitting.
type WordVec[W Word[W]] = word.Vector[W]

// ============================================================================
// Constructors
// ============================================================================

// Const64 initialises a given word with a 64bit value.  This will panic if the
// given value exceeds the available bandwidth of the word in question.
func Const64[W Word[W]](val uint64) W {
	return word.Const64[W](val)
}

// NewWordVector constructs a new word vector from a given array of words.
func NewWordVector[W Word[W]](words ...W) WordVec[W] {
	return word.NewVector(words...)
}

// ============================================================================
// Decoding
// ============================================================================

// DecodeBytes decodes the given set of bytes as big integer values according to
// the given register type(s).  Observe that values are assumed to be packed tightly
// (i.e. without any padding).  Consider the following input byte array:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
//
// Then, decoding this into a u4 array will produce the following:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// If the input array is not a multiple of the bitwidth
func DecodeBytes[W Word[W]](bytes []byte, registers []register.Register) []W {
	var (
		bitwidth = bitwidthOf(registers)
		// Initially empty buffer which is expanded as necessary to accommodate
		// reading bits of the given data types.
		buffer []byte
	)
	// Decode array into
	values, _ := bit.DecodeArray(bitwidth, bytes, func(bytes []byte) (ints []big.Int) {
		var (
			reader = bit.NewReader(bytes)
		)
		// Decode the type using the given buffer
		for _, t := range registers {
			var vs []big.Int
			//
			vs, buffer = DecodeUnsignedInt(t.Width(), &reader, buffer)
			//
			ints = append(ints, vs...)
		}
		// Done
		return ints
	})
	// Flattern decoded tuples
	return array.FlatMap(values, func(ints []big.Int) []W {
		var words = make([]W, len(ints))
		//
		for i, v := range ints {
			var ith W
			//
			words[i] = ith.SetBigInt(&v)
		}
		//
		return words
	})
}

// EncodeBytes encodes the given set of word values as packed bytes according to
// the given registers type(s). This is the inverse of DecodeAll.  Consider the
// following input array of u4 values:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// Then, encoding this as a u4 array will produce the following bytes:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
func EncodeBytes[W Word[W]](values []W, registers []register.Register) []byte {
	var (
		nRegs     = uint(len(registers))
		nElems    = uint(len(values))
		bitOffset uint
		// total bitwidth of round
		bitwidth = bitwidthOf(registers)
		// buffer size required to hold a round
		n = bit.BytesRequiredFor(bitwidth)
		// create buffer
		buf = make([]byte, n)
		// Determine number of (full) rounds
		nRounds = nElems / nRegs
		// Initial total bits calculation
		totalBits = nRounds * bitwidth
	)
	// Update total bits calc for odd number of elements.
	for i := (nRounds * nRegs); i < nElems; i++ {
		totalBits += registers[i].Width()
	}
	// Determine required size result buffer
	totalBytes := (totalBits + 7) / 8
	// Allocate result buffer
	result := make([]byte, totalBytes)
	//
	for i := 0; i < len(values); {
		for j := 0; j < len(registers) && i < len(values); j++ {
			var (
				reg = registers[j]
				val = values[i]
			)
			EncodeUnsignedInt(reg.Width(), val.BigInt(), buf)
			bit.BigEndianCopy(buf, 0, result, bitOffset, reg.Width())
			bitOffset += reg.Width()
			i = i + 1
		}
	}
	//
	return result
}

// ============================================================================
// Decoding
// ============================================================================

// DecodeUnsignedInt decodes the given set of bytes as big integer values
// according to the given register type(s).  Observe that values are assumed to
// be packed tightly (i.e. without any padding).  Consider the following input
// byte array:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
//
// Then, decoding this into a u4 array will produce the following:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// If the input array is not a multiple of the bitwidth
func DecodeUnsignedInt(bitwidth uint, reader *bit.Reader, buffer []byte) ([]big.Int, []byte) {
	var (
		val big.Int
		// Determine number of bytes required to hold value
		n = bit.BytesRequiredFor(bitwidth)
		// Calculate excess bits (needed for alignment)
		m = (n * 8) - bitwidth
	)
	// Expand buffer to ensure enough space
	buffer = expandBufferAsNeeded(bitwidth, buffer)
	// Read bitwidth bits out
	reader.BigEndianReadInto(bitwidth, buffer)
	// Assign (unaligned) bytes
	val.SetBytes(buffer[:n])
	// Right shift to fix alignment
	val.Rsh(&val, m)
	//
	return []big.Int{val}, buffer
}

// ============================================================================
// Encoding
// ============================================================================

// EncodeUnsignedInt encodes the given big integer as packed bytes according to
// the given bitwidths.  This is the inverse of DecodeUnsignedInt.
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// Then, encoding this as a u4 array will produce the following bytes:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
func EncodeUnsignedInt(bitwidth uint, v *big.Int, buf []byte) {
	var (
		w big.Int
		// Determine number of bytes required to hold value
		n = bit.BytesRequiredFor(bitwidth)
		// Calculate excess bits (needed for alignment)
		m = (n * 8) - bitwidth
	)
	// Clear buffer
	for i := range buf {
		buf[i] = 0
	}
	// Left-shift to account for alignment
	w.Lsh(v, m)
	// Fill with big-endian bytes of v, right-aligned in buf
	valBytes := w.Bytes()
	//
	if len(valBytes) > 0 {
		copy(buf[n-uint(len(valBytes)):], valBytes)
	}
}

// ============================================================================
// Misc
// ============================================================================

func expandBufferAsNeeded(bitwidth uint, buffer []byte) []byte {
	var n = bit.BytesRequiredFor(bitwidth)
	//
	if uint(len(buffer)) >= n {
		return buffer
	}
	//
	return make([]byte, n)
}

func bitwidthOf(registers []register.Register) uint {
	var width uint
	//
	for _, r := range registers {
		if r.IsNative() {
			panic("cannot determine bitwidth of native register")
		}
		//
		width += r.Width()
	}
	//
	return width
}
