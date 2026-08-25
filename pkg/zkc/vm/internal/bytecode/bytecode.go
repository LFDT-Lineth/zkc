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
	"encoding/gob"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Condition represents the set of permission comparitors for a SkipIf
// instruction.
type Condition uint

const (
	// CONDITION_EQ indicates an equality condition
	CONDITION_EQ Condition = 0
	// CONDITION_NEQ indicates a non-equality condition
	CONDITION_NEQ Condition = 1
	// CONDITION_LT indicates a less-than condition
	CONDITION_LT Condition = 2
	// CONDITION_GT indicates a greater-than condition
	CONDITION_GT Condition = 3
	// CONDITION_LTEQ indicates a less-than-or-equals condition
	CONDITION_LTEQ Condition = 4
	// CONDITION_GTEQ indicates a greater-than-or-equals condition
	CONDITION_GTEQ Condition = 5
)

// RegisterId just provides a convenient alias to make the code more readable.
type RegisterId = uint16

// ModuleId represents module identifiers
type ModuleId = uint16

// Address just provides a convenient alias to make the code more readable.
type Address = uint32

// FieldConfig provides a convenient alias for the field configuration passed to
// Bytecode.Validate (mirroring the field.Config argument of
// instruction.Instruction.MicroValidate).  Aliasing it here keeps the per-
// instance Validate signatures free of an otherwise package-wide import.
type FieldConfig = field.Config

// Operation identifies an operation performed by a bytecode instruction: an
// arithmetic operation (ADD, SUB, MUL), a bitwise operation (AND, OR, XOR, NOT,
// SHL, SHR), a field operation (ADDMOD_P, SUBMOD_P, MULMOD_P) or a hint
// operation (DIV_HINT, WIDE_SHL, WIDE_SHR, WIDE_DIVMOD).
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
	// DIV_HINT is the hint operation which computes the quotient, remainder
	// and range witness for a division hint (see Intrinsic).
	DIV_HINT
	// WIDE_SHL is the hint operation which computes a logical shift left of a
	// (possibly multi-limb) value by a given amount, mirroring the Bitwise SHL
	// instruction but operating over vectored operands (see Intrinsic).
	WIDE_SHL
	// WIDE_SHR is the hint operation which computes a logical shift right of a
	// (possibly multi-limb) value by a given amount, mirroring the Bitwise SHR
	// instruction but operating over vectored operands (see Intrinsic).
	WIDE_SHR
	// WIDE_DIVMOD is the hint operation which computes both the quotient and
	// the remainder of a (possibly multi-limb) dividend and divisor, mirroring
	// the DIVMOD instruction but operating over vectored operands (see
	// Intrinsic).
	WIDE_DIVMOD
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
	// found (or nil when it is well-formed).  Field is the surrounding field
	// configuration and env resolves register and module information.
	Validate(field FieldConfig, env Environment[W]) []error
	// String returns a suitable string representation of this bytecode.
	String(Environment[W]) string
}

// Environment provides a mechanism to allow Bytecode functions access to
// information about the enclosing environment.  For example, to generate a
// suitable string for a given instruction, it is useful to know the names of
// registers in the enclosing function, etc.
type Environment[W word.Word[W]] interface {
	// Name returns the name of the enclosing function.
	Name() string
	// HasModule checks whether a module with the given name exists and, if so,
	// returns its module identifier. Otherwise, it returns none.
	HasModule(name string) util.Option[ModuleId]
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns none.
	HasRegister(name string) util.Option[RegisterId]
	// Module returns information about the given module, or none if it does not
	// exist.
	Module(id ModuleId) util.Option[ModuleInfo]
	// Register returns the ith register used in this module.
	Register(id RegisterId) RegisterInfo
	// RegisterCount returns the number of registers in the enclosing module.
	RegisterCount() uint
	// VectorCount returns the number of vectors in the enclosing function.
	VectorCount() uint
	// ValueOf optionally returns the current value held in the given register.
	// This is used (for example) by the debugger to render register values
	// inline within an instruction's string representation.  Environments which
	// have no notion of a "current value" (i.e. those used outside of a concrete
	// execution context) return None.
	ValueOf(id RegisterId) util.Option[W]
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
	// Name returns the name of this module.
	Name() string
	// IsFunction indicates whether this module can be called.
	IsFunction() bool
	// HasUnsafeArgs indicates whether a function accepts maybe-undefined arguments.
	HasUnsafeArgs() bool
	// IsMemory indicates whether this module supports memory accesses.
	IsMemory() bool
	// IsReadOnly indicates whether this module forbids writes.
	IsReadOnly() bool
	// IsWriteOnly indicates whether this module forbids reads.
	IsWriteOnly() bool
	// NumInputs returns the number of input registers in this module.
	NumInputs() uint
	// NumOutputs returns the number of output registers in this module.
	NumOutputs() uint
	// Width returns the total number of registers in this module.
	Width() uint
}

