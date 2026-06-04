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
	"errors"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
)

// Header provides a structured header for the binary file format.  In
// particular, it supports versioning and embedded (binary) metadata.
type Header struct {
	Identifier   [8]byte
	MajorVersion uint16
	MinorVersion uint16
	MetaData     []byte
}

// GetMetaData attempts to parse the metadata bytes as JSON which is then
// unmarshalled into a map.  This can fail if the embedded metadata bytes are
// not, in fact, JSON.  Observe that, if there are no metadata bytes, then nil
// will be returned.
func (p *Header) GetMetaData() (typed.Map, error) {
	// Check for empty metadata
	if len(p.MetaData) == 0 {
		return typed.NewMap(nil), nil
	}
	// Attempt to unmarshal metadata bytes
	return typed.FromJsonBytes(p.MetaData)
}

// SetMetaData attempts to set the metadata bytes for this header, using a JSON
// encoding of the given map.  If this fails, an error is returned and the
// metadata bytes are unaffected.
func (p *Header) SetMetaData(metadata typed.Map) error {
	bytes, err := metadata.ToJsonBytes()
	// Check for error
	if err != nil {
		return err
	}
	// success
	p.MetaData = bytes
	//
	return nil
}

// MarshalBinary converts the LT file header into a sequence of bytes. Observe
// that we don't use GobEncoding here to avoid being tied to that encoding
// scheme.
func (p *Header) MarshalBinary() ([]byte, error) {
	var (
		buffer     bytes.Buffer
		majorBytes [2]byte
		minorBytes [2]byte
		metaLength [4]byte
	)
	// Marshall version numbers
	binary.BigEndian.PutUint16(majorBytes[:], p.MajorVersion)
	binary.BigEndian.PutUint16(minorBytes[:], p.MinorVersion)
	binary.BigEndian.PutUint32(metaLength[:], uint32(len(p.MetaData)))
	// Write identifier
	buffer.Write(p.Identifier[:])
	// Write major version
	buffer.Write(majorBytes[:])
	// Write minor version
	buffer.Write(minorBytes[:])
	// Write metadata length
	buffer.Write(metaLength[:])
	// Write metadata itself
	buffer.Write(p.MetaData)
	// Done
	return buffer.Bytes(), nil
}

// UnmarshalBinary initialises this LT file header from a given set of data
// bytes. This should match exactly the encoding above.
func (p *Header) UnmarshalBinary(buffer *bytes.Buffer) error {
	var (
		majorBytes      [2]byte
		minorBytes      [2]byte
		metaLengthBytes [4]byte
	)
	// Read identifier
	if n, err := buffer.Read(p.Identifier[:]); err != nil {
		return err
	} else if n != 8 {
		return errors.New("malformed trace file")
	}
	// Read major version
	if n, err := buffer.Read(majorBytes[:]); err != nil {
		return err
	} else if n != len(majorBytes) {
		return errors.New("malformed trace file")
	}
	// Read minor version
	if n, err := buffer.Read(minorBytes[:]); err != nil {
		return err
	} else if n != len(minorBytes) {
		return errors.New("malformed trace file")
	}
	// Read metadata length
	if n, err := buffer.Read(metaLengthBytes[:]); err != nil {
		return err
	} else if n != len(metaLengthBytes) {
		return errors.New("malformed trace file")
	}
	// Make space for the metadata
	var (
		metaLength = binary.BigEndian.Uint32(metaLengthBytes[:])
		metaBytes  = make([]byte, metaLength)
	)
	// Read metadata itself
	if n, err := buffer.Read(metaBytes[:]); err != nil {
		return err
	} else if n != len(metaBytes) {
		return errors.New("malformed trace file")
	}
	// Finally assign everything over
	p.MajorVersion = binary.BigEndian.Uint16(majorBytes[:])
	p.MinorVersion = binary.BigEndian.Uint16(minorBytes[:])
	p.MetaData = metaBytes
	// Done
	return nil
}

// IsCompatible determines whether a given binary file is compatible with this
// version of go-corset.
func (p *Header) IsCompatible() bool {
	//
	return p.Identifier == ZKTRACER &&
		((p.MajorVersion == LTV1_MAJOR_VERSION) ||
			(p.MajorVersion == LTV2_MAJOR_VERSION) ||
			(p.MajorVersion == LTV3_MAJOR_VERSION && p.MinorVersion <= LTV3_MINOR_VERSION))
}
