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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// PAGE_SIZE determines the number of words in a single page of a
// PagedRandomAccess memory.  This is fixed at 1M words.
const PAGE_SIZE uint64 = 1 << 20

// PagedRandomAccess provides a read/write implementation of Memory which
// represents memory as a single array of pages (each of PAGE_SIZE words).  The
// page table grows on demand as higher addresses are written, whilst
// individual pages are only allocated on their first write.  Reads of
// locations which have never been written simply return zero.  We can view
// memory as follows:
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
	// Pages of memory, allocated on demand.
	pages [][]W
}

// NewPagedRandomAccess constructs a new paged random access memory.
func NewPagedRandomAccess[W word.Word[W]](descriptor descriptor.Memory[W]) *PagedRandomAccess[W] {
	return &PagedRandomAccess[W]{
		descriptor: descriptor,
	}
}

// Descriptor implementation for Memory interface.
func (p *PagedRandomAccess[W]) Descriptor() *descriptor.Memory[W] {
	return &p.descriptor
}

// Initialise implementation for Memory interface.  The provided contents
// populate memory starting from address zero; all other locations are cleared.
func (p *PagedRandomAccess[W]) Initialise(contents []W) {
	p.pages = nil
	//
	for len(contents) > 0 {
		var (
			n    = min(uint64(len(contents)), PAGE_SIZE)
			page = make([]W, PAGE_SIZE)
		)
		//
		copy(page, contents[:n])
		p.pages = append(p.pages, page)
		contents = contents[n:]
	}
}

// Read implementation for Memory interface.
func (p *PagedRandomAccess[W]) Read(address uint64) (W, error) {
	var (
		val    W
		page   = address / PAGE_SIZE
		offset = address % PAGE_SIZE
	)
	//
	if page < uint64(len(p.pages)) && p.pages[page] != nil {
		val = p.pages[page][offset]
	}
	//
	return val, nil
}

// Write implementation for Memory interface.
func (p *PagedRandomAccess[W]) Write(address uint64, value W) error {
	var (
		page   = address / PAGE_SIZE
		offset = address % PAGE_SIZE
	)
	// extend page table if needed
	p.pages = expand(p.pages, page+1)
	// allocate page if needed
	if p.pages[page] == nil {
		p.pages[page] = make([]W, PAGE_SIZE)
	}
	//
	p.pages[page][offset] = value
	//
	return nil
}

// Contents implementation for Memory interface.
func (p *PagedRandomAccess[W]) Contents() []W {
	panic("unsupported operation")
}

// Pages returns an iterator over the allocated pages backing this memory.  Each
// yielded page covers the PAGE_SIZE words beginning at physical address
// i*PAGE_SIZE (where i is its page number); pages which have never been written
// are omitted.  The backing data slices are referenced directly (not copied),
// so callers must not mutate them.
func (p *PagedRandomAccess[W]) Pages() iter.Iterator[checkpoint.Page[W]] {
	var pages []checkpoint.Page[W]
	//
	for i, page := range p.pages {
		if page != nil {
			var address = uint64(i) * PAGE_SIZE

			pages = append(pages, checkpoint.NewPage(address, page))
		}
	}
	//
	return iter.NewArrayIterator(pages)
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
