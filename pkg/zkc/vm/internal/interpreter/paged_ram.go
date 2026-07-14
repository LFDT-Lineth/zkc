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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// PAGE_SIZE determines the number of words in a single page of a
// PagedRandomAccess memory.  This is fixed at 1M words.
const PAGE_SIZE uint64 = 1 << 20

// PagedRandomAccess provides a read/write implementation of Memory which
// represents memory as a single array of pages (each of PAGE_SIZE timestamped
// cells).  The page table grows on demand as higher addresses are written,
// whilst individual pages are only allocated on their first write.  Reads of
// locations which have never been written simply return zero and, to preserve
// sparsity, never allocate a page.  We can view memory as follows:
//
// +--------+--------+ ................. +
// | page 0 | page 1 |  (unallocated)
// +--------+--------+ ................. +
//
//	0        PAGE_SIZE
//
// Since the page table is indexed densely by address / PAGE_SIZE, this
// representation assumes programs do not access very high addresses.
type PagedRandomAccess[W word.Word[W]] struct {
	descriptor descriptor.Memory[W]
	// Pages of timestamped cells, allocated on demand.
	pages [][]TimestampedCell[W]
	// timestamp; increments by 1 with every memory access
	timestamp uint64
	// logs memory accesses in chronological order (no-op unless tracing)
	accessLog Log[W]
}

// NewPagedRandomAccess constructs a new paged random access memory.
func NewPagedRandomAccess[W word.Word[W]](descriptor descriptor.Memory[W]) *PagedRandomAccess[W] {
	return &PagedRandomAccess[W]{
		descriptor: descriptor,
		accessLog:  &CheckpointingMemoryLog[W]{},
	}
}

// Descriptor implementation for Memory interface.
func (p *PagedRandomAccess[W]) Descriptor() *descriptor.Memory[W] {
	return &p.descriptor
}

// log returns this memory's access log, lazily installing the no-op log if none
// has been set.  Trace generation installs a recording log via SetLog.
func (p *PagedRandomAccess[W]) log() Log[W] {
	if p.accessLog == nil {
		p.accessLog = &CheckpointingMemoryLog[W]{}
	}
	//
	return p.accessLog
}

// SetLog installs the given access log; used to switch a RAM into tracing mode.
func (p *PagedRandomAccess[W]) SetLog(log Log[W]) {
	p.accessLog = log
}

// Clock returns the current value of this memory's access clock.
func (p *PagedRandomAccess[W]) Clock() uint64 {
	return p.timestamp
}

// Initialise implementation for Memory interface.  The provided contents
// populate memory starting from address zero (timestamp zero); all other
// locations are cleared.  Resets the access log and clock.
func (p *PagedRandomAccess[W]) Initialise(contents []W) {
	p.pages = nil
	//
	for len(contents) > 0 {
		var (
			n    = min(uint64(len(contents)), PAGE_SIZE)
			page = make([]TimestampedCell[W], PAGE_SIZE)
		)
		//
		for i := range n {
			page[i] = TimestampedCell[W]{value: contents[i]}
		}
		//
		p.pages = append(p.pages, page)
		contents = contents[n:]
	}
	//
	p.log().Reset()
	p.timestamp = 0
}

// Read implementation for Memory interface.
func (p *PagedRandomAccess[W]) Read(address uint64) (W, error) {
	return p.access(address, false)
}

// Write implementation for Memory interface.
func (p *PagedRandomAccess[W]) Write(address uint64, value W) error {
	_, err := p.access(address, true, value)
	//
	return err
}

