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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FromBytesV1 parses a byte array representing a given (legacy) LT trace
// file into an columns, or produces an error if the original file was malformed
// in some way.   The input represents the original legacy format of trace files
// (i.e. without any additional header information prepended, etc).
func FromBytesV1(data []byte) (WordHeap, []Module[word.BigEndian], error) {
	var modules []Module[word.BigEndian]
	// Read out all column data
	heap, columns, error := readLegacyBytes(data)
	// Post process into structured form
	if error == nil {
		modules = groupLegacyColumns(columns)
	}
	//
	return heap, modules, error
}

type legacyHeader struct {
	name   string
	length uint
	width  uint
}

func groupLegacyColumns(columns []Column[word.BigEndian]) []Module[word.BigEndian] {
	var (
		modules []Module[word.BigEndian]
		modmap  map[string]int = make(map[string]int)
	)
	// Process each column one by one
	for _, column := range columns {
		mod, col := splitQualifiedColumnName(column.name)
		// Check whether module already allocated
		index, ok := modmap[mod]
		//
		if !ok {
			index = len(modules)
			modules = append(modules, Module[word.BigEndian]{trace.ParseModuleName(mod), nil})
			modmap[mod] = index
		}
		// Update column name
		column.name = col
		// Group it
		modules[index].Columns = append(modules[index].Columns, column)
	}
	//
	return modules
}

func readLegacyBytes(data []byte) (WordHeap, []Column[word.BigEndian], error) {
	var (
		buf     = bytes.NewReader(data)
		heap    = pool.NewSharedHeap[word.BigEndian]()
		builder = array.NewDynamicBuilder(heap)
	)
	// Read Number of BytesColumns
	var ncols uint32
	if err := binary.Read(buf, binary.BigEndian, &ncols); err != nil {
		return WordHeap{}, nil, err
	}
	// Construct empty environment
	headers := make([]legacyHeader, ncols)
	columns := make([]Column[word.BigEndian], ncols)
	// Read column headers
	for i := uint32(0); i < ncols; i++ {
		header, err := readLegacyColumnHeader(buf)
		// Read column
		if err != nil {
			// Handle error
			return WordHeap{}, nil, err
		}
		// Assign header
		headers[i] = header
	}
	// Determine byte slices
	offset := uint(len(data) - buf.Len())
	c := make(chan util.Pair[uint, array.MutArray[word.BigEndian]], ncols)
	// Dispatch go-routines
	for i := uint(0); i < uint(ncols); i++ {
		ith := headers[i]
		// Calculate length (in bytes) of this column
		nbytes := ith.width * ith.length
		// Dispatch go-routine
		go func(i uint, offset uint) {
			// Read column data
			elements := readColumnData(ith, data[offset:offset+nbytes], builder)
			// Package result
			c <- util.NewPair(i, elements)
		}(i, offset)
		// Update byte offset
		offset += nbytes
	}
	// Collect results
	for i := uint(0); i < uint(ncols); i++ {
		// Read packaged result from channel
		res := <-c
		// Split qualified column name
		name := headers[res.Left].name
		// Construct appropriate slice
		columns[res.Left] = Column[word.BigEndian]{name, res.Right}
	}
	// Done
	return *heap.Localise(), columns, nil
}

// Read the meta-data for a specific column in this trace file.
func readLegacyColumnHeader(buf *bytes.Reader) (legacyHeader, error) {
	var header legacyHeader
	// Qualified column name length
	var nameLen uint16
	// Read column name length
	if err := binary.Read(buf, binary.BigEndian, &nameLen); err != nil {
		return header, err
	}
	// Read column name bytes
	name := make([]byte, nameLen)
	if _, err := buf.Read(name); err != nil {
		return header, err
	}

	// Read bytes per element
	var bytesPerElement uint8
	if err := binary.Read(buf, binary.BigEndian, &bytesPerElement); err != nil {
		return header, err
	}

	// Read column length
	var length uint32
	if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
		return header, err
	}
	// Height is length
	header.length = uint(length)
	header.name = string(name)
	header.width = uint(bytesPerElement)
	// Add new column
	return header, nil
}

