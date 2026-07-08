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
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
)

// RandomAccess is a memory implementation backed by a dynamically sizing slice
// of timestamped cells; an out-of-bound read returns the zero cell.  Every
// access re-stamps the touched cell with the current clock, and (when a
// recording Log is installed) is logged for trace generation.  The type
// parameter W is the (machine) word type; the timestamp is internal bookkeeping.
type RandomAccess[W util.Uinter64] struct {
	// timestamped memory contents
	StaticArray[W, TimestampedCell[W]]
	// timestamp; increments by 1 with every memory access
	timestamp uint64
	// logs memory accesses in chronological order (no-op unless tracing)
	accessLog Log[W]
}

// Read function handles out-of-bounds accesses.
func (ram *RandomAccess[W]) Read(address uint64) (W, error) {
	return ram.access(address, false)
}

// Write stores value at the given address, wrapping it in a timestamped cell.
func (ram *RandomAccess[W]) Write(address uint64, value W) error {
	_, err := ram.access(address, true, value)
	//
	return err
}

// access performs a single read or write, records it in the access log, and
// returns the value that was read from the cell.  Following the memory-
// consistency argument (see ram.md), every access reads the cell's current
// (value, timestamp), then re-stamps the cell with the current time: a write
// stores the new value, a read stores the value back unchanged.
func (ram *RandomAccess[W]) access(address uint64, isWrite bool, newValue ...W) (W, error) {
	// newValue should hold at most one value
	if len(newValue) > 1 || (isWrite && len(newValue) != 1) || (!isWrite && len(newValue) != 0) {
		var ignored W
		return ignored, fmt.Errorf("invalid access call; isWrite = %v, len(newValues) = %d", isWrite, len(newValue))
	}

	// unexceptional accesses raise the time stamp; raising the timestamp initially
	// means that every deliberate memory access has nonzero timestamp; it follows
	// that we can identify memory cells that deliberately touched by them having
	// a nonzero timestamp; this property will be exploited in the finalization
	// phase
	ram.timestamp++

	// Read the current cell.  An out-of-range address reads as the zero cell
	// (value 0 at timestamp 0).
	var old TimestampedCell[W]
	//
	if address < uint64(len(ram.data)) {
		old = ram.data[address]
	}
	// The value written back: the new value for a write, the value just read
	// for a read (a read leaves the stored value unchanged).
	valueWritten := old.value
	//
	if isWrite {
		valueWritten = newValue[0]
	}
	//
	timestampWrite := ram.timestamp
	// Re-stamp the cell with this access's timestamp.
	ram.data = expand(ram.data, address+1)
	ram.data[address] = TimestampedCell[W]{timestamp: timestampWrite, value: valueWritten}
	// Log the access (write-side; the read-side is reconstructed at trace time
	// by a state-tracking observer -- see Log).
	if isWrite {
		ram.log().Write(address, timestampWrite, valueWritten)
	} else {
		ram.log().Read(address, timestampWrite, valueWritten)
	}
	//
	return valueWritten, nil
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
func (ram *RandomAccess[W]) SetLog(log Log[W]) {
	ram.accessLog = log
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

// Reset clears all contents and the access log, and sets the clock to the given
// value.  Used on resume before the captured cells are restored -- mirrors
// PagedRandomAccess.Reset, so both RAMs restore as Reset-then-install.
func (ram *RandomAccess[W]) Reset(clock uint64) {
	ram.data = nil
	ram.timestamp = clock
	ram.log().Reset()
}

// RestoreCells re-seeds the memory from a checkpoint snapshot's pages (e.g. on
// resume).  A flat memory is stored as a single page at address zero, so this
// unpacks that page into cells.  Call Reset first to set the clock and clear any
// prior state.  Takes the checkpoint's page representation (shared with
// PagedRandomAccess) so both memories restore identically.
func (ram *RandomAccess[W]) RestoreCells(pages []checkpoint.Page[W]) {
	// A flat memory is captured as a single page at address zero; more than one
	// page means a corrupt or mismatched snapshot.
	if len(pages) > 1 {
		panic(fmt.Sprintf("flat memory %q: expected at most one checkpoint page, got %d", ram.Name(), len(pages)))
	}
	//
	if len(pages) == 1 {
		ram.data = cellsFromPage(pages[0])
	}
}

// cellsFromPage zips a checkpoint page's parallel value/timestamp columns into a
// fresh slice of timestamped cells.  The columns are stored separately in
// checkpoint.Page because that (lower-level) package cannot reference this
// package's TimestampedCell type.  Used to turn a captured page back into the
// cells a RAM restores from (a flat RAM's single page, or each page of a paged
// RAM).
func cellsFromPage[W util.Uinter64](page checkpoint.Page[W]) []TimestampedCell[W] {
	var (
		data       = page.Data()
		timestamps = page.Timestamps()
		cells      = make([]TimestampedCell[W], len(data))
	)
	//
	for i, value := range data {
		var ts uint64
		//
		if i < len(timestamps) {
			ts = timestamps[i]
		}
		//
		cells[i] = TimestampedCell[W]{timestamp: ts, value: value}
	}
	//
	return cells
}

// Accesses returns the ordered log of reads/writes performed since the last
// Initialise.  Used by the trace observer to materialise per-access rows; nil
// unless a recording log has been installed (i.e. in tracing mode).
func (ram *RandomAccess[W]) Accesses() []AccessData[W] {
	return ram.log().Accesses()
}

// NewRandomAccess constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
//
// Note: accessLog is set to be a CheckpointingMemoryLog: no logging by default.
// The intent is that it be swapped out for a TraceableMemoryLog using SetLog
// when tracing is required.
func NewRandomAccess[W util.Uinter64](name string, geometry Geometry[W]) *RandomAccess[W] {
	return &RandomAccess[W]{
		StaticArray: NewStaticArray[W, TimestampedCell[W]](name, READWRITE_MEMORY, geometry),
		accessLog:   &CheckpointingMemoryLog[W]{},
	}
}

// TimestampedCell holds a value and the timestamp at which that data
// was written; reads and writes both update that timestamp to the
// current one
type TimestampedCell[W util.Uinter64] struct {
	timestamp uint64
	value     W
}

// NewTimestampedCell constructs a cell holding value with the given timestamp.
func NewTimestampedCell[W util.Uinter64](value W, timestamp uint64) TimestampedCell[W] {
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
