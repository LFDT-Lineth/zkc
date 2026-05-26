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
package constraints

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/consensys/go-corset/pkg/ir/air"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/collection/typed"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm"
)

// BINFILE_MAJOR_VERSION is the major version of the binary file format.
// Regardless of version, the file always begins with the ZKBINARY identifier
// followed by a hand-rolled binary Header.  The encoding of everything after
// the header is determined by the major version.
const BINFILE_MAJOR_VERSION uint16 = 0

// BINFILE_MINOR_VERSION is the minor version of the binary file format.  Files
// with a lower minor version remain readable by this implementation, but files
// produced by this implementation may not be readable by older versions.
const BINFILE_MINOR_VERSION uint16 = 0

// ZKC_EXEC is used as the file identifier for binary file types.  This just
// helps us identify actual binary files from corrupted files.
var ZKC_EXEC [8]byte = [8]byte{'z', 'k', 'c', ' ', 'e', 'x', 'e', 'c'}

// BinaryFile provides two pieces of functionality: (i) a means for serialising
// and deserialising a set of AIR constraints; (ii) a means for generating a
// trace for those constraints from a given set of inputs.  Thus, we can write a
// set of constraints to a binary file (e.g. on disk) which can then be read
// back and used to generate a zero-knowledge proof.
type BinaryFile[F field.Element[F]] struct {
	// Header holds the magic identifier, version numbers, and optional JSON
	// metadata for the file.
	header Header
	// Attributes carry supplementary information that is not required for
	// constraint checking but may be useful for tooling (e.g. source-column
	// mappings for debug output).
	attributes []Attribute
	// Config identifies the field configuration supported by this binary file.
	// This is important for e.g. checking the binary file is compiled for the
	// right field, etc.
	config field.Config
	// Machine is the current representation of "constraints".  Its not very
	// pretty, but right now its all we have.  This will certainly change in the
	// near future.
	machine vm.WordMachine[vm.Uint]
	// cached air constraints
	cache util.Option[air.Schema[F]]
}

// NewBinaryFile constructs a BinaryFile with a header stamped at the current
// major/minor version.  Metadata is an optional JSON blob stored verbatim in
// the header (pass nil for none).  A field configuration is required to allow
// clients to check they are targeting the expected field.
func NewBinaryFile[F field.Element[F]](metadata []byte, attributes []Attribute, config field.Config,
	machine vm.WordMachine[vm.Uint]) *BinaryFile[F] {
	//
	return &BinaryFile[F]{
		Header{ZKC_EXEC, BINFILE_MAJOR_VERSION, BINFILE_MINOR_VERSION, metadata},
		attributes,
		config,
		machine,
		util.None[air.Schema[F]](),
	}
}

// Header returns the binary file header, which contains the file version and
// optional metadata.
func (p *BinaryFile[F]) Header() Header {
	return p.header
}

// LimbsMap provides a mapping from top-level registers to register limbs.  This
// is useful to understand the mapping before / after register splitting.
func (p *BinaryFile[F]) LimbsMap() module.LimbsMap {
	return newLimbsMap(p.config, p.machine.Modules()...)
}

// Field returns the field configuration for which this binary file is compiled.
// The primary purpose of this is to allow sanity check that the fields match
// between the client and what is embedded in this file.
func (p *BinaryFile[F]) Field() field.Config {
	return p.config
}

// AirConstraints returns the arithmetic (AIR) constraints encoded in this file.
func (p *BinaryFile[F]) AirConstraints() air.Schema[F] {
	// Check cache
	if p.cache.HasValue() {
		return p.cache.Unwrap()
	}
	//
	var (
		stats = util.NewPerfStats()
		// Lower from word-level machine to field-level machine
		fir = vm.LowerWordMachine[vm.Uint, F](p.config, &p.machine)
		// Generate arithmetic intermediate representation
		air = GenerateAirConstraints(fir, p.Field())
	)
	// cache result
	p.cache = util.Some(air)
	// Log stats
	stats.Log("Constraint compilation")
	//
	return air
}

// WordMachine returns the top-level word machine encoded in this file.
func (p *BinaryFile[F]) WordMachine() vm.WordMachine[vm.Uint] {
	return p.machine
}

// Check a given trace against the constraints embodied in this constraints
// file, potentially producing one (or more) constraint failures.
func (p *BinaryFile[F]) Check(tr trace.Trace[F], config TraceConfig) []schema.Failure {
	var (
		sc    = p.AirConstraints()
		stats = util.NewPerfStats()
	)
	// Check constraints
	failures := schema.Accepts(config.Parallelism(), config.BatchSize(), sc, tr)
	// Log stats
	stats.Log("Constraint checking")
	//
	return failures
}