func readColumnData(header legacyHeader, bytes []byte, heap ArrayBuilder) array.MutArray[word.BigEndian] {
	// Handle special cases
	switch header.width {
	case 1:
		// Check whether can optimise this case
		if areAllBits(bytes) {
			return readBitColumnData(header, bytes)
		}
		//
		return readByteColumnData(header.length, bytes, 0, 1)
	case 2:
		return readWordColumnData(header, bytes)
	case 4:
		return readDWordColumnData(header, bytes)
	case 8:
		return readQWordColumnData(header, bytes, heap)
	}
	// General case
	return readArbitraryColumnData(header, bytes, heap)
}

func areAllBits(bytes []byte) bool {
	for _, b := range bytes {
		if b > 1 {
			return false
		}
	}
	//
	return true
}

func readBitColumnData(header legacyHeader, bytes []byte) array.MutArray[word.BigEndian] {
	arr := array.NewBitArray[word.BigEndian](header.length)
	//
	for i := uint(0); i < header.length; i++ {
		ith := bytes[i]
		arr.SetRaw(i, ith > 0)
	}
	// Done
	return &arr
}

func readByteColumnData(length uint, bytes []byte, start, stride uint) array.MutArray[word.BigEndian] {
	//
	var (
		arr    = array.NewSmallArray[uint8, word.BigEndian](length, 8)
		offset = start
	)
	//
	for i := uint(0); i < length; i++ {
		ith := bytes[offset]
		arr.SetRaw(i, ith)
		//
		offset += stride
	}
	// Done
	return &arr
}

func readWordColumnData(header legacyHeader, bytes []byte) array.MutArray[word.BigEndian] {
	var (
		arr    = array.NewSmallArray[uint16, word.BigEndian](header.length, header.width*8)
		offset = uint(0)
		mx     uint16
		zero   word.BigEndian
	)
	// Assign elements
	for i := uint(0); i < header.length; i++ {
		var (
			b1  = uint16(bytes[offset])
			b0  = uint16(bytes[offset+1])
			ith = (b1 << 8) | b0
		)
		// Construct ith element
		arr.SetRaw(i, ith)
		// Move offset to next element
		offset += 2
		mx = max(mx, ith)
	}
	//
	switch {
	case mx == 0:
		return array.NewConstantArray[word.BigEndian](header.length, 0, zero)
	case mx < 256:
		return readByteColumnData(header.length, bytes, 1, 2)
	}
	// Done
	return &arr
}

func readDWordColumnData(header legacyHeader, bytes []byte) array.MutArray[word.BigEndian] {
	var (
		arr    = array.NewSmallArray[uint32, word.BigEndian](header.length, header.width*8)
		offset = uint(0)
	)
	// Assign elements
	for i := uint(0); i < header.length; i++ {
		var (
			b3 = uint32(bytes[offset])
			b2 = uint32(bytes[offset+1])
			b1 = uint32(bytes[offset+2])
			b0 = uint32(bytes[offset+3])
		)
		// Construct ith element
		arr.SetRaw(i, (b3<<24)|(b2<<16)|(b1<<8)|b0)
		// Move offset to next element
		offset += 4
	}
	// Done
	return &arr
}

func readQWordColumnData(header legacyHeader, bytes []byte, builder ArrayBuilder) array.MutArray[word.BigEndian] {
	var (
		arr    = builder.NewArray(header.length, header.width*8)
		offset = uint(0)
	)
	// Assign elements
	for i := uint(0); i < header.length; i++ {
		// Construct ith element
		arr = arr.Set(i, word.NewBigEndian(bytes[offset:offset+8]))
		// Move offset to next element
		offset += 8
	}
	// Done
	return arr
}

// Read column data which is has arbitrary width
func readArbitraryColumnData(header legacyHeader, bytes []byte, builder ArrayBuilder,
) array.MutArray[word.BigEndian] {
	var (
		arr    = builder.NewArray(header.length, header.width*8)
		offset = uint(0)
	)
	// Assign elements
	for i := uint(0); i < header.length; i++ {
		// Calculate position of next element
		next := offset + header.width
		// Construct ith element
		arr = arr.Set(i, word.NewBigEndian(bytes[offset:next]))
		// Move offset to next element
		offset = next
	}
	// Done
	return arr
}

// SplitQualifiedColumnName splits a qualified column name into its module and
// column components.
func splitQualifiedColumnName(name string) (string, string) {
	i := strings.Index(name, ".")
	if i >= 0 {
		// Split on "."
		return name[0:i], name[i+1:]
	}
	// No module name given, therefore its in the prelude.
	return "", name
}
