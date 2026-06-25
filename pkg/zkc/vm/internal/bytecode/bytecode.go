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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Cond provides a convenient alias to make the code more readable.
type Cond = opcode.Condition

// RegisterId just provides a convenient alias to make the code more readable.
type RegisterId = uint16

// ModuleId represents module identifiers
type ModuleId = uint16

// Address just provides a convenient alias to make the code more readable.
type Address = uint32

// Module provides a convenient alias to make the code more readable.
type Module = base.Module

// FieldConfig provides a convenient alias for the field configuration passed to
// Bytecode.Validate (mirroring the field.Config argument of
// instruction.Instruction.MicroValidate).  Aliasing it here keeps the per-
// instance Validate signatures free of an otherwise package-wide import.
type FieldConfig = field.Config

// Operation identifies a bitwise operation (AND, OR, XOR, NOT, SHL or SHR).
type Operation uint8

// Symbol returns a suitable string representation of this operator.
func (p Operation) Symbol() string {
	switch p {
	case OP_ADD:
		return "+"
	case OP_SUB:
		return "-"
	case OP_MUL:
		return "*"
	case OP_AND:
		return "&"
	case OP_OR:
		return "|"
	case OP_XOR:
		return "^"
	case OP_ADDMOD_P:
		return "⊕"
	case OP_SUBMOD_P:
		return "⊖"
	case OP_MULMOD_P:
		return "⊗"
	default:
		panic("unknown operation")
	}
}

// Prefix returns a suitable "prefix" string for this operator.
func (p Operation) Prefix() string {
	switch p {
	case OP_ADD:
		return "add"
	case OP_SUB:
		return "sub"
	case OP_MUL:
		return "mul"
	case OP_AND:
		return "and"
	case OP_OR:
		return "or"
	case OP_XOR:
		return "xor"
	case OP_ADDMOD_P:
		return "fadd"
	case OP_SUBMOD_P:
		return "fsub"
	case OP_MULMOD_P:
		return "fmul"
	default:
		panic("unknown operation")
	}
}

const (
	// OP_ADD integer addition
	OP_ADD Operation = iota
	// OP_SUB integer subtraction
	OP_SUB
	// OP_MUL integer multiplication
	OP_MUL
	// OP_AND bitwise conjunction.
	OP_AND
	// OP_OR bitwise disjunction.
	OP_OR
	// OP_XOR bitwise exclusive-or.
	OP_XOR
	// OP_NOT bitwise negation.
	OP_NOT
	// OP_SHL logical shift left.
	OP_SHL
	// OP_SHR logical shift right.
	OP_SHR
	// OP_ADDMOD_P represents addition modulus the prime P
	OP_ADDMOD_P
	// OP_SUBMOD_P represents subtraction modulus the prime P
	OP_SUBMOD_P
	// OP_MULMOD_P represents multiplication modulus the prime P
	OP_MULMOD_P
)

// ============================================================================
// Interfaces
// ============================================================================

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W word.Word[W]] interface {
	// Uses returns the set of registers used (i.e. read) by this bytecode.
	Uses() []RegisterId
	// Definitions returns the set of registers defined (i.e. written) by this
	// bytecode.
	Definitions() []RegisterId
	// Validate checks that this bytecode is well-formed, returning any errors
	// found (or nil when it is well-formed).  Here, width is the number of
	// bytecodes in the enclosing vector, field is the surrounding field
	// configuration and env resolves register information.
	Validate(width uint, field FieldConfig, env Environment) []error
	// String returns a suitable string representation of this bytecode.
	String(Environment) string
}

// Environment provides a mechanism to allow Bytecode functions access to
// information about the enclosing environment.  For example, to generate a
// suitable string for a given instruction, it is useful to know the names of
// registers in the enclosing function, etc.
type Environment interface {
	// Name returns the name of the enclosing function.
	Name() string
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns false.
	HasModule(name string) (RegisterId, bool)
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns false.
	HasRegister(name string) (RegisterId, bool)
	// Register returns the ith register used in this module.
	Module(id ModuleId) ModuleInfo
	// Register returns the ith register used in this module.
	Register(id RegisterId) RegisterInfo
}

// RegisterInfo provides a minimal amount of information about a register in the
// enclosing function.
type RegisterInfo interface {
	// Name returns the  name of this register
	Name() string
	// Bitwidth returns the bitwidth of this register, or the empty option for a
	// native register (which has no fixed bitwidth).  Used by Bytecode.Validate
	// to detect width overflows.
	Bitwidth() util.Option[uint]
}

