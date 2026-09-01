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
	"bytes"
	"encoding/gob"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// RandomAccess is a memory implementation backed by a dynamically sizing slice
// of timestamped cells; an out-of-bound read returns the zero cell.  Every
// access re-stamps the touched cell with the current clock, and (when a
// recording Log is installed) is logged for trace generation.  The type
// parameter W is the (machine) word type; the timestamp is internal bookkeeping.
type RandomAccess[W word.Word[W]] struct {
	// timestamped memory contents
	StaticArray[W, TimestampedCell[W]]
	// timestamp; increments by 1 with every memory access
	timestamp uint64
	// logs memory accesses in chronological order (no-op unless tracing)
	accessLog Log[W]
	// cached accessLog.Records(): true only while a recording log is installed,
	// so the hot read/write path can skip the log call without an interface
	// dispatch.  Set by SetLog; false for the default (checkpointing) log.
	logging bool
}

// Read returns the value at the given address (the zero value for an address
// never written), re-stamping the cell with the current clock as required by
// the memory-consistency argument (see ram.md); a read leaves the stored value
// unchanged.  Out-of-bounds reads are handled (they return the zero value).
func (ram *RandomAccess[W]) Read(address uint64) (W, error) {
	value, readStamp := ram.valueAt(address)
	// A read stores the value back unchanged, re-stamped with this access's time.
	ram.restamp(address, value)
	//
	if ram.logging {
		// Write-side only; the read-side is reconstructed at trace time by a
		// state-tracking observer (see Log).
		ram.accessLog.Read(address, readStamp, ram.timestamp, value)
	}
	//
	return value, nil
}

// Write stores value at the given address, wrapping it in a timestamped cell
// stamped with the current clock.
func (ram *RandomAccess[W]) Write(address uint64, newValue W) error {
	oldValue, readStamp := ram.valueAt(address)
	//
	ram.restamp(address, newValue)
	//
	if ram.logging {
		ram.accessLog.Write(address, readStamp, ram.timestamp, oldValue, newValue)
	}
	//
	return nil
}

// Tick advances the access clock by one.  Called once per logical memory access
// (one RD_RAM/WR_RAM instruction), BEFORE any of its data lanes are touched, so
// all lanes of a single access share one timestamp -- matching the RAM
// constraints, where TIMESTAMP_READ/TIMESTAMP_WRITTEN are single columns while
// the address and value columns are lane-sliced (see ram.md).  Advancing before
// the access also keeps every deliberately-touched cell at a nonzero timestamp,
// so touched cells stay identifiable in the finalization phase.
func (ram *RandomAccess[W]) Tick() {
	ram.timestamp++
}

// valueAt returns the value stored at address, or the zero value if the address
// has never been written (including out-of-range addresses).
func (ram *RandomAccess[W]) valueAt(address uint64) (W, uint64) {
	var value W
	//
	if address < uint64(len(ram.data)) {
		cell := ram.data[address]
		//
		return cell.value, cell.timestamp
	}
	//
	return value, 0
}

// restamp stores value at address, stamped with the current clock, growing the
// backing slice as needed.
func (ram *RandomAccess[W]) restamp(address uint64, value W) {
	ram.data = expand(ram.data, address+1)
	ram.data[address] = TimestampedCell[W]{timestamp: ram.timestamp, value: value}
}

// log returns this memory's access log, lazily installing the no-op log if none
// has been set (e.g. on a freshly gob-decoded memory).  Trace generation
// installs a recording log via SetLog before execution.
func (ram *RandomAccess[W]) log() Log[W] {
	if ram.accessLog == nil {
		ram.accessLog = &CheckpointingMemoryLog[W]{}
	}
	//
	return ram.accessLog
}

// SetLog installs the given access log; used to switch a RAM into tracing mode.
// It caches whether the log records, so the hot path can skip logging cheaply.
func (ram *RandomAccess[W]) SetLog(log Log[W]) {
	ram.accessLog = log
	ram.logging = log.Records()
}

// Contents returns the stored values (dropping timestamps), so RandomAccess
// still satisfies the Memory[W] interface.
func (ram *RandomAccess[W]) Contents() []W {
	values := make([]W, len(ram.data))
	//
	for i, cell := range ram.data {
		values[i] = cell.value
	}
	//
	return values
}

