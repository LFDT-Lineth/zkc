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
package io

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

const (
	// PC_NAME gives the name used for the program counter in traces.
	PC_NAME = "$pc"
	// RET_NAME gives the name used for the return line in traces.
	RET_NAME = "$ret"
)

// Register defines the notion of a register within a function.
type Register = register.Register

// RegisterId abstracts the notion of a register id.
type RegisterId = register.Id

const (
	// UNUSED_REGISTER provides a simple way to distinguish registers and
	// constants in certain instructions.
	UNUSED_REGISTER = math.MaxUint
)

// Function defines a distinct functional entity within the system.  Functions
// accepts zero or more inputs and produce zero or more outputs.  Functions
// declare zero or more internal registers for use, and their interpretation is
// given by a sequence of zero or more instructions.
type Function[T Instruction] struct {
	// Unique name of this function.
	name string
	// Indicates whether this is an externally visible function (or not).
	public bool
	// Registers describes zero or more registers of a given width.  Each
	// register can be designated as an input / output or temporary.
	registers []Register
	// Buses describes any buses used within this function.
	buses []Bus
	// Number of input registers
	numInputs uint
	// Number of output registers
	numOutputs uint
	// Code defines the body of this function.
	code []T
}

// NewFunction constructs a new function with the given components.
func NewFunction[T Instruction](name module.Name, public bool, registers []Register, buses []Bus,
	code []T) Function[T] {
	//
	var (
		numInputs  = array.CountMatching(registers, func(r Register) bool { return r.IsInput() })
		numOutputs = array.CountMatching(registers, func(r Register) bool { return r.IsOutput() })
	)
	// Check registers sorted as: inputs, outputs then internal.
	if !registersCorrectlyOrdered(registers) {
		panic("function registers ordered incorrectly")
	} else if name.Multiplier != 1 {
		panic("functions only support multiplers of 1")
	}
	// All good
	return Function[T]{name.Name, public, registers, buses, numInputs, numOutputs, code}
}

// Buses returns the set of all buses used by any instruction within this
// function.
func (p *Function[T]) Buses() []Bus {
	return p.buses
}

// CodeAt returns the ith instruction making up the body of this function.
func (p *Function[T]) CodeAt(i uint) T {
	return p.code[i]
}

// Code returns the instructions making up the body of this function.
func (p *Function[T]) Code() []T {
	return p.code
}

// IsPublic determines whether or not this is an "externally visible" function
// or not.  The differences between internal and external functions is small.
// Specifically, internal functions are not visible in the generated trace
// interface; likewise, they are hidden by default in the inspector.
func (p *Function[T]) IsPublic() bool {
	return p.public
}

// IsSynthetic modules are generated during compilation, rather than being
// provided by the user.  At this time, functions can never be synthetic
func (p *Function[T]) IsSynthetic() bool {
	return false
}

// IsNative reports whether this module corresponds to a function backed by
// a native circuit.  Assembly-level functions are never native; only ZkC
// modules can be.
func (p *Function[T]) IsNative() bool {
	return false
}

// IsStatic reports whether this module is a static reference table.
// Assembly-level functions are never static.
func (p *Function[T]) IsStatic() bool {
	return false
}

// IsAtomic determines whether or not this is a "one line function".  That is,
// where every instance of this function occupies exactly one line in the
// corresponding trace.  This is useful to know, as certain optimisations can be
// applied for one line functions (e.g. no PC register is required).
func (p *Function[T]) IsAtomic() bool {
	return len(p.code) == 1
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p *Function[T]) HasRegister(name string) (RegisterId, bool) {
	for i, r := range p.registers {
		if r.Name() == name {
			return register.NewId(uint(i)), true
		}
	}
	// Failed
	return register.UnusedId(), false
}

// Inputs returns the set of input registers for this function.
func (p *Function[T]) Inputs() []Register {
	return p.registers[:p.numInputs]
}

// NumInputs returns the number of input registers for this function.
func (p *Function[T]) NumInputs() uint {
	return p.numInputs
}

// NumOutputs returns the number of output registers for this function.
func (p *Function[T]) NumOutputs() uint {
	return p.numOutputs
}

// Name returns the name of this function.
func (p *Function[T]) Name() module.Name {
	// Functions always have a multiplier of 1.
	return module.NewName(p.name, 1)
}

// Outputs returns the set of output registers for this function.
func (p *Function[T]) Outputs() []Register {
	return p.registers[p.numInputs : p.numInputs+p.numOutputs]
}

// Register returns the ith register used in this function.
func (p *Function[T]) Register(id register.Id) Register {
	return p.registers[id.Unwrap()]
}

// Registers returns the set of all registers used during execution of this
// function.
func (p *Function[T]) Registers() []Register {
	return p.registers
}

// Width returns the number of registers in this module.'
func (p *Function[T]) Width() uint {
	return uint(len(p.registers))
}

// AllocateRegister allocates a new register of the given kind, name and width
// into this function.
func (p *Function[T]) AllocateRegister(kind register.Type, name string, width uint, padding big.Int) RegisterId {
	var (
		index = uint(len(p.registers))
	)
	// Sanity check
	if kind == register.INPUT_REGISTER || kind == register.OUTPUT_REGISTER {
		panic("cannot allocate input / output register")
	}
	//
	p.registers = append(p.registers, register.New(kind, name, width, padding))
	// Done
	return register.NewId(index)
}

// Validate that this function and all instructions contained therein is
// well-formed.  For example, that instructions have no conflicting writes, that
// all temporaries have been allocated, etc.  The maximum bit capacity of the
// underlying field is needed for this calculation, to allow instructions to
// check it does not overflow the underlying field.
func (p *Function[T]) Validate(fieldWidth uint) []error {
	var errors []error
	//
	for _, insn := range p.code {
		if err := insn.Validate(fieldWidth, p); err != nil {
			errors = append(errors, err)
		}
	}
	//
	return errors
}

func (p *Function[T]) String() string {
	return register.MapToString(p)
}

// ConstRegister implementation for register.ConstMap interface
func (p *Function[T]) ConstRegister(constant uint8) RegisterId {
	var (
		name  = fmt.Sprintf("%d", constant)
		nregs = uint(len(p.registers))
	)
	// Check whether register already exists
	if rid, ok := p.HasRegister(name); ok {
		return rid
	}
	// Allocate constant register
	p.registers = append(p.registers, register.NewConst(constant))
	//
	return register.NewId(nregs)
}

func registersCorrectlyOrdered(registers []register.Register) bool {
	var index = 0
	// Inputs first
	for index < len(registers) && registers[index].IsInput() {
		index++
	}
	// Outputs next
	for index < len(registers) && registers[index].IsOutput() {
		index++
	}
	// Then computed (or constant)
	for index < len(registers) && registers[index].IsComputed() {
		index++
	}
	// Should be nothing else
	return index == len(registers)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// nolint
func (p *Function[T]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.name); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.registers); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.buses); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.code); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Function[T]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.name); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.registers); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.buses); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.code); err != nil {
		return err
	}
	// Recompute internal values
	p.numInputs = array.CountMatching(p.registers, func(r Register) bool { return r.IsInput() })
	p.numOutputs = array.CountMatching(p.registers, func(r Register) bool { return r.IsOutput() })
	// Done
	return nil
}
