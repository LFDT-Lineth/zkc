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
	// cached accessLog.Records(): true only while a recording log is installed,
	// so the hot read/write path can skip the log call without an interface
	// dispatch.  Set by SetLog; false for the default (checkpointing) log.
	logging bool
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

// AccessLog implementation for the Memory interface: the chronological access
// log (empty unless a recording log is installed for trace generation).
func (p *PagedRandomAccess[W]) AccessLog() []AccessData[W] {
	return p.log().Accesses()
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
// It caches whether the log records, so the hot path can skip logging cheaply.
func (p *PagedRandomAccess[W]) SetLog(log Log[W]) {
	p.accessLog = log
	p.logging = log.Records()
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

// Read returns the value at the given address (the zero value for an address
// whose page has never been allocated).  A read re-stamps an already-allocated
// cell with the current clock but never allocates a page, preserving sparsity.
func (p *PagedRandomAccess[W]) Read(address uint64) (W, error) {
	var (
		page      = address / PAGE_SIZE
		offset    = address % PAGE_SIZE
		allocated = page < uint64(len(p.pages)) && p.pages[page] != nil
		cell      TimestampedCell[W]
	)
	//
	if allocated {
		cell = p.pages[page][offset]
		// Re-stamp the existing cell (value unchanged); never allocate on a read.
		p.pages[page][offset] = TimestampedCell[W]{timestamp: p.timestamp, value: cell.value}
	}
	//
	if p.logging {
		// Write-side only; the read-side is reconstructed at trace time by a
		// state-tracking observer (see Log).
		p.accessLog.Read(address, cell.timestamp, p.timestamp, cell.value)
	}
	//
	return cell.value, nil
}

// Write stores value at the given address, allocating the enclosing page if
// needed, and stamps the cell with the current clock.
func (p *PagedRandomAccess[W]) Write(address uint64, value W) error {
	var (
		page   = address / PAGE_SIZE
		offset = address % PAGE_SIZE
	)
	// Materialise the page if needed, then re-stamp the cell.
	p.pages = expand(p.pages, page+1)
	//
	if p.pages[page] == nil {
		p.pages[page] = make([]TimestampedCell[W], PAGE_SIZE)
	}
	// read out current contents of call
	curr := p.pages[page][offset]
	//
	p.pages[page][offset] = TimestampedCell[W]{timestamp: p.timestamp, value: value}
	//
	if p.logging {
		p.accessLog.Write(address, curr.timestamp, p.timestamp, curr.value, value)
	}
	//
	return nil
}

// Tick advances the access clock by one.  Called once per logical memory access
// (one RD_PRAM/WR_PRAM instruction), before any of its data lanes are touched,
// so all lanes share one timestamp.  See RandomAccess.Tick for the rationale.
func (p *PagedRandomAccess[W]) Tick() {
	p.timestamp++
}

// Contents implementation for Memory interface.
func (p *PagedRandomAccess[W]) Contents() []W {
	panic("unsupported operation")
}

// Checkpoint implementation for memory interface
func (p *PagedRandomAccess[W]) Checkpoint(mid uint16) checkpoint.Memory {
	var pages []checkpoint.Page
	//
	for i, page := range p.pages {
		if page != nil {
			var (
				address       = uint64(i) * PAGE_SIZE
				bytes, stamps = PackTimed(p.descriptor.DataRegisters(), page)
			)
			//
			pages = append(pages, checkpoint.NewStampedPage(address, bytes, stamps))
		}
	}
	//
	return checkpoint.NewMemory(mid, p.timestamp, pages...)
}

// Restore implementation for memory interface
func (p *PagedRandomAccess[W]) Restore(m checkpoint.Memory) {
	//
	p.timestamp = m.Clock()
	p.log().Reset()
	//
	for _, page := range m.Pages() {
		var (
			pageIdx = page.Address() / PAGE_SIZE
		)
		//
		p.pages = expand(p.pages, pageIdx+1)
		p.pages[pageIdx] = UnpackTimed(p.descriptor, page.Bytes(), page.Stamps())
	}
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
