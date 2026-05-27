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
package register

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
)

// Register represents a specific register in the schema that, eventually, will
// be mapped to one (or more) columns in the trace.  Observe that multiple
// registers can end up being mapped to the same column via "register
// allocation".  Likewise, a single register can end up being mapped across
// multiple columns as a result of subdivision to ensure field agnosticity.
// Hence, why they are referred to as registers rather than columns --- they are
// similar, but not identical, concepts.
type Register struct {
	// Kind of register (input / output)
	kind Type
	// Given name of this register.
	name string
	// Width (in bits) of this register
	width uint
	// Determines what value will be used to pad this register.
	padding big.Int
}

// New constructs a new register of a given kind (i.e. input, output or
// computed) with the given name and bitwidth.
func New(kind Type, name string, bitwidth uint, padding big.Int) Register {
	return Register{kind, name, bitwidth, padding}
}

// NewNative constructs a new "native" register.  That is, a register backed by
// a field element rather than a fixed-width unsigned integer.  Such registers
// have no fixed bitwidth and can hold any value representable in the
// underlying field.
func NewNative(kind Type, name string, padding big.Int) Register {
	return New(kind, name, math.MaxUint, padding)
}

// NewInput constructs a new input register with the given name and
// bitwidth.
func NewInput(name string, bitwidth uint, padding big.Int) Register {
	return Register{INPUT_REGISTER, name, bitwidth, padding}
}

// NewOutput constructs a new output register with the given name and
// bitwidth.
func NewOutput(name string, bitwidth uint, padding big.Int) Register {
	return Register{OUTPUT_REGISTER, name, bitwidth, padding}
}

// NewComputed constructs a new computed register with the given name and
// bitwidth.
func NewComputed(name string, bitwidth uint, padding big.Int) Register {
	return Register{COMPUTED_REGISTER, name, bitwidth, padding}
}

// NewConst constructs a new "constant register".  That is a register which
// always holds a constant value.  Currently, only constants 0 or 1 are
// supported.
func NewConst(value uint8) Register {
	var name = fmt.Sprintf("%d", value)
	//
	switch value {
	case 0:
		return Register{ZERO_REGISTER, name, 0, *big.NewInt(0)}
	case 1:
		return Register{ONE_REGISTER, name, 1, *big.NewInt(1)}
	default:
		panic(fmt.Sprintf("unsupported constant register (%d)", value))
	}
}

// Bound returns the first value which cannot be represented by the given
// bitwidth.  For example, the bound of an 8bit register is 256.
func (p *Register) Bound() *big.Int {
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
func (p *Register) IsInput() bool {
	return p.kind == INPUT_REGISTER
}

// IsInputOutput determines whether or not this is an input or output register
func (p *Register) IsInputOutput() bool {
	return p.IsInput() || p.IsOutput()
}

// IsOutput determines whether or not this is an output register
func (p *Register) IsOutput() bool {
	return p.kind == OUTPUT_REGISTER
}

// IsComputed determines whether or not this is a computed register.  Observer
// that "zero" registers are included in this, since they are neither input nor
// output registers.
func (p *Register) IsComputed() bool {
	return p.kind == COMPUTED_REGISTER || p.IsConst()
}

// IsConst determines whether or not this is a constant "zero" or "one" register
func (p *Register) IsConst() bool {
	return p.kind == ZERO_REGISTER || p.kind == ONE_REGISTER
}

// IsNative determines whether or not this is a "native" register.  That is, a
// register backed by a field element rather than a fixed-width unsigned
// integer.
func (p *Register) IsNative() bool {
	return p.width == math.MaxUint
}

// ConstValue determines the constant value for a given constant register.
func (p *Register) ConstValue() uint8 {
	switch p.kind {
	case ZERO_REGISTER:
		return 0
	case ONE_REGISTER:
		return 1
	default:
		panic("register not constant")
	}
}

// MaxValue returns the largest value expressible in this register (i.e. Bound() -
// 1).  For example, the max value of an 8bit register is 255.
func (p *Register) MaxValue() *big.Int {
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
func (p Register) Kind() Type {
	return p.kind
}

// Name returns the (unqualified) name of this register
func (p Register) Name() string {
	return p.name
}

// QualifiedName returns the fully qualified name of this register
func (p Register) QualifiedName(mod Map) string {
	var (
		name    = p.name
		modName = mod.Name().String()
	)
	//
	if modName != "" {
		return fmt.Sprintf("%s:%s", modName, name)
	}
	//
	return name
}

// Padding determines what value will be used to padd this register.
func (p *Register) Padding() *big.Int {
	return &p.padding
}

// SetPadding updates the padding value to use for this register.
func (p *Register) SetPadding(padding *big.Int) {
	switch p.kind {
	case ZERO_REGISTER, ONE_REGISTER:
		if padding.IsUint64() && padding.Uint64() == uint64(p.ConstValue()) {
			return
		}
		// Sanity Check
		panic(fmt.Sprintf(
			"cannot overide padding of constant register (%s vs %d)", padding.String(), p.ConstValue()))
	}
	//
	p.padding.Set(padding)
}

func (p Register) String() string {
	if p.IsNative() {
		return fmt.Sprintf("%s:𝔽:0x%s", p.name, p.padding.Text(16))
	}
	//
	return fmt.Sprintf("%s:u%d:0x%s", p.name, p.width, p.padding.Text(16))
}

// Width determines the bitwidth of this register.  This panics if called on a
// native register, since such registers have no fixed bitwidth.  Callers should
// guard with IsNative() if the register may be native.
func (p Register) Width() uint {
	if p.IsNative() {
		panic(fmt.Sprintf("Width() called on native register %q", p.name))
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
func (p *Register) MarshalBinary() (data []byte, err error) {
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
	//
	if err := binary.Write(buffer, binary.BigEndian, width); err != nil {
		return nil, err
	}
	// Write register name
	if err := writeByteArray(buffer, []byte(p.name)); err != nil {
		return nil, err
	}
	// Read register padding
	if err := writeByteArray(buffer, p.padding.Bytes()); err != nil {
		return nil, err
	}
	// Success
	return buffer.Bytes(), nil
}

// UnmarshalBinary unmarshals a register
func (p *Register) UnmarshalBinary(data []byte) error {
	var (
		buffer        = bytes.NewBuffer(data)
		kind          uint8
		width         uint16
		name, padding []byte
		err           error
	)
	// Register kind
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
	p.padding.SetBytes(padding)
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
func WidthOfRegisters(regs []Register, rids []Id) uint {
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
