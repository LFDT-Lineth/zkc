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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// MarshalBinary encodes a row-major trace into a binary format.  Lengths,
// counts and descriptor metadata are encoded as unsigned varints.  Row data is
// encoded row-major, using unsigned LEB128 for each word value.
func MarshalBinary[W util.BigInter, M ModuleBuilder[W, M]](tr Array[W, M]) ([]byte, error) {
	var buffer bytes.Buffer
	//
	buffer.Write(rtraceBinaryMagic)
	writeUvarint(&buffer, tr.Width())
	//
	for mid := uint(0); mid < tr.Width(); mid++ {
		if err := marshalModule[W](&buffer, tr.Module(mid)); err != nil {
			return nil, err
		}
	}
	//
	return buffer.Bytes(), nil
}

// UnmarshalBinary decodes a row-major trace encoded by MarshalBinary.
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

func marshalModule[W util.BigInter](buffer *bytes.Buffer, module Module[W]) error {
	writeString(buffer, module.Name())
	marshalRegisters(buffer, module.Descriptor().Collect())
	writeUvarint(buffer, module.Height())
	//
	for rid := uint(0); rid < module.Height(); rid++ {
		row := module.Row(rid)
		//
		if row.Width() != module.Width() {
			return fmt.Errorf("invalid row width (have %d, expected %d)", row.Width(), module.Width())
		}
		//
		for cid := uint(0); cid < row.Width(); cid++ {
			if err := writeWordULEB128(buffer, row.Get(cid)); err != nil {
				return err
			}
		}
	}
	//
	return nil
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
		name      = r.str()
		registers = readWith(&r, readRegisters)
		height    = r.uvarint()
		width     = len(registers)
		//
		module M
	)
	//
	if r.err != nil {
		return module, r.err
	}
	//
	var err error
	//
	rows := make([][]T, height)
	//
	for rid := range height {
		rows[rid] = make([]T, width)
		//
		for cid := range width {
			if rows[rid][cid], err = readWordULEB128[T](buffer); err != nil {
				return module, err
			}
		}
	}
	// Initialise new module
	return module.Initialise(name, registers, rows...), nil
}

func readRegisters(buffer *bytes.Buffer) ([]Register, error) {
	return readSlice(buffer, readRegister)
}

func readRegister(buffer *bytes.Buffer) (Register, error) {
	r := reader{buf: buffer}
	//
	name := r.str()
	bitwidth := r.optionUint()
	//
	if r.err != nil {
		return Register{}, r.err
	}
	//
	return Register{name, bitwidth}, nil
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

func marshalRegisters(buffer *bytes.Buffer, registers []Register) {
	writeSlice(buffer, registers, func(buffer *bytes.Buffer, reg Register) {
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

func writeWordULEB128[W util.BigInter](buffer *bytes.Buffer, word W) error {
	value := word.BigInt()
	if value.Sign() < 0 {
		return fmt.Errorf("cannot encode negative word %s", value.String())
	}
	//
	writeBigULEB128(buffer, value)
	//
	return nil
}

func writeBigULEB128(buffer *bytes.Buffer, value *big.Int) {
	var tmp big.Int
	//
	tmp.Set(value)
	//
	for {
		next := byte(tmp.Uint64() & 0x7f)
		tmp.Rsh(&tmp, 7)
		//
		if tmp.Sign() != 0 {
			next |= 0x80
		}
		//
		buffer.WriteByte(next)
		//
		if tmp.Sign() == 0 {
			return
		}
	}
}

type bigIntSetter[T any] interface {
	SetBigInt(*big.Int) T
}

func readWordULEB128[T any](buffer *bytes.Buffer) (word T, err error) {
	value, err := readBigULEB128(buffer)
	if err != nil {
		return word, err
	}
	//
	setter, ok := any(word).(bigIntSetter[T])
	if !ok {
		return word, fmt.Errorf("rtrace Array.UnmarshalBinary requires word values with SetBigInt")
	}
	//
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid word value %s: %v", value.String(), r)
		}
	}()
	//
	word = setter.SetBigInt(value)
	//
	return word, nil
}

func readBigULEB128(buffer *bytes.Buffer) (*big.Int, error) {
	var (
		result big.Int
		shift  uint
	)
	//
	for {
		if buffer.Len() == 0 {
			return nil, fmt.Errorf("malformed rtrace binary: unterminated LEB128 word")
		}
		//
		next, err := buffer.ReadByte()
		if err != nil {
			return nil, err
		}
		//
		var part big.Int
		part.SetUint64(uint64(next & 0x7f))
		part.Lsh(&part, shift)
		result.Add(&result, &part)
		//
		if next&0x80 == 0 {
			return &result, nil
		} else if shift+7 < shift {
			return nil, fmt.Errorf("malformed rtrace binary: LEB128 shift overflow")
		}
		//
		shift += 7
	}
}