// access performs a single read or write, records it in the access log, and
// returns the value read from the cell.  Every access re-stamps the touched
// cell with the current clock; writes always materialise the cell, whilst reads
// only re-stamp a cell whose page already exists (a read never allocates a page,
// preserving sparsity).
func (p *PagedRandomAccess[W]) access(address uint64, isWrite bool, newValue ...W) (W, error) {
	// newValue should hold at most one value
	if len(newValue) > 1 || (isWrite && len(newValue) != 1) || (!isWrite && len(newValue) != 0) {
		var ignored W
		return ignored, fmt.Errorf("invalid access call; isWrite = %v, len(newValues) = %d", isWrite, len(newValue))
	}
	//
	p.timestamp++
	//
	var (
		page      = address / PAGE_SIZE
		offset    = address % PAGE_SIZE
		allocated = page < uint64(len(p.pages)) && p.pages[page] != nil
		old       TimestampedCell[W]
	)
	//
	if allocated {
		old = p.pages[page][offset]
	}
	// The value written back: the new value for a write, the value just read for
	// a read.
	valueWritten := old.value
	//
	if isWrite {
		valueWritten = newValue[0]
	}
	// Materialise + re-stamp the cell.  Writes allocate the page if needed; reads
	// only re-stamp an already-allocated page (never allocate).
	if isWrite || allocated {
		p.pages = expand(p.pages, page+1)
		//
		if p.pages[page] == nil {
			p.pages[page] = make([]TimestampedCell[W], PAGE_SIZE)
		}
		//
		p.pages[page][offset] = TimestampedCell[W]{timestamp: p.timestamp, value: valueWritten}
	}
	// Log the access (write-side; the read-side is reconstructed at trace time
	// by a state-tracking observer -- see Log).
	if isWrite {
		p.log().Write(address, p.timestamp, valueWritten)
	} else {
		p.log().Read(address, p.timestamp, valueWritten)
	}
	//
	return valueWritten, nil
}

// Contents implementation for Memory interface.
func (p *PagedRandomAccess[W]) Contents() []W {
	panic("unsupported operation")
}

// Pages returns an iterator over the allocated pages backing this memory.  Each
// yielded page covers the PAGE_SIZE cells beginning at physical address
// i*PAGE_SIZE (where i is its page number), carrying both the values and their
// per-cell timestamps; pages which have never been written are omitted.
func (p *PagedRandomAccess[W]) Pages() iter.Iterator[checkpoint.Page[W]] {
	var pages []checkpoint.Page[W]
	//
	for i, page := range p.pages {
		if page != nil {
			var (
				address    = uint64(i) * PAGE_SIZE
				data       = make([]W, len(page))
				timestamps = make([]uint64, len(page))
			)
			//
			for j, cell := range page {
				data[j] = cell.value
				timestamps[j] = cell.timestamp
			}
			//
			pages = append(pages, checkpoint.NewTimestampedPage(address, data, timestamps))
		}
	}
	//
	return iter.NewArrayIterator(pages)
}

// RestoreCells re-seeds the memory from a checkpoint snapshot, installing each
// captured page at its recorded address.  Call Reset first to set the clock and
// clear any prior state.
func (p *PagedRandomAccess[W]) RestoreCells(pages []checkpoint.Page[W]) {
	for _, page := range pages {
		p.RestorePage(page.Address(), cellsFromPage(page))
	}
}

// RestorePage re-seeds a single page (beginning at the given physical address,
// which must be page-aligned) from a snapshot of timestamped cells.  Used on
// resume from a checkpoint.
func (p *PagedRandomAccess[W]) RestorePage(address uint64, cells []TimestampedCell[W]) {
	var (
		pageIdx = address / PAGE_SIZE
		page    = make([]TimestampedCell[W], PAGE_SIZE)
	)
	//
	copy(page, cells)
	//
	p.pages = expand(p.pages, pageIdx+1)
	p.pages[pageIdx] = page
}

// Reset clears all pages and the access log, and resets the clock to the given
// value.  Used on resume before the captured pages are restored.
func (p *PagedRandomAccess[W]) Reset(clock uint64) {
	p.pages = nil
	p.timestamp = clock
	p.log().Reset()
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *PagedRandomAccess[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(&p.descriptor); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.pages); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *PagedRandomAccess[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.descriptor); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.pages); err != nil {
		return err
	}
	//
	return nil
}
