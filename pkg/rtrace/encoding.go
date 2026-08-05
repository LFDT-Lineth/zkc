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
package rtrace

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// MarshalBinary encodes a trace into a binary format.  Lengths, counts and
// descriptor metadata are encoded as unsigned varints.  Register data is
// encoded column-major, with each column written using the natural encoding of
// its underlying array representation (see narray.Array.Encode).
func MarshalBinary[T any, M ModuleBuilder[T, M]](tr Array[T, M]) ([]byte, error) {
	var buffer bytes.Buffer
	//
	buffer.Write(rtraceBinaryMagic)
	writeUvarint(&buffer, tr.Width())
	//
	for mid := uint(0); mid < tr.Width(); mid++ {
		marshalModule(&buffer, tr.Module(mid))
	}
	//
	return buffer.Bytes(), nil
}

// UnmarshalBinary decodes a trace encoded by MarshalBinary.
func (p *Array[T, M]) UnmarshalBinary(data []byte) error {
	var buffer = bytes.NewBuffer(data)
	//
	modules, err := unmarshalModules[T, M](buffer)
	if err != nil {
		return err
	} else if buffer.Len() != 0 {
		return fmt.Errorf("malformed rtrace binary: %d trailing bytes", buffer.Len())
	}
	//
	p.modules = modules
	//
	return nil
}

func marshalModule[T any](buffer *bytes.Buffer, module Module[T]) {
	var metadata uint
	// Build metadata
	if module.Descriptor().Replicated {
		metadata |= 1
	}
	//
	writeString(buffer, module.Name())
	marshalDescriptors(buffer, module.Descriptor().Columns)
	writeUvarint(buffer, metadata)
	writeUvarint(buffer, module.Height())
	//
	for cid := uint(0); cid < module.Width(); cid++ {
		module.Column(cid).Encode(buffer)
	}
}

func unmarshalModules[T any, M ModuleBuilder[T, M]](buffer *bytes.Buffer) ([]M, error) {
	if buffer.Len() < len(rtraceBinaryMagic) {
		return nil, fmt.Errorf("malformed rtrace binary: missing header")
	}
	//
	magic := buffer.Next(len(rtraceBinaryMagic))
	if !bytes.Equal(magic, rtraceBinaryMagic) {
		return nil, fmt.Errorf("malformed rtrace binary: invalid header")
	}
	//
	return readSlice(buffer, unmarshalModule[T, M])
}

func unmarshalModule[T any, M ModuleBuilder[T, M]](buffer *bytes.Buffer) (M, error) {
	var (
		r = reader{buf: buffer}
		//
		name        = r.str()
		descriptors = readWith(&r, readColumnDescriptors)
		metadata    = r.uvarint()
		height      = r.uvarint()
		replicated  = metadata != 0
		//
		module M
	)
	//
	if r.err != nil {
		return module, r.err
	}
	// Initialise (empty) module, thereby allocating an appropriate array
	// representation for each descriptor.
	module = module.Initialise(ModuleDescriptor{name, descriptors, replicated})
	// Decode each column in place.
	for cid := range uint(len(descriptors)) {
		if err := module.MutColumn(cid).Decode(height, buffer); err != nil {
			return module, err
		}
	}
	//
	return module, nil
}

func readColumnDescriptors(buffer *bytes.Buffer) ([]ColumnDescriptor, error) {
	return readSlice(buffer, readColumnDescriptor)
}

func readColumnDescriptor(buffer *bytes.Buffer) (ColumnDescriptor, error) {
	r := reader{buf: buffer}
	//
	name := r.str()
	bitwidth := r.optionUint()
	//
	if r.err != nil {
		return ColumnDescriptor{}, r.err
	}
	//
	return ColumnDescriptor{name, bitwidth}, nil
}

// writeSlice writes a length-prefixed sequence of items: the count as an
// unsigned varint, followed by each item via the given writer.
func writeSlice[T any](buffer *bytes.Buffer, items []T, write func(*bytes.Buffer, T)) {
	writeUvarint(buffer, uint(len(items)))
	//
	for _, item := range items {
		write(buffer, item)
	}
}

// readSlice reads a length-prefixed sequence of items: the count as an unsigned
// varint, followed by each item via the given reader.
func readSlice[T any](buffer *bytes.Buffer, read func(*bytes.Buffer) (T, error)) ([]T, error) {
	n, err := readUvarint(buffer)
	if err != nil {
		return nil, err
	}
	//
	items := make([]T, n)
	//
	for i := range n {
		if items[i], err = read(buffer); err != nil {
			return nil, err
		}
	}
	//
	return items, nil
}

// reader wraps a buffer with a sticky error, allowing a sequence of reads to be
// expressed without checking the error after each one.  Once a read fails, all
// subsequent reads are skipped and yield zero values, and the first error is
// retained in err.
type reader struct {
	buf *bytes.Buffer
	err error
}

// readWith applies a reader function through a sticky reader, retaining the
// first error encountered.  Returns the zero value once in an error state.
func readWith[T any](r *reader, read func(*bytes.Buffer) (T, error)) T {
	if r.err != nil {
		var zero T
		return zero
	}
	//
	value, err := read(r.buf)
	r.err = err
	//
	return value
}

func (r *reader) uvarint() uint                 { return readWith(r, readUvarint) }
func (r *reader) str() string                   { return readWith(r, readString) }
func (r *reader) optionUint() util.Option[uint] { return readWith(r, readOptionUint) }

func marshalDescriptors(buffer *bytes.Buffer, descriptors []ColumnDescriptor) {
	writeSlice(buffer, descriptors, func(buffer *bytes.Buffer, reg ColumnDescriptor) {
		writeString(buffer, reg.Name)
		writeOptionUint(buffer, reg.Bitwidth)
	})
}

func writeString(buffer *bytes.Buffer, str string) {
	writeUvarint(buffer, uint(len(str)))
	buffer.WriteString(str)
}

func writeOptionUint(buffer *bytes.Buffer, option util.Option[uint]) {
	if option.IsEmpty() {
		writeUvarint(buffer, 0)
	} else {
		writeUvarint(buffer, 1)
		writeUvarint(buffer, option.Unwrap())
	}
}

func readString(buffer *bytes.Buffer) (string, error) {
	length, err := readUvarint(buffer)
	if err != nil {
		return "", err
	} else if length > uint(buffer.Len()) {
		return "", fmt.Errorf("malformed rtrace binary: string length %d exceeds remaining bytes %d", length, buffer.Len())
	}
	//
	return string(buffer.Next(int(length))), nil
}

func readOptionUint(buffer *bytes.Buffer) (util.Option[uint], error) {
	tag, err := readUvarint(buffer)
	if err != nil {
		return util.None[uint](), err
	}
	//
	switch tag {
	case 0:
		return util.None[uint](), nil
	case 1:
		value, err := readUvarint(buffer)
		if err != nil {
			return util.None[uint](), err
		}
		//
		return util.Some(value), nil
	default:
		return util.None[uint](), fmt.Errorf("malformed rtrace binary: invalid option tag %d", tag)
	}
}

func writeUvarint(buffer *bytes.Buffer, value uint) {
	var bytes [binary.MaxVarintLen64]byte

	n := binary.PutUvarint(bytes[:], uint64(value))
	buffer.Write(bytes[:n])
}

func readUvarint(buffer *bytes.Buffer) (uint, error) {
	value, err := binary.ReadUvarint(buffer)
	if err != nil {
		return 0, err
	} else if value > uint64(^uint(0)) {
		return 0, fmt.Errorf("uvarint %d overflows uint", value)
	}
	//
	return uint(value), nil
}
