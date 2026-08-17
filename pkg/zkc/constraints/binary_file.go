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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	tracer "github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// BINFILE_MAJOR_VERSION is the major version of the binary file format.
// Regardless of version, the file always begins with the ZKBINARY identifier
// followed by a hand-rolled binary Header.  The encoding of everything after
// the header is determined by the major version.
const BINFILE_MAJOR_VERSION uint16 = 0

// BINFILE_MINOR_VERSION is the minor version of the binary file format.  Files
// with a lower minor version remain readable by this implementation, but files
// produced by this implementation may not be readable by older versions.
//
// History: v0.1 added the per-memory timestamp width (issue #2069); files
// predating it decode with the then-implicit width of 32.
const BINFILE_MINOR_VERSION uint16 = 1

// ZKC_EXEC is used as the file identifier for binary file types.  This just
// helps us identify actual binary files from corrupted files.
var ZKC_EXEC [8]byte = [8]byte{'z', 'k', 'c', ' ', 'e', 'x', 'e', 'c'}

// BinaryFile provides two pieces of functionality: (i) a means for serialising
// and deserialising a set of AIR constraints; (ii) a means for generating a
// trace for those constraints from a given set of inputs.  Thus, we can write a
// set of constraints to a binary file (e.g. on disk) which can then be read
// back and used to generate a zero-knowledge proof.
//
// Only the raw program (along with the header and attributes) is serialised;
// everything derived from it (the tracing / execution programs and the MIR /
// AIR constraints) is compiled on demand and memoised in the caches below.
//
// NOTE: a BinaryFile is *not* safe for concurrent use.  Since the derived
// artifacts are computed lazily, even apparently read-only methods (e.g.
// AirConstraints, Check, Execute, Trace) mutate the caches.  Callers sharing a
// BinaryFile across goroutines must therefore either provide their own
// synchronisation, or force every artifact they need before handing it out.
type BinaryFile[F field.Element[F]] struct {
	// Header holds the magic identifier, version numbers, and optional JSON
	// metadata for the file.
	header Header
	// Attributes carry supplementary information that is not required for
	// constraint checking but may be useful for tooling (e.g. source-column
	// mappings for debug output).
	attributes []Attribute
	// Program is the high-level representation of the "constraints".
	program vm.Program[vm.Uint]
	// Ignores identifies pipeline stages which should be explicitly ignored
	// when compiling downstream components (e.g. program for tracing, etc).
	// Observe this is a property of how the artifacts below are compiled,
	// rather than of the file itself, and is therefore not serialised.
	ignores []string
	// cache (fast mode) execution program
	cachedExecutionProgram util.Option[vm.Program[vm.Uint128]]
	// cache tracing program
	cachedTracingProgram util.Option[vm.Program[vm.Uint32]]
	// cached mir constraints
	cachedMirConstraints util.Option[mir.Schema[F]]
	// cached air constraints
	cachedAirConstraints util.Option[air.Schema[F]]
}

// NewBinaryFile constructs a BinaryFile, for the given (raw) program, with a
// header stamped at the current major/minor version.  Metadata is an optional
// JSON blob stored verbatim in the header (pass nil for none), whilst
// attributes carry supplementary information for tooling (again, pass nil for
// none).  The field type parameter F determines the field against which the
// program's field configuration is checked on deserialisation, and for which
// constraints are subsequently generated.  No pipeline stages are ignored when
// compiling the derived artifacts --- see WithIgnores for that.
func NewBinaryFile[F field.Element[F]](metadata []byte, attributes []Attribute,
	program vm.Program[vm.Uint]) *BinaryFile[F] {
	//
	return &BinaryFile[F]{
		header:                 Header{ZKC_EXEC, BINFILE_MAJOR_VERSION, BINFILE_MINOR_VERSION, metadata},
		attributes:             attributes,
		program:                program,
		ignores:                nil,
		cachedExecutionProgram: util.None[vm.Program[vm.Uint128]](),
		cachedTracingProgram:   util.None[vm.Program[vm.Uint32]](),
		cachedMirConstraints:   util.None[mir.Schema[F]](),
		cachedAirConstraints:   util.None[air.Schema[F]](),
	}
}

// WithIgnores configures the set of pipeline stages to ignore when compiling
// the artifacts derived from the raw program (i.e. the tracing / execution
// programs and the MIR / AIR constraints).  Since this changes how those
// artifacts are compiled, anything already cached against the previous set of
// ignores is discarded.  This returns the receiver, allowing it to be chained
// onto NewBinaryFile.
func (p *BinaryFile[F]) WithIgnores(ignores ...string) *BinaryFile[F] {
	p.ignores = ignores
	// Discard artifacts compiled under the previous set of ignores.
	p.clearCachedArtifacts()
	//
	return p
}