// validateOperands ensures that every register operand exists in the enclosing
// module. Repeated operands are reported at most once.
func validateOperands[W word.Word[W]](env Environment[W], operands ...[]RegisterId) []error {
	var (
		errors []error
		seen   = make(map[RegisterId]bool)
	)

	for _, group := range operands {
		for _, id := range group {
			if seen[id] {
				continue
			}

			seen[id] = true
			if uint(id) >= env.RegisterCount() {
				errors = append(errors, fmt.Errorf("register %d does not exist", id))
			}
		}
	}

	return errors
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
	//
	util.Assert(len(targets) > 0, "atleast one target required")
	//
	return NewArith(OP_ADD, targets, sources, zero)
}

// AddVecConst constructs a vectored addition instruction computing
// "targets = sum(sources) + constant", where targets is a multi-limb register
// vector.
func AddVecConst[W word.Word[W]](targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return NewArith(OP_ADD, targets, sources, constant)
}

// Assign constructs a move instruction which copies the source register into the
// target register.
func Assign[W word.Word[W]](target RegisterId, source RegisterId) *Arith[W] {
	var zero W
	//
	return NewArith(OP_ADD, []RegisterId{target}, []RegisterId{source}, zero)
}

// AssignV constructs a concatenation instruction which joins the source
// registers into the target register vector.
func AssignV[W word.Word[W]](targets []RegisterId, sources ...RegisterId) Bytecode[W] {
	util.Assert(len(targets) > 0, "at least one target required")
	util.Assert(len(sources) > 0, "at least one source required")
	// Avoid (expensive) concat instruction
	if len(targets) == 1 && len(sources) == 1 {
		return Assign[W](targets[0], sources[0])
	}
	//
	return &Cat[W]{Targets: targets, Sources: sources}
}

// CallFun constructs a function-call bytecode with the given flags.
func CallFun[W word.Word[W]](target ModuleId, args []RegisterId, returns []RegisterId) *Call[W] {
	return &Call[W]{target, args, returns}
}

// Jump creates an unconditional jump instruction transferring control to the
// given target address.
func Jump[W word.Word[W]](target Address) *Jmp[W] {
	return &Jmp[W]{Target: target}
}

// NewSkip constructs an uncondition skip instruction which skips over n
// instructions.
func NewSkip[W word.Word[W]](skip uint16) *Skip[W] {
	return &Skip[W]{Skip: skip}
}

// NewSkipIf constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing single registers.
func NewSkipIf[W word.Word[W]](op Condition, skip uint16, left RegisterVector, right Operand[W]) *SkipIf[W] {
	return &SkipIf[W]{Skip: skip, Left: left, Right: right, Op: op}
}

// LoadConst constructs a load-constant (LDC) instruction which assigns the
// given constant to the target register.
// TODO: constant register, see: https://github.com/LFDT-Lineth/zkc/issues/1838
func LoadConst[W word.Word[W]](target RegisterId, constant W) *Arith[W] {
	return NewArith(OP_ADD, []RegisterId{target}, nil, constant)
}

// LoadConstVec constructs a load-constant (LDC) instruction which assigns the
// given constant to the target registers.
func LoadConstVec[W word.Word[W]](targets []RegisterId, constant W) *Arith[W] {
	util.Assert(len(targets) > 0, "atleast one target required")
	//
	return NewArith(OP_ADD, targets, nil, constant)
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
	util.Assert(len(targets) > 0, "atleast one target required")
	//
	if len(sources) == 0 {
		// Bypass multiplication
		return NewArith(OP_ADD, targets, nil, constant)
	}
	//
	return NewArith(OP_MUL, targets, sources, constant)
}

// NewMemRead constructs a memory-read instruction.  The data registers receive
// the row located at the address given by the address registers, in the memory
// identified by id.  The kind of memory being read (ROM, static ROM, RAM, paged
// RAM) is not recorded here: it is resolved from the environment when the
// instruction is encoded.  An optional stamp operand carries the access's
// timestamp (present only after timestamp threading).
func NewMemRead[W word.Word[W]](id uint16, address []RegisterId, data []RegisterId,
	stamp ...[]RegisterId) *ReadWrite[W] {
	return &ReadWrite[W]{Write: false, Id: id, Address: address, Data: data, Stamp: singleStamp(stamp)}
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
	util.Assert(len(targets) > 0, "atleast one target required")
	//
	return NewArith(OP_SUB, targets, sources, constant)
}

