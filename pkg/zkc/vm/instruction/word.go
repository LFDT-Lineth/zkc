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
package instruction

import (
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction/opcode"
	vm_word "github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Word captures the subset of all instructions which can be executed
// by a word machine.
type Word interface {
	Instruction
	// IsWord demarcates word instructions
	IsWord() bool
}

// vmWord is a convenient alias
type vmWord[W any] = vm_word.Word[W]

// UintAssign assigns the value of a given source register to a target register
// (whose types may vary).  However, a machine panic arises if the value
// assigned does not fit into the target register.
func UintAssign[W vmWord[W]](target register.Id, source register.Id) *WordTypeA[W] {
	var (
		zero    W
		sources = []register.Id{source}
	)
	//
	return NewWordTypeA(opcode.INT_ADD, register.NewVector(target), sources, zero)
}

// UintAssignV assigns the value of a given source register to a target vector
// (whose types may vary).  However, a machine panic arises if the value
// assigned does not fit into the target register.
func UintAssignV[W vmWord[W]](target register.Vector, source register.Id) *WordTypeA[W] {
	var (
		zero    W
		sources = []register.Id{source}
	)
	//
	return NewWordTypeA(opcode.INT_ADD, target, sources, zero)
}

// UintAdd computes the integer sum of the source registers plus a constant
// and writes the result into the target register.  Specifically, the value
// assigned is sources[0] + ... + sources[n-1] + constant, evaluated within the
// bit-width of the target register.  Overflow at runtime aborts execution with
// an arithmetic-overflow error.  The source slice may be empty, in which case
// the instruction simply loads the constant.
func UintAdd[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_ADD, register.NewVector(target), sources, constant)
}

// UintAddV computes the integer sum of the source registers plus a constant
// and writes the result into a target vector.  Specifically, the value
// assigned is sources[0] + ... + sources[n-1] + constant, evaluated within the
// bit-width of the target register.  Overflow at runtime aborts execution with
// an arithmetic-overflow error.  The source slice may be empty, in which case
// the instruction simply loads the constant.
func UintAddV[W vmWord[W]](target register.Vector, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_ADD, target, sources, constant)
}

// UintConst assigns a given constant a target register. A machine panic arises
// if the value assigned does not fit into the target register.
func UintConst[W vmWord[W]](target register.Id, source W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_ADD, register.NewVector(target), nil, source)
}

// UintDestruct constructs a new concatenation instruction which concatenates the
// source registers and writes them into the target register.  Observe that we
// have a little endian ordering here for the target registers.  That is, the
// value of the register targets[0] will be assigned the least significant bits of
// the source value
func UintDestruct[W vmWord[W]](targets register.Vector, source register.Id) *WordTypeA[W] {
	var zero W
	return UintAddV(targets, []register.Id{source}, zero)
}

// UintSub computes a chained subtraction of the source registers and a
// constant, assigning the result to the target register.  The value assigned is
// sources[0] - sources[1] - ... - sources[n-1] - constant, evaluated within the
// bit-width of the target register.  Underflow at runtime aborts execution with
// an arithmetic-underflow error.
func UintSub[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_SUB, register.NewVector(target), sources, constant)
}

// UintSubV computes a chained subtraction of the source registers and a
// constant, assigning the result to the target register.  The value assigned is
// sources[0] - sources[1] - ... - sources[n-1] - constant, evaluated within the
// bit-width of the target register.  Underflow at runtime aborts execution with
// an arithmetic-underflow error.
func UintSubV[W vmWord[W]](target register.Vector, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_SUB, target, sources, constant)
}

// UintMul computes the integer product of the source registers and a
// constant, assigning the result to the target register.  The value assigned
// is constant * sources[0] * ... * sources[n-1], evaluated within the
// bit-width of the target register.  Overflow at runtime aborts execution
// with an arithmetic-overflow error.
func UintMul[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_MUL, register.NewVector(target), sources, constant)
}

// UintMulV computes the integer product of the source registers and a constant,
// assigning the result to the target vector.  The value assigned is constant *
// sources[0] * ... * sources[n-1], evaluated within the bit-width of the target
// register.  Overflow at runtime aborts execution with an arithmetic-overflow
// error.
func UintMulV[W vmWord[W]](target register.Vector, sources []register.Id, constant W) *WordTypeA[W] {
	return NewWordTypeA(opcode.INT_MUL, target, sources, constant)
}

// UintDiv computes the (truncated) integer quotient of two source registers,
// assigning the result to the target register.  Specifically, sources[0] is
// the dividend and sources[1] is the divisor; division by zero aborts
// execution with a division-by-zero error.  The constant operand is unused.
func UintDiv[W vmWord[W]](target, dividend, divisor register.Id) *WordTypeB[W] {
	var zero W
	return NewWordTypeB(opcode.INT_DIV, target, []register.Id{dividend, divisor}, zero)
}

