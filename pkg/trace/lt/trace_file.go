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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// ArrayBuilder provides a usefuil alias
type ArrayBuilder = array.DynamicBuilder[word.BigEndian, *pool.SharedHeap[word.BigEndian]]

// WordHeap provides a usefuil alias
type WordHeap = pool.LocalHeap[word.BigEndian]

// LTV1_MAJOR_VERSION givesn the major version of the (currently supported) legacy
// binary file format.  No matter what version, we should always have the
// ZKBINARY identifier first, followed by a GOB encoding of the header.  What
// follows after that, however, is determined by the major version.
const LTV1_MAJOR_VERSION uint16 = 1

// LTV2_MAJOR_VERSION gives the major version of the binary file format.  No
// matter what version, we should always have the ZKBINARY identifier first,
// followed by a GOB encoding of the header.  What follows after that, however,
// is determined by the major version.
const LTV2_MAJOR_VERSION uint16 = 2

// LTV3_MAJOR_VERSION gives the major version of the binary file format.  No
// matter what version, we should always have the ZKBINARY identifier first,
// followed by a GOB encoding of the header.  What follows after that, however,
// is determined by the major version.
const LTV3_MAJOR_VERSION uint16 = 3

// LTV3_MINOR_VERSION gives the minor version of the binary file format.  The
// expected interpretation is that older versions are compatible with newer
// ones, but not vice-versa.
const LTV3_MINOR_VERSION uint16 = 0

// ZKTRACER is used as the file identifier for binary file types.  This just
// helps us identify actual binary files from corrupted files.
var ZKTRACER [8]byte = [8]byte{'z', 'k', 't', 'r', 'a', 'c', 'e', 'r'}

// NumberOfColumns determines the total number of columns in a given array of
// modules.
func NumberOfColumns[F any](modules []Module[F]) uint {
	var count = uint(0)
	//
	for _, ith := range modules {
		for range ith.Columns {
			count++
		}
	}
	//
	return count
}

// TraceFile is a programatic represresentation of an underlying trace file.
type TraceFile struct {
	// Header for the binary file
	header Header
	// Word pool
	heap WordHeap
	// Column data
	modules []Module[word.BigEndian]
}

// NewTraceFile constructs a new trace file with the default header for the
// current default version.
func NewTraceFile(metadata []byte, pool WordHeap, modules []Module[word.BigEndian]) TraceFile {
	return TraceFile{
		Header{ZKTRACER, LTV3_MAJOR_VERSION, LTV3_MINOR_VERSION, metadata},
		pool,
		modules,
	}
}

// NewTraceFileV1 constructs a new legacy trace file with the default header for
// the current default version.
func NewTraceFileV1(metadata []byte, pool WordHeap, modules []Module[word.BigEndian]) TraceFile {
	return TraceFile{
		Header{ZKTRACER, LTV1_MAJOR_VERSION, LTV3_MINOR_VERSION, metadata},
		pool,
		modules,
	}
}

// Builder implementation for trace.Trace interface.
func (p *TraceFile) Builder() array.Builder[word.BigEndian] {
	panic("unsupported operation")
}

// Column implementation for trace.Trace interface.
func (p *TraceFile) Column(ref tr.ColumnRef) tr.Column[word.BigEndian] {
	return p.modules[ref.Module()].Column(ref.Column().Unwrap())
}

// HasModule implementation for trace interface.
func (p *TraceFile) HasModule(name tr.ModuleName) (uint, bool) {
	// Linea scan through list of modules
	for mid, mod := range p.modules {
		if mod.name == name {
			return uint(mid), true
		}
	}
	//
	return math.MaxUint, false
}

// Header returns the trace file header
func (p *TraceFile) Header() Header {
	return p.header
}

// Heap returns the trace file heap
func (p *TraceFile) Heap() WordHeap {
	return p.heap
}

// RawModules provides direct access to the underlying modules
func (p *TraceFile) RawModules() []Module[word.BigEndian] {
	return p.modules
}

// Clone a trace file producing an unaliased copy
func (p *TraceFile) Clone() TraceFile {
	var modules = make([]Module[word.BigEndian], len(p.modules))
	//
	for i, mod := range p.modules {
		var columns = make([]Column[word.BigEndian], len(mod.Columns))
		//
		for j, col := range mod.Columns {
			var data = col.data
			// Clone data (if it exists)
			if data != nil {
				data = data.Clone()
			}
			// Clone colunm data
			columns[j] = Column[word.BigEndian]{
				col.name,
				data,
			}
		}
		//
		modules[i] = Module[word.BigEndian]{mod.Name(), columns}
	}
	//
	return TraceFile{
		p.header,
		p.heap.Clone(),
		modules,
	}
}

// IsTraceFile checks whether the given data file begins with the expected
// "zktracer" identifier.
func IsTraceFile(data []byte) bool {
	var (
		zktracer [8]byte
		buffer   = bytes.NewBuffer(data)
	)
	//
	if _, err := buffer.Read(zktracer[:]); err != nil {
		return false
	}
	// Check whether header identified
	return zktracer == ZKTRACER
}