// ModuleInfo provides a minimal amount of information about a module in the
// enclosing environment.
type ModuleInfo interface {
	// Name returns the  name of this register
	Name() string
}

// ============================================================================
// Constructors
// ============================================================================

// The constructors below provide a more readable way to build bytecode
// instructions than instantiating the underlying instruction structs directly.
// Several of them are thin wrappers around the general-purpose Arith
// instruction, which computes "target = source[0] op source[1] op ... op
// constant" for some Arithmetic operation op (add, subtract or multiply).  The
// "Vec" variants accept a slice of target registers, allowing a single logical
// value to be spread across multiple register limbs (e.g. when a value is wider
// than the underlying word type W).

// AddConst constructs an addition instruction computing
// "target = sum(sources) + constant" into a single target register.
func AddConst[W word.Word[W]](target RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_ADD, []RegisterId{target}, sources, constant)
}

// AddVec constructs a vectored addition instruction computing
// "targets = sum(sources)" (i.e. with no constant addend), where targets is a
// multi-limb register vector.
func AddVec[W word.Word[W]](targets []RegisterId, sources []RegisterId) *Arith[W] {
	var zero W
	return NewArith(OP_ADD, targets, sources, zero)
}

// AddVecConst constructs a vectored addition instruction computing
// "targets = sum(sources) + constant", where targets is a multi-limb register
// vector.
func AddVecConst[W word.Word[W]](targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_ADD, targets, sources, constant)
}

// CallFun constructs a function-call bytecode with the given flags.
func CallFun(target ModuleId, flags CallFlags, args []RegisterId, returns []RegisterId) *Call {
	return &Call{target, flags, args, returns}
}

// Jump creates an unconditional jump instruction transferring control to the
// given target address.
func Jump(target Address) *Jmp {
	return &Jmp{Target: target}
}

// NewSkip constructs an uncondition skip instruction which skips over n
// instructions.
func NewSkip(skip uint16) *Skip {
	return &Skip{Skip: skip}
}

// NewSkipIf constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing single registers.
func NewSkipIf(op Cond, skip uint16, left, right RegisterId) *SkipIf {
	return &SkipIf{Skip: skip, Left: NewRegVec(left), Right: NewRegVec(right), Op: op}
}

// NewSkipIfVec constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing multi-limb register
// vectors.
func NewSkipIfVec(op Cond, skip uint16, left, right register.Vector) *SkipIf {
	return &SkipIf{Skip: skip, Left: NewRegVec(asRegs(left.Registers()...)...),
		Right: NewRegVec(asRegs(right.Registers()...)...), Op: op}
}

// LoadConst constructs a load-constant (LDC) instruction which assigns the
// given constant to the target register.
func LoadConst[W word.Word[W]](target RegisterId, constant W) *Arith[W] {
	return NewArith(OP_ADD, []RegisterId{target}, nil, constant)
}

// Move constructs a move instruction which copies the source register into the
// target register.
func Move[W word.Word[W]](target RegisterId, source RegisterId) *Arith[W] {
	var zero W
	return NewArith(OP_ADD, []RegisterId{target}, []RegisterId{source}, zero)
}

// MultiwaySkip constructs a multiway-skip (SMW) instruction which dispatches on
// the value of the source register against the given (value, target) table.
// Targets are label indices until resolved during encoding (see Smw.Patch).
func MultiwaySkip[W word.Word[W]](source RegisterId, cases []SwitchCase[W]) *Switch[W] {
	return &Switch[W]{Source: source, Cases: cases}
}

// MulConst constructs a multiplication instruction computing
// "target = product(sources) * constant" into a single target register.
func MulConst[W word.Word[W]](target RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_MUL, []RegisterId{target}, sources, constant)
}

// MulVecConst constructs a vectored multiplication instruction computing
// "targets = product(sources) * constant", where targets is a multi-limb
// register vector.
func MulVecConst[W word.Word[W]](targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_MUL, targets, sources, constant)
}

// NewMemRead constructs a memory-read instruction.  The data registers receive
// the row located at the address given by the address registers, in the memory
// identified by id.  The kind of memory being read (ROM, static ROM, RAM, paged
// RAM) is not recorded here: it is resolved from the environment when the
// instruction is encoded.
func NewMemRead(id uint16, address []RegisterId, data []RegisterId) *ReadWrite {
	return &ReadWrite{Write: false, Id: id, Address: address, Data: data}
}

// SubConst constructs a subtraction instruction computing
// "target = sources[0] - ... - constant" into a single target register.
func SubConst[W word.Word[W]](target RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_SUB, []RegisterId{target}, sources, constant)
}