// Cells returns the underlying timestamped storage.  Used by the checkpoint to
// snapshot RAM together with its per-cell timestamps.
func (ram *RandomAccess[W]) Cells() []TimestampedCell[W] {
	return ram.data
}

// Clock returns the current value of this memory's access clock (the timestamp
// of the most recent access).  Saved alongside the cells by the checkpoint.
func (ram *RandomAccess[W]) Clock() uint64 {
	return ram.timestamp
}

// Initialise seeds the backing array from a slice of raw values, wrapping each
// in a timestamped cell (timestamp zero).  Resets the access log and clock for
// a fresh execution.
//
// Note: Initialise is the wrong method for restoring a checkpoint. This task
// befalls the RestoreCells method
func (ram *RandomAccess[W]) Initialise(contents []W) {
	cells := make([]TimestampedCell[W], len(contents))
	//
	for i, value := range contents {
		cells[i] = TimestampedCell[W]{value: value}
	}
	//
	ram.StaticArray.Initialise(cells)
	// Reset the access log and clock for a fresh execution.
	ram.log().Reset()
	ram.timestamp = 0
}

// Checkpoint implementation for memory interface
func (ram *RandomAccess[W]) Checkpoint(mid uint16, field word.Config) checkpoint.Memory {
	var (
		bytes, stamps = PackTimed(field, ram.descriptor.DataRegisters(), ram.data)
		page          = checkpoint.NewStampedPage(0, bytes, stamps)
	)

	return checkpoint.NewMemory(mid, ram.timestamp, page)
}

// Restore implementation for memory interface
func (ram *RandomAccess[W]) Restore(m checkpoint.Memory, field word.Config) {
	var pages = m.Pages()
	// Sanity check
	util.Assert(len(pages) == 1, "random access memory requires one page")
	// Unpack data
	ram.timestamp = m.Clock()
	ram.log().Reset()
	ram.data = UnpackTimed(field, ram.descriptor, pages[0].Bytes(), pages[0].Stamps())
}

// AccessLog returns the ordered log of reads/writes performed since the last
// Initialise.  Used by the trace observer to materialise per-access rows; nil
// unless a recording log has been installed (i.e. in tracing mode).
func (ram *RandomAccess[W]) AccessLog() []AccessData[W] {
	return ram.log().Accesses()
}

// NewRandomAccess constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
//
// Note: accessLog is set to be a CheckpointingMemoryLog: no logging by default.
// The intent is that it be swapped out for a TraceableMemoryLog using SetLog
// when tracing is required.
func NewRandomAccess[W word.Word[W]](descriptor descriptor.Memory[W]) *RandomAccess[W] {
	return &RandomAccess[W]{
		StaticArray: NewStaticArray[W, TimestampedCell[W]](descriptor),
		accessLog:   &CheckpointingMemoryLog[W]{},
	}
}

// TimestampedCell holds a value and the timestamp at which that data
// was written; reads and writes both update that timestamp to the
// current one
type TimestampedCell[W word.Word[W]] struct {
	timestamp uint64
	value     W
}

// NewTimestampedCell constructs a cell holding value with the given timestamp.
func NewTimestampedCell[W word.Word[W]](value W, timestamp uint64) TimestampedCell[W] {
	return TimestampedCell[W]{timestamp: timestamp, value: value}
}

// Value returns the value stored in this cell.
func (tsmc TimestampedCell[W]) Value() W { return tsmc.value }

// Timestamp returns the timestamp at which this cell was last touched.
func (tsmc TimestampedCell[W]) Timestamp() uint64 { return tsmc.timestamp }

// Uint64 method from the util.Uinter64 interface
func (tsmc TimestampedCell[W]) Uint64() uint64 {
	return tsmc.value.Uint64()
}

// GobEncode lets a TimestampedCell be gob-serialised despite its unexported
// fields (gob rejects a struct with no exported fields otherwise).
func (tsmc TimestampedCell[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer

	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(tsmc.timestamp); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&tsmc.value); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// GobDecode is the counterpart to GobEncode.
func (tsmc *TimestampedCell[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&tsmc.timestamp); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&tsmc.value); err != nil {
		return err
	}
	//
	return nil
}
