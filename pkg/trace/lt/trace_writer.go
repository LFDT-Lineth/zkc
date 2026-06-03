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
	"io"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// WordArrayBuilder provides a convenient aliasO
type WordArrayBuilder = array.DynamicBuilder[word.BigEndian, *WordHeap]

// ToBytes writes a given trace file as an array of bytes.  See FromBytes for
// more information on the layout of data in this format.
func ToBytes(heap WordHeap, rawModules []Module[word.BigEndian]) ([]byte, error) {
	var (
		buf         bytes.Buffer
		err         error
		builder     = array.NewDynamicBuilder(&heap)
		headerBytes []byte
		heapBytes   []byte
	)
	// For now we do an ugly split
	moduleEncodings := splitRawColumns(rawModules, builder)
	// Construct header data
	if headerBytes, err = toHeaderBytes(rawModules, moduleEncodings); err != nil {
		return nil, err
	}
	// Construct heap data
	if heapBytes, err = heap.MarshalBinaryV3(); err != nil {
		return nil, err
	}
	// Write header size
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(headerBytes))); err != nil {
		return nil, err
	}
	// Write heap size
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(heapBytes))); err != nil {
		return nil, err
	}
	// Write header bytes
	if n, err := buf.Write(headerBytes); err != nil {
		return nil, err
	} else if n != len(headerBytes) {
		return nil, fmt.Errorf("wrote insufficient header bytes (%d v %d)", n, len(headerBytes))
	}
	// Write heap bytes
	if n, err := buf.Write(heapBytes); err != nil {
		return nil, err
	} else if n != len(heapBytes) {
		return nil, fmt.Errorf("wrote insufficient heap bytes (%d v %d)", n, len(heapBytes))
	}
	// Write column data
	for _, columnEncodings := range moduleEncodings {
		for _, encoding := range columnEncodings {
			if n, err := buf.Write(encoding.Bytes); err != nil {
				return nil, err
			} else if n != len(encoding.Bytes) {
				return nil, fmt.Errorf("wrote insufficient encoded column bytes (%d v %d)", n, len(encoding.Bytes))
			}
		}
	}
	//
	return buf.Bytes(), nil
}

func splitRawColumns(rawModules []Module[word.BigEndian], builder WordArrayBuilder) [][]array.Encoding {
	var encodings = make([][]array.Encoding, len(rawModules))
	//
	for i, ith := range rawModules {
		var ithEncodings = make([]array.Encoding, len(ith.Columns))
		//
		for j, jth := range ith.Columns {
			ithEncodings[j] = builder.Encode(jth.data)
		}
		//
		encodings[i] = ithEncodings
	}
	//
	return encodings
}

func toHeaderBytes(modules []Module[word.BigEndian], encodings [][]array.Encoding) ([]byte, error) {
	var buf bytes.Buffer
	// Write number of modules
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(modules))); err != nil {
		return nil, err
	}
	//
	for i, module := range modules {
		if err := writeModuleHeader(&buf, module, encodings[i]); err != nil {
			return nil, err
		}
	}
	//
	return buf.Bytes(), nil
}

func writeModuleHeader(buf io.Writer, module Module[word.BigEndian], encodings []array.Encoding) (err error) {
	var height = uint32(module.Height())
	// Write module name
	if err = writeName(buf, module.Name().String()); err != nil {
		return err
	}
	// Write module height
	if err := binary.Write(buf, binary.BigEndian, height); err != nil {
		return err
	}
	// Write column count
	if err := binary.Write(buf, binary.BigEndian, uint32(len(module.Columns))); err != nil {
		return err
	}
	// Write column info
	for i, col := range module.Columns {
		if err = writeColumnHeader(buf, col, encodings[i]); err != nil {
			return err
		}
	}
	//
	return nil
}

func writeColumnHeader(buf io.Writer, column Column[word.BigEndian], encoding array.Encoding) (err error) {
	var (
		bitwidth uint16 = uint16(column.data.BitWidth())
		len      uint32 = uint32(len(encoding.Bytes))
	)
	// Write column name
	if err = writeName(buf, column.name); err != nil {
		return err
	}
	// Write column data length
	if err := binary.Write(buf, binary.BigEndian, len); err != nil {
		return err
	}
	// Write column data encoding schema
	if err := binary.Write(buf, binary.BigEndian, encoding.Encoding); err != nil {
		return err
	}
	// Write column bitwidth
	if err := binary.Write(buf, binary.BigEndian, bitwidth); err != nil {
		return err
	}
	// Done
	return nil
}

func writeName(buf io.Writer, name string) (err error) {
	nameBytes := []byte(name)
	nameLen := uint16(len(nameBytes))

	if err := binary.Write(buf, binary.BigEndian, nameLen); err != nil {
		return err
	}
	// Write name bytes
	n, err := buf.Write(nameBytes)
	if n != int(nameLen) {
		return fmt.Errorf("incorrect name bytes written (%d v %d)", nameLen, n)
	}
	//
	return err
}