// SubVecConst constructs a vectored subtraction instruction computing
// "targets = sources[0] - ... - constant", where targets is a multi-limb
// register vector.
func SubVecConst[W word.Word[W]](targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_SUB, targets, sources, constant)
}

// NewMemWrite constructs a memory-write instruction.  The data registers are
// written to the row located at the address given by the address registers, in
// the memory identified by id.  The kind of memory being written (write-once,
// RAM, paged RAM) is not recorded here: it is resolved from the environment when
// the instruction is encoded.
func NewMemWrite(id uint16, address []RegisterId, data []RegisterId) *ReadWrite {
	return &ReadWrite{Write: true, Id: id, Address: address, Data: data}
}

// NewBitwise constructs a bitwise instruction (and/or/xor) computing
// "target = left op right".
func NewBitwise(op Operation, target, left, right RegisterId, bitwidth uint16) *Bitwise {
	return &Bitwise{Op: op, Target: target, Left: left, Right: right, Bitwidth: bitwidth}
}

// NewCheckCast constructs a check-cast instruction asserting that the given
// target register fits within the given bit width.
func NewCheckCast(target RegisterId, bitwidth uint16) *CheckCast {
	//
	return &CheckCast{Bitwidth: bitwidth, Target: target}
}

// NewDebug constructs a debug instruction carrying the given formatted message.
func NewDebug(chunks []base.FormattedChunk) *Debug {
	var (
		hunks   = make([]FormattedChunk, len(chunks))
		sources []RegVec
	)
	//
	for i, c := range chunks {
		var args = asRegs(c.Argument.Registers()...)
		//
		hunks[i] = FormattedChunk{c.Text, c.Format}
		//
		if len(args) > 0 {
			sources = append(sources, NewRegVec(args...))
		}
	}
	//
	return &Debug{hunks, sources}
}

// NewDivHint constructs a division-hint instruction.
func NewDivHint(quotient, remainder, witness, dividend, divisor RegisterId) *DivHint {
	return &DivHint{Quotient: quotient, Remainder: remainder, Witness: witness,
		Dividend: dividend, Divisor: divisor}
}

// NewDivRem constructs a division/remainder instruction computing
// "target = dividend op divisor".
func NewDivRem(op uint32, target, dividend, divisor RegisterId) *DivRem {
	return &DivRem{Opcode: op, Target: target, Dividend: dividend, Divisor: divisor}
}

// NewFail constructs a fail instruction carrying the given formatted message.
func NewFail(chunks []base.FormattedChunk) *Fail {
	var (
		hunks   = make([]FormattedChunk, len(chunks))
		sources []RegVec
	)
	//
	for i, c := range chunks {
		var args = asRegs(c.Argument.Registers()...)
		//
		hunks[i] = FormattedChunk{c.Text, c.Format}
		//
		if len(args) > 0 {
			sources = append(sources, NewRegVec(args...))
		}
	}
	//
	return &Fail{hunks, sources}
}

// NewFieldArith constructs a field arithmetic instruction computing
// "target = sources[0] op ... op constant" modulo the field prime.
func NewFieldArith[W word.Word[W]](op Operation, target RegisterId, sources []RegisterId, constant W) *FieldArith[W] {
	return &FieldArith[W]{Op: op, Target: target, Sources: sources, Constant: constant}
}

// NewRet constructs a return instruction with the given frame width and return
// offset.
func NewRet() *Ret {
	return &Ret{}
}

// Concat constructs a concatenation instruction which joins the source
// registers into the target register vector.
func Concat(targets []RegisterId, sources []RegisterId) *Cat {
	return &Cat{Targets: targets, Sources: sources}
}

func asReg(rid register.Id) RegisterId {
	return util.Cast[uint16](rid.Unwrap())
}

func asRegs(rids ...register.Id) []RegisterId {
	return array.Map(rids, func(_ uint, r register.Id) RegisterId {
		return asReg(r)
	})
}

// IsUnusedConstant checks whether a given constant is the "identity element".
// This depends on the arithmetic operation in question.  For example, for
// addition and subtraction, this is zero.  But, for multiplication it is one.
func IsUnusedConstant[W word.Word[W]](op Operation, constant W) bool {
	switch op {
	case OP_ADD, OP_ADDMOD_P:
		return constant.Cmp64(0) == 0
	case OP_SUB, OP_SUBMOD_P:
		return constant.Cmp64(0) == 0
	case OP_MUL, OP_MULMOD_P:
		return constant.Cmp64(1) == 0
	default:
		panic("unknown arithmetic operation")
	}
}
