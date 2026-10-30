// Copyright Consensys Incorporated
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
package register

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

var (
	one big.Int
)

// Register represents a specific register in the schema that, eventually, will
// be mapped to one (or more) columns in the trace.  Observe that multiple
// registers can end up being mapped to the same column via "register
// allocation".
type Register[F field.Element[F]] struct {
	// Kind of register (input / output)
	kind Type
	// Given name of this register.
	name string
	// Width (in bits) of this register
	width uint
	// Padding value of this register
	padding F
}

// New constructs a new register of a given kind (i.e. input, output or
// computed) with the given name and bitwidth.
func New[F field.Element[F]](kind Type, name string, bitwidth uint) Register[F] {
	var padding F
	return Register[F]{kind, name, bitwidth, padding}
}

// NewNative constructs a new "native" register.  That is, a register backed by
// a field element rather than a fixed-width unsigned integer.  Such registers
// have no fixed bitwidth and can hold any value representable in the
// underlying field.
func NewNative[F field.Element[F]](kind Type, name string) Register[F] {
	return New[F](kind, name, math.MaxUint)
}

// NewInput constructs a new input register with the given name and
// bitwidth.
func NewInput[F field.Element[F]](name string, bitwidth uint, padding F) Register[F] {
	return Register[F]{INPUT_REGISTER, name, bitwidth, padding}
}

// NewOutput constructs a new output register with the given name and
// bitwidth.
func NewOutput[F field.Element[F]](name string, bitwidth uint, padding F) Register[F] {
	return Register[F]{OUTPUT_REGISTER, name, bitwidth, padding}
}

// NewComputed constructs a new computed register with the given name and
// bitwidth.
func NewComputed[F field.Element[F]](name string, bitwidth uint, padding F) Register[F] {
	return Register[F]{COMPUTED_REGISTER, name, bitwidth, padding}
}

// NewConstant constructs a new constant register with the given name, bitwidth
// and constant value.  NOTE: this will panic if the constant value does not fit
// within the specified bitwidth.
func NewConstant[F field.Element[F]](name string, bitwidth uint, constant F) Register[F] {
	util.Assert(constant.FitsWithin(bitwidth), "constant %s out-of-bounds for u%d", constant.String(), bitwidth)
	//
	return Register[F]{CONSTANT_REGISTER, name, bitwidth, constant}
}

// Bound returns the first value which cannot be represented by the given
// bitwidth.  For example, the bound of an 8bit register is 256.
func (p *Register[F]) Bound() *big.Int {
	if p.IsNative() {
		panic(fmt.Sprintf("Bound() called on native register %q", p.name))
	}
	//
	var (
		bound = big.NewInt(2)
		width = big.NewInt(int64(p.width))
	)
	// Compute 2^n
	return bound.Exp(bound, width, nil)
}

// IsInput determines whether or not this is an input register
func (p *Register[F]) IsInput() bool {
	return p.kind == INPUT_REGISTER
}

// IsInputOutput determines whether or not this is an input or output register
func (p *Register[F]) IsInputOutput() bool {
	return p.IsInput() || p.IsOutput()
}

// IsOutput determines whether or not this is an output register
func (p *Register[F]) IsOutput() bool {
	return p.kind == OUTPUT_REGISTER
}

// IsComputed determines whether or not this is a computed register.
func (p *Register[F]) IsComputed() bool {
	return p.kind == COMPUTED_REGISTER
}

// IsConstant determines whether or not this is a constant register, or not.
func (p *Register[F]) IsConstant() bool {
	return p.kind == CONSTANT_REGISTER
}

// Constant returns the constant value associated with this register (or panics
// if this is not a constant register).  Observe that the value returned is
// identical to the padding value of this register.
func (p *Register[F]) Constant() F {
	util.Assert(p.kind == CONSTANT_REGISTER, "register not constant")
	return p.padding
}

// IsNative determines whether or not this is a "native" register.  That is, a
// register backed by a field element rather than a fixed-width unsigned
// integer.
func (p *Register[F]) IsNative() bool {
	return p.width == math.MaxUint
}

// MaxValue returns the largest value expressible in this register (i.e. Bound() -
// 1).  For example, the max value of an 8bit register is 255.
func (p *Register[F]) MaxValue() *big.Int {
	if p.IsNative() {
		panic(fmt.Sprintf("MaxValue() called on native register %q", p.name))
	}
	//
	max := p.Bound()
	max.Sub(max, &one)
	//
	return max
}

// Kind returns the type of this register
func (p Register[F]) Kind() Type {
	return p.kind
}

// Name returns the (unqualified) name of this register
func (p Register[F]) Name() string {
	return p.name
}

// Padding returns the padding value associated with this register.
func (p *Register[F]) Padding() F {
	return p.padding
}

