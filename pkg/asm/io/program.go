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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Program encapsulates one of more functions together, such that one may call
// another, etc.  Furthermore, it provides an interface between assembly
// components and the notion of a Schema.
type Program[T Instruction] struct {
	functions []Component[T]
}

// NewProgram constructs a new program using a given level of instruction.
func NewProgram[T Instruction](components []Component[T]) Program[T] {
	//
	fns := make([]Component[T], len(components))
	copy(fns, components)

	return Program[T]{fns}
}

// Component returns the ith entity in this program.
func (p *Program[T]) Component(id uint) Component[T] {
	return p.functions[id]
}

// Components returns all functions making up this program.
func (p *Program[T]) Components() []Component[T] {
	return p.functions
}

// InferPadding attempts to infer suitable padding values for a function, based
// on those padding values provided for its inputs (which default to 0).  In
// essence, this constructs a witness for the function in question.
func InferPadding[T Instruction](fn Component[T], executor *Executor[T]) {
	switch fn := fn.(type) {
	case *Function[T]:
		inferFunctionPadding(fn, executor)
	default:
		panic("unknown component")
	}
}

func inferFunctionPadding[T Instruction](fn *Function[T], executor *Executor[T]) {
	//
	if fn.IsAtomic() {
		// Only infer padding for one-line functions.
		var (
			insn      = fn.CodeAt(0)
			registers = fn.Registers()
			state     = initialState(registers, fn.Buses(), executor)
		)
		// Execute the one instruction
		_ = insn.Execute(state)
		// Assign padding values
		for i := range registers {
			var (
				val big.Int
				rid = register.NewId(uint(i))
			)
			// Load ith register value
			val.Set(state.Load(rid))
			// Update padding value
			registers[i].SetPadding(&val)
		}
	}
}

// Construct initial state from the given padding values.
func initialState(registers []Register, buses []Bus, iomap Map) State {
	var (
		state = make([]big.Int, len(registers))
		index = 0
	)
	// Initialise arguments
	for i, reg := range registers {
		if reg.IsInput() || reg.IsConst() {
			var ith big.Int
			// Clone big int.
			ith.SetBytes(reg.Padding().Bytes())
			// Assign to ith register
			state[i] = ith
			index = index + 1
		}
	}
	// Initialie I/O buses
	for _, bus := range buses {
		// Initialise address lines from padding
		for _, rid := range bus.Address() {
			state[rid.Unwrap()] = *registers[rid.Unwrap()].Padding()
		}
		// Initialise data lines from padding
		for _, rid := range bus.Data() {
			state[rid.Unwrap()] = *registers[rid.Unwrap()].Padding()
		}
	}
	//
	return NewState(state, registers, iomap)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (p *Program[T]) GobEncode() (data []byte, err error) {
	var buffer bytes.Buffer
	//
	gobEncoder := gob.NewEncoder(&buffer)
	// Left modules
	if err := gobEncoder.Encode(p.functions); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *Program[T]) GobDecode(data []byte) error {
	buffer := bytes.NewBuffer(data)
	gobDecoder := gob.NewDecoder(buffer)
	// Left modules
	if err := gobDecoder.Decode(&p.functions); err != nil {
		return err
	}
	// Success!
	return nil
}
