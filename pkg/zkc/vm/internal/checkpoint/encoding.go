// Copyright Consensys Incorporated
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
package checkpoint

import (
	"encoding/binary"
	"errors"
	"fmt"

	bytedec "github.com/LFDT-Lineth/zkc/pkg/util/collection/byte"
)

// Binary encoding of a checkpoint.  All integers are big endian.  The format
// is deliberately trivial so that it can be emitted directly by generated code
// (which cannot import this package) as well as by the interpreter.
//
//	CheckPoint := u16 function
//	              u32 len(arguments) arguments
//	              u32 len(memories)  Memory*
//	Memory     := u16 moduleId
//	              u64 clock
//	              u32 len(pages)     Page*
//	Page       := u64 address
//	              u32 len(data)      data
//	              u32 len(stamps)    u64*
//
// Zero-length arrays always decode as nil slices.  In particular, a page with
// zero timestamps decodes with nil timestamps (i.e. it is treated as
// unstamped).
const (
	// checkPointHeaderSize is the size of the fixed part of a checkpoint.
	checkPointHeaderSize = 2 + 4 + 4
	// memoryHeaderSize is the size of the fixed part of a memory.
	memoryHeaderSize = 2 + 8 + 4
	// pageHeaderSize is the size of the fixed part of a page.
	pageHeaderSize = 8 + 4 + 4
)

// ErrMalformed signals that the bytes given to Unmarshal do not form a valid
// checkpoint.
var ErrMalformed = errors.New("malformed checkpoint")

// Marshal this checkpoint into a freshly allocated byte array.  The result is
// sized exactly, so a single allocation is made.
func (p CheckPoint) Marshal() []byte {
	var buf = make([]byte, 0, p.marshalledSize())
	//
	buf = binary.BigEndian.AppendUint16(buf, p.function)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.arguments)))
	buf = append(buf, p.arguments...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.memory)))
	//
	for _, m := range p.memory {
		buf = binary.BigEndian.AppendUint16(buf, m.moduleId)
		buf = binary.BigEndian.AppendUint64(buf, m.clock)
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(m.pages)))
		//
		for _, pg := range m.pages {
			buf = binary.BigEndian.AppendUint64(buf, pg.address)
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(pg.data)))
			buf = append(buf, pg.data...)
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(pg.timestamps)))
			//
			for _, ts := range pg.timestamps {
				buf = binary.BigEndian.AppendUint64(buf, ts)
			}
		}
	}
	//
	return buf
}

// marshalledSize computes the exact number of bytes produced by Marshal.
func (p CheckPoint) marshalledSize() int {
	var n = checkPointHeaderSize + len(p.arguments)
	//
	for _, m := range p.memory {
		n += memoryHeaderSize
		//
		for _, pg := range m.pages {
			n += pageHeaderSize + len(pg.data) + 8*len(pg.timestamps)
		}
	}
	//
	return n
}

// Unmarshal this checkpoint from a given array of bytes, as produced by
// Marshal.  To avoid copying, the arguments and page data of the resulting
// checkpoint are subslices of bytes; hence, bytes must not be modified
// afterwards.  Trailing bytes beyond the encoded checkpoint are rejected.
func (p *CheckPoint) Unmarshal(bytes []byte) error {
	var (
		d   = bytedec.NewDecoder(bytes)
		err error
	)
	//
	if p.function, err = d.Uint16(); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	} else if p.arguments, err = d.Bytes(); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	} else if p.memory, err = unmarshalMemories(&d); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	} else if n := d.Remaining(); n != 0 {
		return fmt.Errorf("%w: %d trailing bytes", ErrMalformed, n)
	}
	//
	return nil
}

func unmarshalMemories(d *bytedec.Decoder) ([]Memory, error) {
	n, err := d.Count(memoryHeaderSize)
	//
	if err != nil {
		return nil, err
	} else if n == 0 {
		return nil, nil
	}
	//
	memories := make([]Memory, n)
	//
	for i := range memories {
		m := &memories[i]
		//
		if m.moduleId, err = d.Uint16(); err != nil {
			return nil, err
		} else if m.clock, err = d.Uint64(); err != nil {
			return nil, err
		} else if m.pages, err = unmarshalPages(d); err != nil {
			return nil, err
		}
	}
	//
	return memories, nil
}

func unmarshalPages(d *bytedec.Decoder) ([]Page, error) {
	n, err := d.Count(pageHeaderSize)
	//
	if err != nil {
		return nil, err
	} else if n == 0 {
		return nil, nil
	}
	//
	pages := make([]Page, n)
	//
	for i := range pages {
		pg := &pages[i]
		//
		if pg.address, err = d.Uint64(); err != nil {
			return nil, err
		} else if pg.data, err = d.Bytes(); err != nil {
			return nil, err
		} else if pg.timestamps, err = d.Uint64s(); err != nil {
			return nil, err
		}
	}
	//
	return pages, nil
}