// Attributes returns the set of attributes embedded in this binary file.
func (p *BinaryFile[F]) Attributes() []Attribute {
	return p.attributes
}

// Header returns the binary file header, which contains the file version and
// optional metadata.
func (p *BinaryFile[F]) Header() Header {
	return p.header
}

// LimbsMap provides a mapping from top-level registers to register limbs.  This
// is useful to understand the mapping before / after register splitting.
func (p *BinaryFile[F]) LimbsMap() module.LimbsMap {
	return newLimbsMap(p.program.Field(), p.program.Modules()...)
}

// Field returns the field configuration for which this binary file is compiled.
// The primary purpose of this is to allow sanity check that the fields match
// between the client and what is embedded in this file.
func (p *BinaryFile[F]) Field() field.Config {
	return p.program.Field()
}

// MaxStaticHeight records the maximum height (i.e. number of rows) of static
// tables used when this binary was compiled.  It must be carried in the file so
// that constraints regenerated from it match those produced at compile time
// (the range constraints baked into the machine depend on this value).
func (p *BinaryFile[F]) MaxStaticHeight() uint {
	return p.program.MaxStaticHeight()
}

// RawProgram returns the raw bytecode program encoded in this file, which has
// not been lowered or subject to register splitting.  As such, it is not a
// suitable form for most use cases.
func (p *BinaryFile[F]) RawProgram() vm.Program[vm.Uint] {
	return p.program
}

// TracingProgram returns the program used for tracing.  This corresponds to the
// raw program after lowering and register splitting as appropriate for tracing.
// This will be constructed by compiling it from the raw program upon first
// call, and subsequently cached.
//
// NOTE: this mutates the cache and is therefore not safe for concurrent use.
func (p *BinaryFile[F]) TracingProgram() vm.Program[vm.Uint32] {
	// Check cache
	if !p.cachedTracingProgram.HasValue() {
		var (
			stats = util.NewPerfStats()
			// Lower bytecode program
			program = vm.TransformForTracing[vm.Uint, vm.Uint32](p.program, p.ignores...)
		)
		// Cache lowered program
		p.cachedTracingProgram = util.Some(program)
		// Log stats
		stats.Log("Compiling tracing program")
	}
	//
	return p.cachedTracingProgram.Unwrap()
}

// ExecutionProgram returns the program used for (fast mode) execution.  This
// corresponds to the raw program after lowering and register splitting as
// appropriate for fast-mode execution.  This will be constructed by compiling
// it from the raw program upon first call, and subsequently cached.
//
// NOTE: this mutates the cache and is therefore not safe for concurrent use.
func (p *BinaryFile[F]) ExecutionProgram() vm.Program[vm.Uint128] {
	// Check cache
	if !p.cachedExecutionProgram.HasValue() {
		var (
			stats = util.NewPerfStats()
			// Lower bytecode program
			program = vm.TransformForExecution[vm.Uint, vm.Uint128](p.program, p.ignores...)
		)
		// Cache lowered program
		p.cachedExecutionProgram = util.Some(program)
		// Log stats
		stats.Log("Compiling (fast mode) execution program")
	}
	//
	return p.cachedExecutionProgram.Unwrap()
}

// MirConstraints returns the mid-level (MIR) constraints generated from the
// program encoded in this file. The constraints are constructed by compiling
// them from the tracing program upon first call, and subsequently cached.
// Observe that the MIR constraints should only be used for debugging purposes,
// as they are not intended for sound constraint checking.
//
// NOTE: this mutates the cache and is therefore not safe for concurrent use.
func (p *BinaryFile[F]) MirConstraints() mir.Schema[F] {
	// Check cache
	if !p.cachedMirConstraints.HasValue() {
		var (
			stats  = util.NewPerfStats()
			tracer = p.TracingProgram()
		)
		// Generate + cache mid-level intermediate representation
		p.cachedMirConstraints = util.Some(GenerateMirConstraints[vm.Uint32, F](tracer))
		// Log stats
		stats.Log("Compiling MIR constraints")
	}
	//
	return p.cachedMirConstraints.Unwrap()
}

// AirConstraints returns the arithmetic (AIR) constraints encoded in this file.
// The constraints are constructed by compiling them from the tracing program
// upon first call, and subsequently cached.
//
// NOTE: this mutates the cache and is therefore not safe for concurrent use.
func (p *BinaryFile[F]) AirConstraints() air.Schema[F] {
	// Check cache
	if !p.cachedAirConstraints.HasValue() {
		var (
			stats  = util.NewPerfStats()
			tracer = p.TracingProgram()
		)
		// Generate + cache arithmetic intermediate representation
		p.cachedAirConstraints = util.Some(GenerateAirConstraints[vm.Uint32, F](tracer))
		// Log stats
		stats.Log("Compiling AIR constraints")
	}
	//
	return p.cachedAirConstraints.Unwrap()
}

