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
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// UNKNOWN_BUS signals a bus which is unknown.
const UNKNOWN_BUS = math.MaxUint

// Map represents the interface between the different components of a program.
// It provides a way to send messages to and from components.  The core
// abstraction of a bus are the "address lines" and the "data lines".  For
// example, to read from a bus we set the desired address on the address lines
// and then read off the result from the data lines.  Likewise, to write to a
// bus, we set both the address and data lines, etc.  This mechanism abstracts
// the various I/O peripherals found within a program, such as functions, Random
// Access Memory, etc.
type Map interface {
	// Read a set of n values at a given address on a bus.  The exact meaning of
	// this depends upon the I/O peripheral connected to the bus.  For example,
	// if its a function then the function is executed with the given address as
	// its arguments, producing some number of outputs.  Likewise, if its a
	// memory, then this will return the current value stored in that address,
	// etc.  Finally, the number of expected values must match that provided by
	// the underlying component.
	Read(bus uint, address []big.Int, n uint) []big.Int
	// Write a set of values to a given address on a bus.  This only makes sense
	// for writeable memory, such Random Access Memory (RAM).  In contrast,
	// functions and Read-Only Memory (ROM) are not considered writeable.
	Write(bus uint, address []big.Int, values []big.Int)
}

// Instruction provides an abstract notion of an executable "machine instruction".
type Instruction interface {
	// Execute this instruction with the given local and global state.  The next
	// program counter position is returned, or io.RETURN if the enclosing
	// function has terminated (i.e. because a return instruction was
	// encountered).
	Execute(state State) uint
	// Registers returns the set of registers read this micro instruction.
	RegistersRead() []RegisterId
	// Registers returns the set of registers written by this micro instruction.
	RegistersWritten() []RegisterId
	// Validate that this instruction is well-formed.  For example, that it is
	// balanced, that there are no conflicting writes, that all temporaries have
	// been allocated, etc.  The maximum bit capacity of the underlying field is
	// needed for this calculation, so as to allow an instruction to check it
	// does not overflow the underlying field.
	Validate(fieldWidth uint, fn register.Map) error
	// Produce a suitable string representation of this instruction.  This is
	// primarily used for debugging.
	String(fn register.Map) string
}

// InOutInstruction is simply a kind of instruction which performs some kind of I/O
// operation via a bus.
type InOutInstruction interface {
	// Bus returns information about the bus.  Observe that prior to Link being
	// called, this will return an unlinked bus.
	Bus() Bus
}
