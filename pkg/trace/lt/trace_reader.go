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
package lt

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FromBytesV2 parses a byte array representing a given LTv2 trace file into a set
// of columns, or produces an error if the original file was malformed in some
// way.
func FromBytesV2(data []byte) (WordHeap, []Module[word.BigEndian], error) {
	return fromBytes(data, 2)
}

// FromBytesV3 parses a byte array representing a given LTv3 trace file into a set
// of columns, or produces an error if the original file was malformed in some
// way.
func FromBytesV3(data []byte) (WordHeap, []Module[word.BigEndian], error) {
	return fromBytes(data, 3)
}

// FromBytes parses a byte array representing a given LTv2 trace file into a set
// of columns, or produces an error if the original file was malformed in some
// way.  The file is organised into three sections:
//
//	+-----------------+
//	| HEADER          |
//	+-----------------+
//	| HEAP            |
//	+-----------------+
//	| COLUMNS         |
//	+-----------------+
//
// Here, the HEADER consists primarily of one or more module definitions, each
// of which identifies a module and the columns it contains.  Likewise, the HEAP
// represents a collection of indexed word data.  Finally, COLUMNS contains
// concrete column data which is either represented explicitly (for narrow
// words, like u8) or using indexes into the heap (for wide words, like u256).
func fromBytes(data []byte, version uint) (WordHeap, []Module[word.BigEndian], error) {
	var (
		err                    error
		buf                    = bytes.NewReader(data)
		heap                   = pool.NewLocalHeap[word.BigEndian]()
		builder                = array.NewDynamicBuilder(heap)
		headerBytes, heapBytes uint32
		headers                []moduleHeader
		offset                 uint
	)
	// Determine sizes of all three sections
	if headerBytes, heapBytes, err = readSectionSizes(buf); err != nil {
		return WordHeap{}, nil, err
	}
	// Read header
	if headers, err = readModuleHeaders(buf); err != nil {
		return WordHeap{}, nil, err
	}
	// Read heap
	switch version {
	case 2:
		if err := heap.UnmarshalBinaryV2(data[8+headerBytes : 8+headerBytes+heapBytes]); err != nil {
			return WordHeap{}, nil, err
		}
	case 3:
		if err := heap.UnmarshalBinaryV3(data[8+headerBytes : 8+headerBytes+heapBytes]); err != nil {
			return WordHeap{}, nil, err
		}
	default:
		panic(fmt.Sprintf("unsupported trace file format (v%d)", version))
	}
	//
	modules := make([]Module[word.BigEndian], len(headers))
	offset = uint(8 + headerBytes + heapBytes)
	// Read column data (sequentially)
	for i, ith := range headers {
		var columns = make([]Column[word.BigEndian], len(ith.columns))
		//
		for j, jth := range ith.columns {
			var encoding = array.Encoding{
				Encoding: jth.encoding,
				Bytes:    data[offset : offset+uint(jth.length)],
			}
			// Decode array data
			data := builder.Decode(encoding)
			// Include it
			columns[j] = Column[word.BigEndian]{jth.name, data}
			// Move read offset
			offset += uint(jth.length)
		}
		//
		modules[i] = Module[word.BigEndian]{
			name:    trace.ParseModuleName(ith.name),
			Columns: columns,
		}
	}
	// Done
	return *heap, modules, nil
}

func readSectionSizes(buf *bytes.Reader) (headerBytes, heapBytes uint32, err error) {
	// Read size of header (in bytes)
	if err := binary.Read(buf, binary.BigEndian, &headerBytes); err != nil {
		return headerBytes, heapBytes, err
	}
	// Read size of heap (in bytes)
	if err := binary.Read(buf, binary.BigEndian, &heapBytes); err != nil {
		return headerBytes, heapBytes, err
	}
	// done
	return headerBytes, heapBytes, nil
}

func readModuleHeaders(buf *bytes.Reader) (headers []moduleHeader, err error) {
	var nModules uint32
	// Read number of modules
	if err := binary.Read(buf, binary.BigEndian, &nModules); err != nil {
		return nil, err
	}
	//
	headers = make([]moduleHeader, nModules)
	//
	for i := range headers {
		if headers[i], err = readModuleHeader(buf); err != nil {
			return headers, err
		}
	}
	// Done
	return headers, nil
}

func readModuleHeader(buf *bytes.Reader) (header moduleHeader, err error) {
	var nColumns uint32
	// Read module name
	if header.name, err = readName(buf); err != nil {
		return header, err
	}
	// Read module height
	if err := binary.Read(buf, binary.BigEndian, &header.height); err != nil {
		return header, err
	}
	// Read number of columns
	if err := binary.Read(buf, binary.BigEndian, &nColumns); err != nil {
		return header, err
	}
	// Read columns
	header.columns = make([]columnHeader, nColumns)
	//
	//
	for i := range header.columns {
		if header.columns[i], err = readColumnHeader(buf); err != nil {
			return header, err
		}
	}
	//
	return header, nil
}

func readColumnHeader(buf *bytes.Reader) (header columnHeader, err error) {
	// Read column name
	if header.name, err = readName(buf); err != nil {
		return header, err
	}
	// Read column data length (in bytes)
	if err := binary.Read(buf, binary.BigEndian, &header.length); err != nil {
		return header, err
	}
	// read column data encoding
	if err := binary.Read(buf, binary.BigEndian, &header.encoding); err != nil {
		return header, err
	}
	// read column bitwidth
	if err := binary.Read(buf, binary.BigEndian, &header.bitwidth); err != nil {
		return header, err
	}
	// Done
	return header, nil
}

func readName(buf *bytes.Reader) (string, error) {
	var (
		// Qualified column name length
		nameLen uint16
	)
	// Read name length
	if err := binary.Read(buf, binary.BigEndian, &nameLen); err != nil {
		return "", err
	}
	// Read name bytes
	name := make([]byte, nameLen)
	if _, err := buf.Read(name); err != nil {
		return "", err
	}
	// Done
	return string(name), nil
}

type moduleHeader struct {
	// Name of the module
	name string
	// Height (in rows) of the module
	height uint32
	// Column details
	columns []columnHeader
}

type columnHeader struct {
	// Name of the column
	name string
	// Length (in bytes) of column data
	length uint32
	// encoding scheme used for column data
	encoding uint32
	// Bitwidth of column
	bitwidth uint16
}
