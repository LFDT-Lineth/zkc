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
	"io"
	"log"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// ToBytesLegacy writes a given trace file as an array of (legacy) bytes.  The
// output represents the legacy format if the bytes are used "as is" without any
// additional header information being preprended.
func ToBytesLegacy(modules []Module[word.BigEndian]) ([]byte, error) {
	buf, err := ToBytesBuffer(modules)
	if err != nil {
		return nil, err
	}
	//
	return buf.Bytes(), err
}

// ToBytesBuffer writes a given trace file into a byte buffer.
func ToBytesBuffer(modules []Module[word.BigEndian]) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	if err := WriteBytes(modules, &buf); err != nil {
		return nil, err
	}

	return &buf, nil
}

// WriteBytes a given trace file to an io.Writer.
func WriteBytes(modules []Module[word.BigEndian], buf io.Writer) error {
	ncols := NumberOfColumns(modules)
	// Write column count
	if err := binary.Write(buf, binary.BigEndian, uint32(ncols)); err != nil {
		return err
	}
	// Write header information
	for _, ith := range modules {
		for _, jth := range ith.Columns {
			data := jth.data
			name := trace.QualifiedColumnName(ith.Name(), jth.name)
			// Write name length
			nameBytes := []byte(name)
			nameLen := uint16(len(nameBytes))

			if err := binary.Write(buf, binary.BigEndian, nameLen); err != nil {
				return err
			}
			// Write name bytes
			n, err := buf.Write(nameBytes)
			if n != int(nameLen) || err != nil {
				log.Fatal(err)
			}
			// Determine number of bytes required to hold element of this column.
			byteWidth := data.BitWidth() / 8
			if data.BitWidth()%8 != 0 {
				byteWidth++
			}
			// Write bytes per element
			if err := binary.Write(buf, binary.BigEndian, uint8(byteWidth)); err != nil {
				log.Fatal(err)
			}
			// Write Data length
			if err := binary.Write(buf, binary.BigEndian, uint32(data.Len())); err != nil {
				log.Fatal(err)
			}
		}
	}
	// Write column data information
	for _, ith := range modules {
		for _, jth := range ith.Columns {
			if err := writeArrayBytes(buf, jth.data, jth.data.BitWidth()); err != nil {
				return err
			}
		}
	}
	// Done
	return nil
}

func writeArrayBytes(w io.Writer, data array.Array[word.BigEndian], bitwidth uint) error {
	var (
		bytewidth = word.ByteWidth(bitwidth)
		padding   = make([]byte, bytewidth)
	)
	//
	for i := range data.Len() {
		var (
			ith   = data.Get(i)
			bytes = ith.Bytes()
			// Determine padding bytes required
			n = bytewidth - uint(len(bytes))
		)
		// Write most significant (i.e. padding) bytes
		if _, err := w.Write(padding[0:n]); err != nil {
			return err
		}
		// Write least significant (i.e. content) bytes
		if _, err := w.Write(bytes); err != nil {
			return err
		}
	}
	//
	return nil
}
