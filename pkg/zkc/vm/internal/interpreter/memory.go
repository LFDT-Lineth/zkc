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
package interpreter

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	lword "github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Memory represents (in many ways) the simplest form of memory
// which can be read or written without restrictions.  Initially, all locations
// of a RAM can be considered to hold zero.  Thus, reading a location which has
// not yet been written will return zero; otherwise, it will return the last
// value written.
type Memory[W word.Word[W]] interface {
	// Descriptor defines the layout (a.k.a. the geometry) of this memory.  This
	// is responsible for translating multi-word addresses into a contiguous
	// range within a flat data slice.  Memories in this VM address their
	// contents using tuples of words (i.e. []W) rather than a single scalar,
	// because a single field element may not be wide enough to express every
	// valid address.
	Descriptor() *descriptor.Memory[W]
	// Initialise this memory with the given contents.  This will overwrite any
	// existing contents. For WOM's it resets the seen addresses.
	Initialise(contents []W)
	// Read the value at a given physical address within this memory, possibly
	// producing an error (e.g. for an out-of-bounds access).
	Read(address uint64) (W, error)
	// Write to a given physical address within this memory, possibly
	// producing an error (e.g. for an out-of-bounds access).
	Write(address uint64, value W) error
	// Contents returns the contents of this memory as an array.
	Contents() []W
	//
	Checkpoint(mid uint16) checkpoint.Memory
	//
	Restore(checkpoint.Memory)
	// AccessLog returns the chronological log of reads / writes performed
	// against this memory, or nil if it does not record accesses (only
	// read-write memory records, and only when a recording log is installed
	// for trace generation).  The trace observer consumes this to materialise
	// the RAM trace.
	AccessLog() []AccessData[W]
}

// InputOutput identifiers memory used to represent inputs or outputs.  The main
// purpose of this is to enable inspection of said memory to ensure e.g. the
// correct outputs are produced.
type InputOutput[W word.Word[W]] interface {
	// Contents returns the contents of this memory as an array.
	Contents() []W
	// Descriptor defines the layout (a.k.a. the geometry) of this memory
	Descriptor() *descriptor.Memory[W]
	// Initialise this memory with the given contents.  This will overwrite any
	// existing contents.
	Initialise(contents []W)
}

// Pack a given set of words into a given set of bytes, according to a given set
// of descriptors.  Cells map onto registers in a round-robin fashion (i.e. cell
// i is described by regs[i % len(regs)]), matching the flat layout of both
// memory contents and stack frames.  Each cell is encoded big endian using a
// fixed number of bytes determined by its register (see Register.Bytewidth) or,
// for native registers, by the full bandwidth of the machine word.  Since the
// geometry is fixed, Unpack can reconstruct the original words from the bytes
// alone.
func Pack[W word.Word[W]](regs []descriptor.Register[W], data []W) []byte {
	var (
		offset, total uint
		zero          W
		// Fallback width for native registers.
		native = lword.ByteWidth(zero.Bandwidth())
	)
	// Determine total number of bytes required.
	for i := range data {
		total += regs[i%len(regs)].Bytewidth().UnwrapOr(native)
	}
	//
	bytes := make([]byte, total)
	//
	for i, ith := range data {
		n := regs[i%len(regs)].Bytewidth().UnwrapOr(native)
		packCell(bytes[offset:offset+n], ith)
		offset += n
	}
	//
	return bytes
}

// PackTimed packs a given set of (timestamped) words into a given set of bytes,
// according to a given set of descriptors.  The values are encoded exactly as
// for Pack, whilst the timestamps are returned separately (one per cell).
func PackTimed[W word.Word[W]](regs []descriptor.Register[W], data []TimestampedCell[W]) ([]byte, []uint64) {
	var (
		values = make([]W, len(data))
		stamps = make([]uint64, len(data))
	)
	//
	for i, cell := range data {
		values[i] = cell.value
		stamps[i] = cell.timestamp
	}
	//
	return Pack(regs, values), stamps
}

// Unpack a set of words from a given set of bytes.  This is the inverse of
// Pack: the number of cells is determined entirely by the length of the data
// and the (fixed) encoded width of each register.
func Unpack[W word.Word[W]](regs []descriptor.Register[W], data []byte) []W {
	var (
		words  []W
		offset uint
		zero   W
		// Fallback width for native registers.
		native = lword.ByteWidth(zero.Bandwidth())
	)
	//
	for i := 0; offset < uint(len(data)); i++ {
		n := regs[i%len(regs)].Bytewidth().UnwrapOr(native)
		//
		words = append(words, unpackCell[W](data[offset:offset+n]))
		offset += n
	}
	//
	return words
}

// UnpackTimed unpacks a set of (timestamped) words from a given set of bytes.
// This is the inverse of PackTimed, recombining each decoded value with its
// corresponding timestamp.
func UnpackTimed[W word.Word[W]](mem descriptor.Memory[W], data []byte, stamps []uint64) []TimestampedCell[W] {
	var values = Unpack(mem.DataRegisters(), data)
	// Sanity check timestamps line up with decoded cells.
	util.Assert(len(values) == len(stamps), "malformed page (%d cells, but %d timestamps)", len(values), len(stamps))
	//
	cells := make([]TimestampedCell[W], len(values))
	//
	for i, value := range values {
		cells[i] = TimestampedCell[W]{timestamp: stamps[i], value: value}
	}
	//
	return cells
}

// packCell encodes a single value big endian into the given buffer, whose
// length determines the encoded width.  This will panic if the value does not
// fit within the buffer (implying it exceeded its register's width).
func packCell[W word.Word[W]](buf []byte, value W) {
	// Fast path avoids big.Int allocation for cells of at most 64 bits.
	if len(buf) <= 8 {
		v := value.Uint64()
		//
		for i := len(buf) - 1; i >= 0; i-- {
			buf[i] = byte(v)
			v >>= 8
		}
		//
		util.Assert(v == 0, "value %s exceeds cell width (%d bytes)", value.Text(16), len(buf))
	} else {
		value.BigInt().FillBytes(buf)
	}
}

// unpackCell decodes a single (big endian) value from the given buffer, being
// the inverse of packCell.
func unpackCell[W word.Word[W]](buf []byte) W {
	var value W
	// Fast path avoids big.Int allocation for cells of at most 64 bits.
	if len(buf) <= 8 {
		var v uint64
		//
		for _, b := range buf {
			v = (v << 8) | uint64(b)
		}
		//
		return value.SetUint64(v)
	}
	//
	return value.SetBigInt(new(big.Int).SetBytes(buf))
}