// MarshalBinary converts the TraceFile into a sequence of bytes.
func (p *TraceFile) MarshalBinary() ([]byte, error) {
	var (
		buffer      bytes.Buffer
		columnBytes []byte
		err         error
	)
	// Bytes header
	headerBytes, err := p.header.MarshalBinary()
	// Error check
	if err != nil {
		return nil, err
	}
	// Encode header
	buffer.Write(headerBytes)
	// Write column data
	switch p.header.MajorVersion {
	case LTV1_MAJOR_VERSION:
		columnBytes, err = ToBytesLegacy(p.modules)
	case LTV3_MAJOR_VERSION:
		columnBytes, err = ToBytes(p.heap, p.modules)
	default:
		err = fmt.Errorf("unknown lt major file format %d", p.header.MajorVersion)
	}
	// Error check
	if err != nil {
		return nil, err
	}
	// Encode column data
	buffer.Write(columnBytes)
	// Done
	return buffer.Bytes(), nil
}

// Module implementation for trace.Trace interface
func (p *TraceFile) Module(mid tr.ModuleId) tr.Module[word.BigEndian] {
	return &p.modules[mid]
}

// Modules implementation for trace.Trace interface
func (p *TraceFile) Modules() iter.Iterator[tr.Module[word.BigEndian]] {
	panic("unsupported operation")
}

// UnmarshalBinary initialises this TraceFile from a given set of data bytes.
// This should match exactly the encoding above.
func (p *TraceFile) UnmarshalBinary(data []byte) error {
	var err error
	//
	buffer := bytes.NewBuffer(data)
	// Read header
	if err = p.header.UnmarshalBinary(buffer); err != nil {
		return err
	} else if !p.header.IsCompatible() {
		return fmt.Errorf("incompatible binary trace file (v%d.%d))",
			p.header.MajorVersion, p.header.MinorVersion)
	}
	//
	switch p.header.MajorVersion {
	case 1:
		// Legacy Format
		p.heap, p.modules, err = FromBytesV1(buffer.Bytes())
	case 2:
		// Legacy format
		p.heap, p.modules, err = FromBytesV2(buffer.Bytes())
	case 3:
		// New format
		p.heap, p.modules, err = FromBytesV3(buffer.Bytes())
	default:
		panic("unreachable")
	}
	//
	return err
}

// Width implementation for trace.Trace interface
func (p *TraceFile) Width() uint {
	return uint(len(p.modules))
}

// Module groups together columns from the same module.
type Module[F any] struct {
	name    trace.ModuleName
	Columns []Column[F]
}

// NewModule constructs a new trace module with a given name and column set.
func NewModule[F any](name trace.ModuleName, columns []Column[F]) Module[F] {
	return Module[F]{name, columns}
}

// Name implementation for trace.Module interface
func (p *Module[F]) Name() trace.ModuleName {
	return p.name
}

// Column implementation for trace.Module interface
func (p *Module[F]) Column(id uint) trace.Column[F] {
	return &p.Columns[id]
}

// ColumnOf implementation for trace.Module interface
func (p *Module[F]) ColumnOf(name string) trace.Column[F] {
	panic("unsupported operation")
}

// FindLast implementation for the trace.Module interface.
func (p *Module[T]) FindLast(...T) uint {
	panic("unsupported operation")
}

// Keys implementation for the trace.Module interface.
func (p *Module[T]) Keys() uint {
	panic("unsupported operation")
}

// Width implementation for trace.Module interface
func (p *Module[F]) Width() uint {
	return uint(len(p.Columns))
}

// Height returns the height of this module in the trace.
func (p *Module[F]) Height() uint {
	if len(p.Columns) == 0 || p.Columns[0].Data() == nil {
		return 0
	}
	//
	return p.Columns[0].data.Len()
}

// Column captures the raw data for a given column.
type Column[F any] struct {
	// Name of the column
	name string
	// Data held in the column
	data array.MutArray[F]
}

// NewColumn constructs a new column
func NewColumn[F any](name string, data array.MutArray[F]) Column[F] {
	return Column[F]{name, data}
}

// Name implementation for trace.Column interface
func (p *Column[F]) Name() string {
	return p.name
}

// Data implementation for trace.Column interface
func (p *Column[F]) Data() array.Array[F] {
	return p.data
}

// MutData provides access to real data
func (p *Column[F]) MutData() array.MutArray[F] {
	return p.data
}

// Get implementation for trace.Column interface
func (p *Column[F]) Get(row int) F {
	if row < 0 || uint(row) >= p.data.Len() {
		var zero F
		// out-of-bounds access
		return zero
	}
	//
	return p.data.Get(uint(row))
}

// Padding implementation for trace.Column interface
func (p *Column[F]) Padding() F {
	panic("unsupported operation")
}