// NewMemWrite constructs a memory-write instruction.  The data registers are
// written to the row located at the address given by the address registers, in
// the memory identified by id.  The kind of memory being written (write-once,
// RAM, paged RAM) is not recorded here: it is resolved from the environment when
// the instruction is encoded.  An optional stamp operand carries the access's
// timestamp (present only after timestamp threading).
func NewMemWrite[W word.Word[W]](id uint16, address []RegisterId, data []RegisterId,
	stamp ...[]RegisterId) *ReadWrite[W] {
	return &ReadWrite[W]{Write: true, Id: id, Address: address, Data: data, Stamp: singleStamp(stamp)}
}

// singleStamp unwraps the optional variadic stamp operand of NewMemRead /
// NewMemWrite, accepting either no stamp or exactly one.  The variadic
// [][]RegisterId shape only encodes optionality (a Go optional-argument
// idiom); it does NOT limit the stamp to one lane — the single accepted
// operand is itself a register vector, whose lanes are the stamp's limbs
// after register splitting.
func singleStamp(stamp [][]RegisterId) []RegisterId {
	switch len(stamp) {
	case 0:
		return nil
	case 1:
		return stamp[0]
	default:
		panic("at most one stamp operand allowed")
	}
}

// NewBitwise constructs a bitwise instruction (and/or/xor) computing
// "target = left op right" for a register or (AND/OR/XOR only) constant right
// operand.
func NewBitwise[W word.Word[W]](op Operation, target, left RegisterId, right Operand[W], bitwidth uint16) *Bitwise[W] {
	return &Bitwise[W]{Op: op, Target: target, Left: left, Right: right, Bitwidth: bitwidth}
}

// NewCheckCast constructs a check-cast instruction asserting that the given
// target register fits within the given bit width.
func NewCheckCast[W word.Word[W]](target RegisterId, bitwidth uint16) *CheckCast[W] {
	//
	return &CheckCast[W]{Bitwidth: bitwidth, Target: target}
}

// NewDebug constructs a debug instruction carrying the given formatted message.
func NewDebug[W word.Word[W]](chunks []FormattedChunk, sources []RegisterId) *Debug[W] {
	return &Debug[W]{chunks, array.Map(sources, func(_ uint, id RegisterId) RegisterVector {
		return NewRegisterVector(id)
	})}
}

// NewDivRem constructs a division/remainder instruction computing both the
// quotient and the remainder of "dividend / divisor" for a register or
// constant divisor.
func NewDivRem[W word.Word[W]](quotient, remainder, dividend RegisterId, divisor Operand[W]) *DivRem[W] {
	return &DivRem[W]{Quotient: quotient, Remainder: remainder, Dividend: dividend, Divisor: divisor}
}

// NewFail constructs a fail instruction carrying the given formatted message.
func NewFail[W word.Word[W]](chunks []FormattedChunk, sources []RegisterId) *Fail[W] {
	return &Fail[W]{chunks, array.Map(sources, func(_ uint, id RegisterId) RegisterVector {
		return NewRegisterVector(id)
	})}
}

// NewFieldArith constructs a field arithmetic instruction computing
// "target = sources[0] op ... op constant" modulo the field prime.
func NewFieldArith[W word.Word[W]](op Operation, target RegisterId, sources []RegisterId, constant W) *FieldArith[W] {
	return &FieldArith[W]{Op: op, Target: target, Sources: sources, Constant: constant}
}

// NewRet constructs a return instruction with the given frame width and return
// offset.
func NewRet[W word.Word[W]]() *Ret[W] {
	return &Ret[W]{}
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

// RegisterGobTypes registers every concrete Bytecode[W] implementation with the
// gob package for the given word type W.  This is required so that the Bytecode
// interface values held within a Vector can be marshalled / unmarshalled.  gob
// registration is keyed on the concrete type, so registering the same
// instantiation more than once is harmless (and registering distinct word types
// yields distinct names, hence no conflict).
func RegisterGobTypes[W word.Word[W]]() {
	gob.Register(&Arith[W]{})
	gob.Register(&Bitwise[W]{})
	gob.Register(&Call[W]{})
	gob.Register(&Cat[W]{})
	gob.Register(&CheckCast[W]{})
	gob.Register(&Debug[W]{})
	gob.Register(&DivRem[W]{})
	gob.Register(&Fail[W]{})
	gob.Register(&FieldArith[W]{})
	gob.Register(&UintToField[W]{})
	gob.Register(&FieldToUint[W]{})
	gob.Register(&Intrinsic[W]{})
	gob.Register(&Jmp[W]{})
	gob.Register(&ReadWrite[W]{})
	gob.Register(&Ret[W]{})
	gob.Register(&Skip[W]{})
	gob.Register(&SkipIf[W]{})
	gob.Register(&Switch[W]{})
	gob.Register(&Dispatch[W]{})
	gob.Register(&Operand[W]{})
}
