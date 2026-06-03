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
package binfile

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
)

// ============================================================================
// Binary File Format
// ============================================================================

// BinaryFile is the in-memory representation of a compiled constraint binary.
// It is produced by the go-corset compiler and consumed by the checker/prover.
// The on-disk layout is: a custom binary Header, followed by a gob-encoded
// attribute list, followed by a gob-encoded MacroHirProgram schema.
type BinaryFile struct {
	// Header holds the magic identifier, version numbers, and optional JSON
	// metadata for the file.
	Header Header
	// Attributes carry supplementary information that is not required for
	// constraint checking but may be useful for tooling (e.g. source-column
	// mappings for debug output).
	Attributes []Attribute
	// Schema is the compiled constraint program, combining macro-level assembly
	// instructions with a HIR constraint schema.
	Schema asm.MacroHirProgram
}

// NewBinaryFile constructs a BinaryFile with a header stamped at the current
// major/minor version.  metadata is an optional JSON blob stored verbatim in
// the header (pass nil for none).
func NewBinaryFile(metadata []byte, attributes []Attribute, schema asm.MacroHirProgram,
) *BinaryFile {
	//
	return &BinaryFile{
		Header{ZKBINARY, BINFILE_MAJOR_VERSION, BINFILE_MINOR_VERSION, metadata},
		attributes,
		schema,
	}
}

// GetAttribute returns the first instance of a given attribute, or nil if none
// exists.
func GetAttribute[T Attribute](binf *BinaryFile) (T, bool) {
	var empty T
	//
	for _, attr := range binf.Attributes {
		if a, ok := attr.(T); ok {
			return a, true
		}
	}
	//
	return empty, false
}

// Attribute is an extension point for storing arbitrary metadata alongside the
// compiled schema.  Typical uses include source-to-column mappings and
// debug/profiling annotations.  Attribute values must be gob-encodable.
type Attribute interface {
	// AttributeName returns the name of this attribute.
	AttributeName() string
}

// Header is the fixed-layout prefix of every binary file.  It is serialised
// using a hand-rolled big-endian encoding (not gob) so that the magic
// identifier and version numbers can be read without a full decode.
type Header struct {
	// Identifier is the 8-byte magic constant "zkbinary" that marks the file type.
	Identifier [8]byte
	// MajorVersion must match BINFILE_MAJOR_VERSION exactly for the file to be
	// considered compatible.
	MajorVersion uint16
	// MinorVersion must be ≤ BINFILE_MINOR_VERSION for the file to be
	// considered compatible (older minor versions remain readable).
	MinorVersion uint16
	// MetaData is an optional JSON blob carrying key/value pairs (e.g. the
	// source file path, compiler version, or build timestamp).
	MetaData []byte
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

// MarshalBinary converts the BinaryFile Header into a sequence of bytes.
// Observe that we don't use GobEncoding here to avoid being tied to that
// encoding scheme.
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

// UnmarshalBinary initialises this BinaryFile Header from a given set of data bytes.
// This should match exactly the encoding above.
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
		return errors.New("malformed binary file")
	}
	// Read major version
	if n, err := buffer.Read(majorBytes[:]); err != nil {
		return err
	} else if n != len(majorBytes) {
		return errors.New("malformed binary file")
	}
	// Read minor version
	if n, err := buffer.Read(minorBytes[:]); err != nil {
		return err
	} else if n != len(minorBytes) {
		return errors.New("malformed binary file")
	}
	// Read metadata length
	if n, err := buffer.Read(metaLengthBytes[:]); err != nil {
		return err
	} else if n != len(metaLengthBytes) {
		return errors.New("malformed binary file")
	}
	// Make space for the metadata
	var (
		metaLength        = binary.BigEndian.Uint32(metaLengthBytes[:])
		metaBytes  []byte = make([]byte, metaLength)
	)
	// Read metadata itself
	if n, err := buffer.Read(metaBytes[:]); err != nil {
		return err
	} else if n != len(metaBytes) {
		return errors.New("malformed binary file")
	}
	// Finally assign everything over
	p.MajorVersion = binary.BigEndian.Uint16(majorBytes[:])
	p.MinorVersion = binary.BigEndian.Uint16(minorBytes[:])
	p.MetaData = metaBytes
	// Done
	return nil
}

// IsCompatible reports whether this header can be decoded by the current
// version of go-corset.  Compatibility requires the "zkbinary" magic
// identifier, an exact match on the major version, and a minor version no
// greater than the current minor version.
func (p *Header) IsCompatible() bool {
	//
	return p.Identifier == ZKBINARY &&
		p.MajorVersion == BINFILE_MAJOR_VERSION &&
		p.MinorVersion <= BINFILE_MINOR_VERSION
}

// BINFILE_MAJOR_VERSION is the major version of the binary file format.
// Regardless of version, the file always begins with the ZKBINARY identifier
// followed by a hand-rolled binary Header.  The encoding of everything after
// the header is determined by the major version.
const BINFILE_MAJOR_VERSION uint16 = 11

// BINFILE_MINOR_VERSION is the minor version of the binary file format.  Files
// with a lower minor version remain readable by this implementation, but files
// produced by this implementation may not be readable by older versions.
const BINFILE_MINOR_VERSION uint16 = 0

// ZKBINARY is used as the file identifier for binary file types.  This just
// helps us identify actual binary files from corrupted files.
var ZKBINARY [8]byte = [8]byte{'z', 'k', 'b', 'i', 'n', 'a', 'r', 'y'}

// IsBinaryFile checks whether the given data file begins with the expected
// "zkbinary" identifier.
func IsBinaryFile(data []byte) bool {
	var (
		zkbinary [8]byte
		buffer   *bytes.Buffer = bytes.NewBuffer(data)
	)
	//
	if _, err := buffer.Read(zkbinary[:]); err != nil {
		return false
	}
	// Check whether header identified
	return zkbinary == ZKBINARY
}

// MarshalBinary converts the BinaryFile into a sequence of bytes.
func (p *BinaryFile) MarshalBinary() ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder *gob.Encoder = gob.NewEncoder(&buffer)
	)
	// Bytes header
	headerBytes, err := p.Header.MarshalBinary()
	//
	if err != nil {
		return nil, err
	}
	// Encode header
	buffer.Write(headerBytes)
	// Encode attributes
	if err := gobEncoder.Encode(p.Attributes); err != nil {
		return nil, err
	}
	// Encode schema
	if err := gobEncoder.Encode(&p.Schema); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// UnmarshalBinary initialises this BinaryFile from a given set of data bytes.
// This should match exactly the encoding above.
func (p *BinaryFile) UnmarshalBinary(data []byte) error {
	var err error
	//
	buffer := bytes.NewBuffer(data)
	// Read header
	if err = p.Header.UnmarshalBinary(buffer); err == nil && p.Header.IsCompatible() {
		// Looks good, proceed.
		decoder := gob.NewDecoder(buffer)
		// Proceed to decoding any attributes.
		if err = decoder.Decode(&p.Attributes); err == nil {
			// Finally, decode the schema itself
			err = decoder.Decode(&p.Schema)
		}
	} else if err == nil {
		err = fmt.Errorf("incompatible binary file was v%d.%d, but expected v%d.%d)",
			p.Header.MajorVersion, p.Header.MinorVersion, BINFILE_MAJOR_VERSION, BINFILE_MINOR_VERSION)
	}
	//
	return err
}
