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
package bytecode

import (
	"bytes"
	"encoding/gob"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	lword "github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Type wraps a raw register type with additional meta-information (i.e. that is
// not required at the constraints level).
type Type struct {
	// underlying register type
	underlying register.Type
	// flag to indicate whether this register is part of an synthetic stamp
	// register (i.e. added by ThreadTimeStamps).
	stamp bool
	// safe registers have dedicated checks to enforce their bounds.
	safe bool
}

var (
	// INPUT_REGISTER signals a register used for holding the input values of a
	// function.
	INPUT_REGISTER = Type{register.INPUT_REGISTER, false, true}
	// OUTPUT_REGISTER signals a register used for holding the output values of
	// a function.
	OUTPUT_REGISTER = Type{register.OUTPUT_REGISTER, false, true}
	// COMPUTED_REGISTER signals a register whose values are computed from one
	// (or more) assignments during trace expansion.
	COMPUTED_REGISTER = Type{register.COMPUTED_REGISTER, false, true}
)

// WithSafety returns the given type with its safety status updated according to
// the given flag.  All other aspects of the type remain unchanged.
func (p Type) WithSafety(safe bool) Type {
	p.safe = safe
	//
	return p
}

// WithStamp returns the given type with its stamp status updated according to
// the given flag.  All other aspects of the type remain unchanged.
func (p Type) WithStamp(stamp bool) Type {
	p.stamp = stamp
	//
	return p
}

// Cmp implementation for Comparable interface
func (p Type) Cmp(other Type) int {
	if c := p.underlying.Cmp(other.underlying); c != 0 {
		return c
	} else if p.stamp == other.stamp {
		return 0
	} else if p.stamp {
		return -1
	}
	//
	return 1
}

// Register represents an individual register in a module that, eventually, will
// be mapped to one (or more) columns in the trace.  Likewise, a single register
// can end up being mapped across multiple columns as a result of register
// splitting  to ensure field agnosticity. Hence, why they are referred to as
// registers rather than columns --- they are similar, but not identical,
// concepts.
type Register[W word.Word[W]] struct {
	// Kind of register (input / output)
	kind Type
	// Given name of this register.
	name string
	// Bitwidth holds the bitwidth of word registers, otherwise is empty (for
	// field registers).
	bitwidth util.Option[uint]
	// Determines what value will be used to pad this register.
	padding W
}

// NewRegister constructs a new register descriptor.
func NewRegister[W word.Word[W]](kind Type, name string, bitwidth util.Option[uint], padding W) Register[W] {
	if bitwidth.HasValue() && bitwidth.Unwrap() == math.MaxUint {
		panic("invalid register bitwidth")
	}
	//
	return Register[W]{kind, name, bitwidth, padding}
}

// Bitwidth determines the bitwidth of this register (if applicable).  Observe
// that native registers have no explicit bitwidth and, hence, this simply
// returns none in such cases.
func (p Register[W]) Bitwidth() util.Option[uint] {
	return p.bitwidth
}

// Bytewidth determines the number of bytes required to hold any value stored in
// this register (if applicable).  Observe that native registers have no
// explicit bytewidth and, hence, this simply returns none in such cases.
func (p Register[W]) Bytewidth() util.Option[uint] {
	if p.bitwidth.HasValue() {
		return util.Some(lword.ByteWidth(p.bitwidth.Unwrap()))
	}
	//
	return util.None[uint]()
}

// Kind returns the kind of this register (e.g. input, output, computed).
func (p Register[W]) Kind() Type {
	return p.kind
}

// IsInput determines whether or not this is an input register
func (p Register[W]) IsInput() bool {
	return p.kind.underlying == register.INPUT_REGISTER
}

// IsInputOutput determines whether or not this is an input or output register
func (p Register[W]) IsInputOutput() bool {
	return p.IsInput() || p.IsOutput()
}

// IsNative determines whether or not this is a native register
func (p Register[W]) IsNative() bool {
	return !p.bitwidth.HasValue()
}

// IsOutput determines whether or not this is an output register
func (p Register[W]) IsOutput() bool {
	return p.kind.underlying == register.OUTPUT_REGISTER
}

// IsComputed determines whether or not this is a computed register.  Observer
// that "zero" registers are included in this, since they are neither input nor
// output registers.
func (p Register[W]) IsComputed() bool {
	return p.kind.underlying == register.COMPUTED_REGISTER
}

// IsStamp determines whether or not this is a stamp register
func (p Register[W]) IsStamp() bool {
	return p.kind.stamp
}

// IsSafe determines whether the bitwidth of this register is "safe" --- i.e. is
// guaranteed by a dedicated bounds check.
func (p Register[W]) IsSafe() bool {
	return p.kind.safe
}

// IsZeroWidth returns true for zero-width registers. Zero-width registers are registers
// that carry no data, so apparent reads and writes to a shared placeholder cannot
// conflict.
func (p Register[W]) IsZeroWidth() bool {
	width := p.Bitwidth()
	return width.HasValue() && width.Unwrap() == 0
}

// Name returns the  name of this register
func (p Register[W]) Name() string {
	return p.name
}

// Padding returns the padding for this register
func (p Register[W]) Padding() W {
	return p.padding
}

// ToRawRegister converts a register descriptor into a schema register
func (p Register[W]) ToRawRegister() register.Register {
	var (
		bitwidth uint = math.MaxUint
	)
	// Determine bitwidth (if applicable)
	if !p.IsNative() {
		bitwidth = p.Bitwidth().Unwrap()
	} else if p.Padding().Cmp64(0) != 0 {
		// NOTE: this is a stop-gap measure to ensure no padding values are
		// dropped.  Eventually, the notion of padding would be dropped entirely
		// from the concept of a register.
		panic("non-zero padding unsupported")
	}
	//
	return register.New(p.kind.underlying, p.Name(), bitwidth)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode marshals this register descriptor.  All of the register's fields
// are unexported, so an explicit encoding is required (gob would otherwise skip
// them).
//
// nolint
func (p *Register[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.kind.safe); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.kind.stamp); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.kind.underlying); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.name); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.bitwidth); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.padding); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Register[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.kind.safe); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.kind.stamp); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.kind.underlying); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.name); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.bitwidth); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.padding); err != nil {
		return err
	}
	//
	return nil
}