// clearCachedArtifacts discards every artifact derived from the raw program,
// such that each is recompiled on the next call to its accessor.  This is
// necessary whenever something they were compiled against changes (i.e. the
// program itself, or the set of ignored pipeline stages).
func (p *BinaryFile[F]) clearCachedArtifacts() {
	p.cachedExecutionProgram = util.None[vm.Program[vm.Uint128]]()
	p.cachedTracingProgram = util.None[vm.Program[vm.Uint32]]()
	p.cachedMirConstraints = util.None[mir.Schema[F]]()
	p.cachedAirConstraints = util.None[air.Schema[F]]()
}

// Check a given trace against the AIR constraints embodied in this constraints
// file, potentially producing one (or more) constraint failures.
func (p *BinaryFile[F]) Check(tr trace.Trace[F], config TraceConfig) []schema.Failure {
	var (
		sc    = p.AirConstraints()
		stats = util.NewPerfStats()
	)
	// Check constraints
	failures := schema.Accepts(config.Parallelism(), sc, tr)
	// Log stats
	stats.Log("Constraint checking")
	//
	return failures
}

// Execute executes the program embodied by these constraints in chunks of n
// steps at a time, producing any outputs arising.  Execution is faster than
// trace because it does not record any internal information about the trace ---
// it simply extracts the outputs at the end.
func (p *BinaryFile[F]) Execute(input map[string][]byte) (output map[string][]byte, errs []error) {
	var (
		bci = vm.NewBytecodeInterpreter(p.ExecutionProgram())
	)
	// Boot and execute fast machine
	output, _, errs = vm.BootAndExecute(bci, input, math.MaxUint)
	//
	return output, errs
}

// Trace generates a suitable trace from the given inputs for the contraints
// embodied in this file.  Inputs are given as byte arrays which are decoded via
// vm.DecodeInputs() based on the register types of the corresponding memory.
// This can return one (or more) errors if, for example, the input is malformed
// (e.g. is missing expected fields and/or contains unexpected fields).
// The raw (row-major) trace produced by the machine is also returned, since it
// carries the original register / limb structure before AIR expansion (e.g. for
// reporting statistics).  It is nil when execution fails.
func (p *BinaryFile[F]) Trace(input map[string][]byte, cfg TraceConfig,
) (output map[string][]byte, rtr rtrace.Trace[F], tr trace.Trace[F], errs []error) {
	//
	var (
		stats = util.NewPerfStats()
		// Lower bytecode program
		prog32 = p.TracingProgram()
		//
		builder = tracer.NewBuilder[vm.Uint32, F, *rtrace.CompactModule[F]](prog32)
	)
	// Execute machine in chunks of 1K steps
	rtr, output, errs = vm.BootAndTrace(prog32, input, math.MaxUint, builder)
	//
	if rtr != nil {
		var berrs []error
		// Extract AIR constraints
		constraints := p.AirConstraints()
		// Construct trace builder
		builder := ir.NewTraceBuilder[F]().
			// NOTE: never use validation, as it hides constraint failures.
			WithValidation(false).
			WithDefensivePadding(false).
			WithExpansionChecks(true).
			WithExpansion(true).
			WithParallelism(cfg.parallel).
			WithBatchSize(cfg.batchSize).
			WithPadding(cfg.paddingStrategy)
		// Build the trace (finally)
		tr, berrs = builder.Expand(constraints, rtrace.ToTrace(rtr))
		// Include any builder errors
		errs = append(errs, berrs...)
	}
	//
	stats.Log("Trace generation")
	//
	return output, rtr, tr, errs
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
	// Encode schema
	if err := gobEncoder.Encode(&p.program); err != nil {
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
			if err = decoder.Decode(&p.program); err == nil {
				// Discard the ignores configured against the previous program,
				// since they are a property of how that program was to be
				// compiled (see WithIgnores).
				p.ignores = nil
				// Discard anything cached against the previous program, since
				// this file now describes a different one.
				p.clearCachedArtifacts()
				// extract modulus defined used for the compiling the given
				// constraints.
				var mod = p.program.Field().Modulus()
				// check for compatible field
				if modulus.Cmp(mod) != 0 {
					err = fmt.Errorf("incompatible prime field (0x%s versus 0x%s))", modulus.Text(16), mod.Text(16))
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