// UintRem computes the remainder of the integer division of two source
// registers, assigning the result to the target register.  Specifically,
// sources[0] is the dividend and sources[1] is the divisor; division by zero
// aborts execution with a division-by-zero error.  The constant operand is
// unused.
func UintRem[W vmWord[W]](target, dividend, divisor register.Id) *WordTypeB[W] {
	var zero W
	return NewWordTypeB(opcode.INT_REM, target, []register.Id{dividend, divisor}, zero)
}

// UintAddModP computes the sum of the source registers and a constant within
// the prime field of the surrounding machine, assigning the result to the
// target register.  The value assigned is sources[0] + ... + sources[n-1] +
// constant, reduced modulo the field's prime characteristic.  The source slice
// may be empty, in which case the instruction simply loads the constant.
func UintAddModP[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.INT_ADDMOD_P, target, sources, constant)
}

// UintSubModP computes a chained subtraction of the source registers and a
// constant within the prime field of the surrounding machine, assigning the
// result to the target register.  The value assigned is sources[0] - sources[1]
// - ... - sources[n-1] - constant, reduced modulo the field's prime
// characteristic.
func UintSubModP[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.INT_SUBMOD_P, target, sources, constant)
}

// UintMulModP computes the product of the source registers and a constant
// within the prime field of the surrounding machine, assigning the result to
// the target register.  The value assigned is constant * sources[0] * ... *
// sources[n-1], reduced modulo the field's prime characteristic.
func UintMulModP[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.INT_MULMOD_P, target, sources, constant)
}

// BitAnd computes the bitwise AND of the source registers and a constant,
// assigning the result to the target register.  The value assigned is constant
// & sources[0] & ... & sources[n-1].  Callers needing AND with no constant
// contribution should pass the AND identity (all-ones within the target
// bit-width) as the constant.
func BitAnd[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.BIT_AND, target, sources, constant)
}

// BitNot computes the bitwise complement of a single source register and
// assigns the result to the target register.  The complement is taken within
// the bit-width of the target register.  The constant operand is unused.
func BitNot[W vmWord[W]](target, source register.Id) *WordTypeB[W] {
	var zero W
	return NewWordTypeB(opcode.BIT_NOT, target, []register.Id{source}, zero)
}

// BitOr computes the bitwise OR of the source registers and a constant,
// assigning the result to the target register.  The value assigned is constant
// | sources[0] | ... | sources[n-1].
func BitOr[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.BIT_OR, target, sources, constant)
}

// BitXor computes the bitwise exclusive-OR of the source registers and a
// constant, assigning the result to the target register.  The value assigned is
// constant ^ sources[0] ^ ... ^ sources[n-1].
func BitXor[W vmWord[W]](target register.Id, sources []register.Id, constant W) *WordTypeB[W] {
	return NewWordTypeB(opcode.BIT_XOR, target, sources, constant)
}

// BitShl computes the bitwise left-shift of one source register by another,
// assigning the result to the target register.  Specifically, sources[0] is the
// value to be shifted and sources[1] is the shift amount, with the result
// evaluated within the bit-width of the target register.  The constant operand
// is unused.
func BitShl[W vmWord[W]](target, value, amount register.Id) *WordTypeB[W] {
	var zero W
	return NewWordTypeB(opcode.BIT_SHL, target, []register.Id{value, amount}, zero)
}

// BitShr computes the bitwise (logical) right-shift of one source register
// by another, assigning the result to the target register.  Specifically,
// sources[0] is the value to be shifted and sources[1] is the shift amount. The
// constant operand is unused.
func BitShr[W vmWord[W]](target, value, amount register.Id) *WordTypeB[W] {
	var zero W
	return NewWordTypeB(opcode.BIT_SHR, target, []register.Id{value, amount}, zero)
}

// BitConcat constructs a new concatenation instruction which concatenates
// the source registers and writes them into the target register.  Observe
// that we have a little endian ordering here for the source registers.  That
// is, the value of the register sources[0] will occupy the least significant
// bits of the result.
func BitConcat[W vmWord[W]](target register.Id, sources []register.Id) *WordTypeA[W] {
	var zero W
	return NewWordTypeA(opcode.BIT_CONCAT, register.NewVector(target), sources, zero)
}

// BitConcatV constructs a new concatenation instruction which concatenates the
// source registers and writes them into the target vector.  Observe that we
// have a little endian ordering here for the source registers.  That is, the
// value of the register sources[0] will occupy the least significant bits of
// the result.
func BitConcatV[W vmWord[W]](target register.Vector, sources []register.Id) *WordTypeA[W] {
	var zero W
	return NewWordTypeA(opcode.BIT_CONCAT, target, sources, zero)
}
