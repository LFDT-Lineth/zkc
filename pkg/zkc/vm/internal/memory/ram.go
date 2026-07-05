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
)

// RandomAccess is a memory implementation backed by a dynamically sizing []W,
// meaning that an out-of-bound read will return 0.  Reads are performed by
// delegating address decoding to a D (an AddressDecoder) which translates the
// incoming multi-word address tuple into a (start, end) index range, and then
// returning the corresponding sub-slice of the backing data.
//
// The type parameter W is the word type (e.g. a field element or big.Int), and
// D is the AddressDecoder strategy that encodes the layout of rows within the
// flat slice.
type RandomAccess[W util.Uinter64] struct {
	// timestamped memory contents
	StaticArray[W, TimestampedCell[W]]
	// timestamp; increments by 1 with every memory access
	timestamp int
	// accesses logs memory accesses in chronological order
	accesses []AccessData[W]
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
	timestampWrite := uint64(ram.timestamp)
	// Re-stamp the cell with this access's timestamp.
	ram.data = expand(ram.data, address+1)
	ram.data[address] = TimestampedCell[W]{timestamp: timestampWrite, value: valueWritten}
	// Log the access.
	ram.accesses = append(ram.accesses, AccessData[W]{
		timestampRead:  old.timestamp,
		timestampWrite: timestampWrite,
		address:        address,
		valueRead:      old.value,
		valueWritten:   valueWritten,
		isWrite:        isWrite,
	})
	//
	return old.value, nil
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

// Initialise seeds the backing array from a slice of raw values, wrapping each
// in a timestamped cell.
func (ram *RandomAccess[W]) Initialise(contents []W) {
	cells := make([]TimestampedCell[W], len(contents))
	//
	for i, value := range contents {
		cells[i] = TimestampedCell[W]{value: value}
	}
	//
	ram.StaticArray.Initialise(cells)
	// Reset the access log and clock for a fresh execution.
	ram.accesses = nil
	ram.timestamp = 0
}

// Accesses returns the ordered log of reads/writes performed since the last
// Initialise.  Used by the trace observer to materialise per-access rows.
func (ram *RandomAccess[W]) Accesses() []AccessData[W] {
	return ram.accesses
}

// NewRandomAccess constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
func NewRandomAccess[W util.Uinter64](name string, geometry Geometry[W]) *RandomAccess[W] {
	return &RandomAccess[W]{
		StaticArray: NewStaticArray[W, TimestampedCell[W]](name, READWRITE_MEMORY, geometry),
	}
}

// AccessData logs a ram access; the relevant data is
//   - the address
//   - the current value and the updated value
//   - the current timestamp and the updated timestamp
//
// as well as the knowledge of whether this access performed a write or not
type AccessData[W util.Uinter64] struct {
	address        uint64
	valueRead      W
	valueWritten   W
	timestampRead  uint64
	timestampWrite uint64
	isWrite        bool
}

// TimestampRead returns the timestamp that was recorded when
// first reading a TimestampedCell
func (a AccessData[W]) TimestampRead() uint64 { return a.timestampRead }

// TimestampWrite returns the timestamp that was recorded as the
// timestamp at which a value was written to a TimestampedCell
func (a AccessData[W]) TimestampWrite() uint64 { return a.timestampWrite }

// Address returns the flat address touched by this access.
func (a AccessData[W]) Address() uint64 { return a.address }

// ValueRead returns the value read or written by this access.
func (a AccessData[W]) ValueRead() W { return a.valueRead }

// ValueWritten returns the value read or written by this access.
func (a AccessData[W]) ValueWritten() W { return a.valueWritten }

// IsWrite reports whether this access was a write (true) or a read (false).
func (a AccessData[W]) IsWrite() bool { return a.isWrite }

// TimestampedCell holds a value and the timestamp at which that data
// was written; reads and writes both update that timestamp to the
// current one
type TimestampedCell[W util.Uinter64] struct {
	timestamp uint64
	value     W
}

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
