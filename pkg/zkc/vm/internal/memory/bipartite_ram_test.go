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
package memory

import (
	"math/big"
	"testing"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Note: tests are written for word.Uint (an arbitrary-precision unsigned word
// implementation).  Sticking with a single concrete word type keeps the tests
// readable; the BiPartiteArray itself is generic over W.

// uint64W constructs a word.Uint from the given uint64 value.
func uint64W(v uint64) word.Uint {
	var w word.Uint
	return w.SetUint64(v)
}

// newBiPartite builds a fresh BiPartiteArray with one numInputs-wide input
// register (named "addr") and numOutputs 64-bit output registers (named
// "data0", "data1", ...).  It returns the memory and a freshly allocated frame
// of the right size, with frame[0] being the address slot and frame[1..] being
// data slots.
func newBiPartite(addrWidth uint, numOutputs uint) (Memory[word.Uint], []word.Uint) {
	regs := []register.Register{
		register.NewInput("addr", addrWidth, *big.NewInt(0)),
	}
	//
	for range numOutputs {
		regs = append(regs, register.NewOutput("data", 64, *big.NewInt(0)))
	}
	//
	mem := NewBiPartiteRandomAccess[word.Uint]("test", regs)
	frame := make([]word.Uint, 1+numOutputs)
	//
	return mem, frame
}

// addrIds and dataIds return the register identifiers for use in Read/Write,
// matching the layout produced by newBiPartite.
func addrIds() []register.Id {
	return []register.Id{register.NewId(0)}
}

func dataIds(numOutputs uint) []register.Id {
	ids := make([]register.Id, numOutputs)
	//
	for i := range numOutputs {
		ids[i] = register.NewId(1 + i)
	}
	//
	return ids
}

// writeOne writes a single-word value at the given address and panics if the
// underlying call fails.
func writeOne(t *testing.T, mem Memory[word.Uint], frame []word.Uint, addr, val uint64) {
	t.Helper()
	//
	frame[0] = uint64W(addr)
	frame[1] = uint64W(val)
	//
	if err := mem.Write(frame, addrIds(), dataIds(1)); err != nil {
		t.Fatalf("Write at %d failed: %v", addr, err)
	}
}

// readOne reads a single-word value at the given address.
func readOne(t *testing.T, mem Memory[word.Uint], frame []word.Uint, addr uint64) uint64 {
	t.Helper()
	//
	frame[0] = uint64W(addr)
	frame[1] = uint64W(0)
	//
	if err := mem.Read(frame, addrIds(), dataIds(1)); err != nil {
		t.Fatalf("Read at %d failed: %v", addr, err)
	}
	//
	return frame[1].Uint64()
}

// Test_BiPartite_Lower_WriteRead exercises a write-then-read in the lower
// partition.
func Test_BiPartite_Lower_WriteRead(t *testing.T) {
	cases := map[uint64]uint64{
		0:    42,
		1:    7,
		100:  99,
		1000: 123,
	}
	//
	checkReadWriteWords(t, cases, nil)
}

// Test_BiPartite_Upper_WriteRead exercises a write-then-read in the upper
// partition.  All addresses are kept close to TOP_POS to avoid huge
// allocations.
func Test_BiPartite_Upper_WriteRead(t *testing.T) {
	cases := map[uint64]uint64{
		TOP_POS:       11,
		TOP_POS - 1:   22,
		TOP_POS - 5:   33,
		TOP_POS - 100: 44,
	}
	//
	checkReadWriteWords(t, cases, nil)
}

// Test_BiPartite_Partition_Boundary verifies that addresses on either side of
// the HALF_START boundary land in the correct partition.  HALF_START itself
// is the lowest upper address; reading it on an empty upper partition must
// return zero.
func Test_BiPartite_Partition_Boundary(t *testing.T) {
	writes := map[uint64]uint64{
		1000:    1, // lower
		TOP_POS: 2, // upper
	}
	//
	checkReadWriteWords(t, writes, []uint64{HALF_START})
}

// Test_BiPartite_Independence verifies that writes in one partition do not
// affect adjacent (unwritten) addresses in the other.
func Test_BiPartite_Independence(t *testing.T) {
	writes := map[uint64]uint64{
		5:           100,
		TOP_POS - 5: 200,
	}
	//
	checkReadWriteWords(t, writes, []uint64{6, TOP_POS - 6})
}

// Test_BiPartite_OutOfBounds verifies that reading addresses beyond what has
// been written still returns zero (in both partitions).
func Test_BiPartite_OutOfBounds(t *testing.T) {
	writes := map[uint64]uint64{
		5:           99,
		TOP_POS - 5: 77,
	}
	//
	checkReadWriteWords(t, writes, []uint64{1000, TOP_POS - 1000})
}

// Test_BiPartite_DoubleWord_Lower_WriteRead exercises a write-then-read in
// the lower partition of a numOutputs=2 memory.
func Test_BiPartite_DoubleWord_Lower_WriteRead(t *testing.T) {
	cases := map[uint64][2]uint64{
		0:    {1, 2},
		1:    {3, 4},
		100:  {5, 6},
		1000: {7, 8},
	}
	//
	checkReadWriteDoubleWords(t, cases, nil)
}

// Test_BiPartite_DoubleWord_Upper_WriteRead exercises a write-then-read in
// the upper partition of a numOutputs=2 memory.  Addresses are kept near
// HALF_START so that the encoded start (addr*2) lands near TOP_POS.
func Test_BiPartite_DoubleWord_Upper_WriteRead(t *testing.T) {
	cases := map[uint64][2]uint64{
		HALF_START:       {11, 12},
		HALF_START - 1:   {13, 14},
		HALF_START - 5:   {15, 16},
		HALF_START - 100: {17, 18},
	}
	//
	checkReadWriteDoubleWords(t, cases, nil)
}

// Test_BiPartite_DoubleWord_Independence verifies that two-word writes in one
// partition do not affect adjacent (unwritten) addresses in the other.
func Test_BiPartite_DoubleWord_Independence(t *testing.T) {
	writes := map[uint64][2]uint64{
		5:              {100, 101},
		HALF_START - 5: {200, 201},
	}
	//
	checkReadWriteDoubleWords(t, writes, []uint64{6, HALF_START - 6})
}

// Test_BiPartite_MultiWord exercises a memory with numOutputs == 2, so each
// address row holds two words.  Writes index 5, then verifies the adjacent
// (unwritten) index 6 reads back as zero.
func Test_BiPartite_MultiWord(t *testing.T) {
	writes := map[uint64][2]uint64{
		5: {11, 22},
	}
	//
	checkReadWriteDoubleWords(t, writes, []uint64{6})
}

// Test_BiPartite_Upper_Overflow regression-tests the case where a multi-word
// write near TOP_POS extends past the end of the addressable range.  Without
// the bounds cap in Write, `start + i` wraps uint64 and indexes the upper
// slice at a huge value, panicking.  Cells whose position would exceed
// TOP_POS must be silently dropped on write and read back as zero.
//
// We use numOutputs=3 with a 64-bit address: there exists an address such
// that 3*addr mod 2^64 == TOP_POS, so the row's positions are TOP_POS,
// TOP_POS+1 (wrap), TOP_POS+2 (wrap) — only the first fits.
func Test_BiPartite_Upper_Overflow(t *testing.T) {
	regs := []register.Register{
		register.NewInput("addr", 64, *big.NewInt(0)),
		register.NewOutput("d0", 64, *big.NewInt(0)),
		register.NewOutput("d1", 64, *big.NewInt(0)),
		register.NewOutput("d2", 64, *big.NewInt(0)),
	}
	//
	mem := NewBiPartiteRandomAccess[word.Uint]("test", regs)
	frame := make([]word.Uint, 4)
	// 3 * 6148914691236517205 == 2^64 - 1 == TOP_POS (mod 2^64), so start
	// lands exactly at TOP_POS and only one cell fits.
	const addr uint64 = 6148914691236517205
	//
	frame[0] = uint64W(addr)
	frame[1] = uint64W(11)
	frame[2] = uint64W(22)
	frame[3] = uint64W(33)
	//
	if err := mem.Write(frame, addrIds(), dataIds(3)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	// Clear data slots, then read back.
	frame[1] = uint64W(0)
	frame[2] = uint64W(0)
	frame[3] = uint64W(0)
	//
	if err := mem.Read(frame, addrIds(), dataIds(3)); err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	// First cell is in range and must round-trip; the other two are past
	// TOP_POS and must read as zero.
	if got := frame[1].Uint64(); got != 11 {
		t.Errorf("data[0]: expected 11, got %d", got)
	}
	//
	if got := frame[2].Uint64(); got != 0 {
		t.Errorf("data[1] (overflow): expected 0, got %d", got)
	}
	//
	if got := frame[3].Uint64(); got != 0 {
		t.Errorf("data[2] (overflow): expected 0, got %d", got)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func checkReadWriteWords(t *testing.T, writes map[uint64]uint64, reads []uint64) {
	mem, frame := newBiPartite(64, 1)
	//
	for addr, val := range writes {
		writeOne(t, mem, frame, addr, val)
	}
	//
	for addr, want := range writes {
		if got := readOne(t, mem, frame, addr); got != want {
			t.Errorf("[%d]: expected %d, got %d", addr, want, got)
		}
	}
	// Check for non-interference
	for _, addr := range reads {
		if got := readOne(t, mem, frame, addr); got != 0 {
			t.Errorf("[%d]: expected 0, got %d", addr, got)
		}
	}
}

// checkReadWriteDoubleWords is the two-word analogue of checkReadWriteWords.
// It builds a bipartite memory with numOutputs=2, writes each (addr, [v0,v1])
// pair from writes, reads them back, and verifies that the addresses listed
// in reads return (0,0).  A 63-bit address register is used so that
// start = addr*2 stays within uint64 and addresses near HALF_START remain
// usable for exercising the upper partition.
func checkReadWriteDoubleWords(t *testing.T, writes map[uint64][2]uint64, reads []uint64) {
	mem, frame := newBiPartite(63, 2)
	//
	for addr, vals := range writes {
		writeTwo(t, mem, frame, addr, vals[0], vals[1])
	}
	//
	for addr, want := range writes {
		v0, v1 := readTwo(t, mem, frame, addr)
		if v0 != want[0] || v1 != want[1] {
			t.Errorf("[%d]: expected (%d,%d), got (%d,%d)", addr, want[0], want[1], v0, v1)
		}
	}
	// Check for non-interference
	for _, addr := range reads {
		v0, v1 := readTwo(t, mem, frame, addr)
		if v0 != 0 || v1 != 0 {
			t.Errorf("[%d]: expected (0,0), got (%d,%d)", addr, v0, v1)
		}
	}
}

// writeTwo writes a two-word value at the given address.
func writeTwo(t *testing.T, mem Memory[word.Uint], frame []word.Uint, addr, v0, v1 uint64) {
	t.Helper()
	//
	frame[0] = uint64W(addr)
	frame[1] = uint64W(v0)
	frame[2] = uint64W(v1)
	//
	if err := mem.Write(frame, addrIds(), dataIds(2)); err != nil {
		t.Fatalf("Write at %d failed: %v", addr, err)
	}
}

// readTwo reads a two-word value at the given address.
func readTwo(t *testing.T, mem Memory[word.Uint], frame []word.Uint, addr uint64) (uint64, uint64) {
	t.Helper()
	//
	frame[0] = uint64W(addr)
	frame[1] = uint64W(0)
	frame[2] = uint64W(0)
	//
	if err := mem.Read(frame, addrIds(), dataIds(2)); err != nil {
		t.Fatalf("Read at %d failed: %v", addr, err)
	}
	//
	return frame[1].Uint64(), frame[2].Uint64()
}
