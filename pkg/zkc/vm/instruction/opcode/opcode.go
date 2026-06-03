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
package opcode

// OpCode identifies an instruction exactly
type OpCode uint8

const (
	// ========================================================================
	// Base Instructions
	// ========================================================================

	// CALL instruction
	CALL OpCode = iota
	// FAIL instruction
	FAIL
	// JUMP (unconditional) instruction
	JUMP
	// MEMORY_READ instruction
	MEMORY_READ
	// MEMORY_WRITE instruction
	MEMORY_WRITE
	// SKIP_IF instruction
	SKIP_IF
	// SKIP_IFC instruction
	SKIP_IFC
	// SKIP instruction
	SKIP
	// RETURN instruction
	RETURN
	// DEBUG instruction
	DEBUG

	// ========================================================================
	// Word Instructions
	// ========================================================================

	// INT_ADD instruction
	INT_ADD
	// INT_SUB instruction
	INT_SUB
	// INT_MUL instruction
	INT_MUL
	// INT_DIV instruction
	INT_DIV
	// INT_REM instruction
	INT_REM
	// INT_ADDMOD_P instruction
	INT_ADDMOD_P
	// INT_SUBMOD_P instruction
	INT_SUBMOD_P
	// INT_MULMOD_P instruction
	INT_MULMOD_P
	// BIT_AND instruction
	BIT_AND
	// BIT_OR instruction
	BIT_OR
	// BIT_XOR instruction
	BIT_XOR
	// BIT_NOT instruction
	BIT_NOT
	// BIT_SHL (shift left) instruction
	BIT_SHL
	// BIT_SHR (shift right) instruction
	BIT_SHR
	// BIT_CONCAT (concatenation) instruction
	BIT_CONCAT

	// ========================================================================
	// Field Instructions
	// ========================================================================

	// FIELD_ASSIGN represents a field assignment instruction.
	FIELD_ASSIGN

	// ========================================================================
	// Hint Instructions
	// ========================================================================

	// HINT_DIVISION represents a non-deterministic register assignment with no
	// polynomial constraint.
	HINT_DIVISION
)

// ARITH_OPCODES captures those opcodes that are known as "arithmetic operations"
var ARITH_OPCODES = []OpCode{
	INT_ADD,
	INT_SUB,
	INT_MUL,
	BIT_CONCAT,
}

// TYPE_B_OPCODES captures those opcodes that are known as "bitwise operations"
var TYPE_B_OPCODES = []OpCode{
	INT_DIV,
	INT_REM,
	BIT_AND,
	BIT_OR,
	BIT_XOR,
	BIT_NOT,
	BIT_SHL,
	BIT_SHR,
}

// TYPE_F_OPCODES captures those opcodes that are known as "field operations"
var TYPE_F_OPCODES = []OpCode{
	INT_ADDMOD_P,
	INT_SUBMOD_P,
	INT_MULMOD_P,
}