// QualifiedName returns the fully qualified name of this register
func (p Register[F]) QualifiedName(mod Map[F]) string {
	var (
		name    = p.name
		modName = mod.Name()
	)
	//
	if modName != "" {
		return fmt.Sprintf("%s:%s", modName, name)
	}
	//
	return name
}

func (p Register[F]) String() string {
	if p.IsNative() {
		return fmt.Sprintf("%s:𝔽", p.name)
	}
	//
	return fmt.Sprintf("%s:u%d", p.name, p.width)
}

// Width determines the bitwidth of this register.  This panics if called on a
// native register, since such registers have no fixed bitwidth.  Callers should
// guard with IsNative() if the register may be native.
func (p Register[F]) Width() uint {
	if p.IsNative() {
		panic(fmt.Sprintf("Width() called on native register %q", p.name))
	}
	//
	return p.width
}

// WidthOrNative returns the bitwidth of this register, or math.MaxUint if it is a native register.
func (p Register[F]) WidthOrNative() uint {
	if p.IsNative() {
		return math.MaxUint
	}
	//
	return p.width
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nativeWidthSentinel is written in place of the bitwidth field when
// serialising a native register.  It is the maximum representable value of the
// uint16 wire format, and is mapped back to math.MaxUint on unmarshal.
const nativeWidthSentinel uint16 = 0xFFFF

// MarshalBinary converts a register into binary data
func (p *Register[F]) MarshalBinary() (data []byte, err error) {
	var (
		buffer = bytes.NewBuffer(data)
		width  uint16
	)
	// Register kind
	if err := binary.Write(buffer, binary.BigEndian, p.kind); err != nil {
		return nil, err
	}
	// Register bitwidth
	if p.IsNative() {
		width = nativeWidthSentinel
	} else {
		width = uint16(p.width)
	}
	// Bitwidth
	if err := binary.Write(buffer, binary.BigEndian, width); err != nil {
		return nil, err
	}
	// Write register name
	if err := writeByteArray(buffer, []byte(p.name)); err != nil {
		return nil, err
	}
	// Write padding
	if err := writeByteArray(buffer, p.padding.Bytes()); err != nil {
		return nil, err
	}
	// Success
	return buffer.Bytes(), nil
}

// UnmarshalBinary unmarshals a register
func (p *Register[F]) UnmarshalBinary(data []byte) error {
	var (
		buffer  = bytes.NewBuffer(data)
		kind    uint8
		width   uint16
		name    []byte
		padding []byte
		err     error
	)
	// Register[F] kind
	if err := binary.Read(buffer, binary.BigEndian, &kind); err != nil {
		return err
	}
	// Register bitwidth
	if err := binary.Read(buffer, binary.BigEndian, &width); err != nil {
		return err
	}
	// Read register name
	if name, err = readByteArray(buffer); err != nil {
		return err
	}
	// Read register padding
	if padding, err = readByteArray(buffer); err != nil {
		return err
	}
	//
	p.kind = Type{kind}
	if width == nativeWidthSentinel {
		p.width = math.MaxUint
	} else {
		p.width = uint(width)
	}
	//
	p.name = string(name)
	p.padding = p.padding.SetBytes(padding)
	// Success!
	return nil
}

func readByteArray(buf *bytes.Buffer) ([]byte, error) {
	var (
		len  uint8
		data []byte
	)
	//
	// Register name length
	if err := binary.Read(buf, binary.BigEndian, &len); err != nil {
		return nil, err
	}

	data = make([]byte, len)
	//
	if n, err := buf.Read(data); err != nil {
		return nil, err
	} else if n != int(len) {
		return nil, errors.New("malformed register encoding")
	}
	//
	return data, nil
}

func writeByteArray(buf *bytes.Buffer, data []byte) error {
	var len uint8 = uint8(len(data))
	// Data length
	if err := binary.Write(buf, binary.BigEndian, len); err != nil {
		return err
	}
	// Data itself
	if n, err := buf.Write(data); err != nil {
		return err
	} else if n != int(len) {
		return errors.New("malformed register encoding")
	}
	// Success
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// ToString provides a simplistic default string implementation for a
// RegisterId.  This is useful primarily for debugging where we want to e.g.
// print a constraint but don't have access to an appropriate mapping, etc.
func ToString(rid Id) string {
	return fmt.Sprintf("#%d", rid.Unwrap())
}

// WidthOfRegisters returns the combined bitwidth of the given
// registers.  For example, suppose we have three registers: x:u8, y:u8, z:u11.
// Then the combined width is 8+8+11=27.
func WidthOfRegisters[F field.Element[F]](regs []Register[F], rids []Id) uint {
	var (
		width uint
	)
	//
	for _, rid := range rids {
		width += regs[rid.Unwrap()].Width()
	}
	//
	return width
}

func init() {
	one = *big.NewInt(1)
}