// Execute executes the program embodied by these constraints in chunks of n
// steps at a time, producing any outputs arising.  Execution is faster than
// trace because it does not record any internal information about the trace ---
// it simply extracts the outputs at the end.
func (p *BinaryFile[F]) Execute(input map[string][]byte, n uint) (output map[string][]byte, errs []error) {
	var (
		steps  uint
		inputs map[string][]vm.Uint
		stats  = util.NewPerfStats()
	)
	// Execute machine in chunks of 1K steps
	if inputs, errs = vm.DecodeInputs(&p.machine, input); len(errs) != 0 {
		return nil, errs
	} else if err := p.machine.Boot("main", inputs); err != nil {
		errs = append(errs, err)
	} else if steps, err = vm.ExecuteAll(&p.machine, n); err != nil {
		errs = append(errs, err)
	} else {
		output = vm.EncodeOutputs(&p.machine)
	}
	// Log stats
	stats.Log(fmt.Sprintf("Constraint execution (%d steps)", steps))
	//
	return output, errs
}

// Trace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  Inputs are given as byte arrays which are decoded via
// vm.DecodeInputs() based on the register types of the corresponding memory.
// This can return one (or more) errors if, for example, the input is malformed
// (e.g. is missing expected fields and/or contains unexpected fields).
func (p *BinaryFile[F]) Trace(input map[string][]byte, config TraceConfig) (tr trace.Trace[F], errs []error) {
	var inputs map[string][]vm.Uint
	// Execute machine in chunks of 1K steps
	if inputs, errs = vm.DecodeInputs(&p.machine, input); len(errs) == 0 {
		return Trace(p, inputs, config)
	}
	//
	return nil, errs
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// MarshalBinary converts the BinaryFile into a sequence of bytes.
func (p *BinaryFile[F]) MarshalBinary() ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder *gob.Encoder = gob.NewEncoder(&buffer)
	)
	// Bytes header
	headerBytes, err := p.header.MarshalBinary()
	//
	if err != nil {
		return nil, err
	}
	// Encode header
	buffer.Write(headerBytes)
	// Encode attributes
	if err := gobEncoder.Encode(p.attributes); err != nil {
		return nil, err
	}
	// Encode field configuration
	if err := gobEncoder.Encode(p.config); err != nil {
		return nil, err
	}
	// Encode schema
	if err := gobEncoder.Encode(&p.machine); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// UnmarshalBinary initialises this BinaryFile from a given set of data bytes.
// This should match exactly the encoding above.
func (p *BinaryFile[F]) UnmarshalBinary(data []byte) error {
	var (
		err     error
		element F
		modulus = element.Modulus()
	)
	//
	buffer := bytes.NewBuffer(data)
	// Read header
	if err = p.header.UnmarshalBinary(buffer); err == nil && p.header.IsCompatible() {
		// Looks good, proceed.
		decoder := gob.NewDecoder(buffer)
		// Proceed to decoding any attributes.
		if err = decoder.Decode(&p.attributes); err == nil {
			if err = decoder.Decode(&p.config); err == nil {
				// extract modulus defined used for the compiling the given
				// constraints.
				var mod = p.config.Modulus()
				// check for compatible field
				if modulus.Cmp(p.config.Modulus()) != 0 {
					err = fmt.Errorf("incompatible prime field (0x%s versus 0x%s))", modulus.Text(16), mod.Text(16))
				} else {
					// finally, decode the constraints
					err = decoder.Decode(&p.machine)
				}
			}
		}
	} else if err == nil {
		err = fmt.Errorf("incompatible binary file was v%d.%d, but expected v%d.%d)",
			p.header.MajorVersion, p.header.MinorVersion, BINFILE_MAJOR_VERSION, BINFILE_MINOR_VERSION)
	}
	//
	return err
}

// ============================================================================

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

// IsBinaryFile checks whether the given data file begins with the expected
// "zkc exec" identifier.
func IsBinaryFile(data []byte) bool {
	var (
		zkc_exec [8]byte
		buffer   *bytes.Buffer = bytes.NewBuffer(data)
	)
	//
	if _, err := buffer.Read(zkc_exec[:]); err != nil {
		return false
	}
	// Check whether header identified
	return zkc_exec == ZKC_EXEC
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
	return p.Identifier == ZKC_EXEC &&
		p.MajorVersion == BINFILE_MAJOR_VERSION &&
		p.MinorVersion <= BINFILE_MINOR_VERSION
}

// ============================================================================

// Attribute is an extension point for storing arbitrary metadata alongside the
// compiled schema.  Typical uses include source-to-column mappings and
// debug/profiling annotations.  Attribute values must be gob-encodable.
type Attribute interface {
	// AttributeName returns the name of this attribute.
	AttributeName() string
}

// GetAttribute returns the first instance of a given attribute, or nil if none
// exists.
func GetAttribute[T Attribute, F field.Element[F]](binf *BinaryFile[F]) (T, bool) {
	var empty T
	//
	for _, attr := range binf.attributes {
		if a, ok := attr.(T); ok {
			return a, true
		}
	}
	//
	return empty, false
}
