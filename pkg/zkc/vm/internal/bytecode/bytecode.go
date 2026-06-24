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

// ============================================================================
// Interfaces
// ============================================================================

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W word.Word[W]] interface {
	// Clone returns a deep copy of this bytecode, sharing no mutable state (in
	// particular, no operand slices) with the original.  See
	// Program.AddCheckPoint.
	Clone() Patched
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

// Patchable bytecodes contain a branch target which must be resolved during
// encoding.  Until resolved, their encoded width is unknown (it can depend on
// the target), hence MaxWidth provides a conservative bound.
type Patchable[W word.Word[W]] interface {
	Bytecode[W]
	// Patch returns a copy of this bytecode with its target resolved against
	// the given label addresses.  The receiver is left untouched.
	Patch(labels []Address) Patched
	// MaxWidth returns the largest number of code words this bytecode can
	// occupy, regardless of where its target resolves.
	MaxWidth() uint32
}

// Patched is a bytecode whose branch target has been resolved (i.e. the result
// of Patchable.Patch).  Its method set matches Bytecode, which is independent
// of the word type; hence patched bytecodes convert directly into Bytecode[W].
type Patched interface {
	String(Environment) string
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
func AddConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_ADD, asRegs(target), asRegs(sources...), constant)
}

// AddVec constructs a vectored addition instruction computing
// "targets = sum(sources)" (i.e. with no constant addend), where targets is a
// multi-limb register vector.
func AddVec[W word.Word[W]](targets []register.Id, sources []register.Id) *Arith[W] {
	var zero W
	return NewArith(ARITHOP_ADD, asRegs(targets...), asRegs(sources...), zero)
}

// AddVecConst constructs a vectored addition instruction computing
// "targets = sum(sources) + constant", where targets is a multi-limb register
// vector.
func AddVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_ADD, asRegs(targets...), asRegs(sources...), constant)
}

// CallFun constructs a function-call bytecode.
func CallFun(target ModuleId, checkpoint bool, args []register.Id, returns []register.Id) *Call {
	return &Call{target, checkpoint, asRegs(args...), asRegs(returns...)}
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
func NewSkipIf(op Cond, skip uint16, left, right register.Id) *SkipIf {
	return &SkipIf{Skip: skip, Left: NewRegVec(asReg(left)), Right: NewRegVec(asReg(right)), Op: op}
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
func LoadConst[W word.Word[W]](target register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_ADD, asRegs(target), nil, constant)
}

// Move constructs a move instruction which copies the source register into the
// target register.
func Move[W word.Word[W]](target register.Id, source register.Id) *Arith[W] {
	var zero W
	return NewArith(ARITHOP_ADD, asRegs(target), asRegs(source), zero)
}

// MultiwaySkip constructs a multiway-skip (SMW) instruction which dispatches on
// the value of the source register against the given (value, target) table.
// Targets are label indices until resolved during encoding (see Smw.Patch).
func MultiwaySkip[W word.Word[W]](source register.Id, cases []SwitchCase[W]) *Switch[W] {
	return &Switch[W]{Source: asReg(source), Cases: cases}
}

// MulConst constructs a multiplication instruction computing
// "target = product(sources) * constant" into a single target register.
func MulConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_MUL, asRegs(target), asRegs(sources...), constant)
}

// MulVecConst constructs a vectored multiplication instruction computing
// "targets = product(sources) * constant", where targets is a multi-limb
// register vector.
func MulVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_MUL, asRegs(targets...), asRegs(sources...), constant)
}

// ReadRom constructs a read instruction for a (non-static) read-only memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadRom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: ROM_READ, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// ReadStaticRom constructs a read instruction for a static read-only memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadStaticRom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: SROM_READ, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// ReadRam constructs a read instruction for a (small) random-access memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: SRAM_READ, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// ReadPagedRam constructs a read instruction for a paged random-access memory.
// The data registers receive the row located at the address given by the
// address registers, in the memory identified by id.
func ReadPagedRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: PRAM_READ, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// SubConst constructs a subtraction instruction computing
// "target = sources[0] - ... - constant" into a single target register.
func SubConst[W word.Word[W]](target register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_SUB, asRegs(target), asRegs(sources...), constant)
}

// SubVecConst constructs a vectored subtraction instruction computing
// "targets = sources[0] - ... - constant", where targets is a multi-limb
// register vector.
func SubVecConst[W word.Word[W]](targets []register.Id, sources []register.Id, constant W) *Arith[W] {
	return NewArith(ARITHOP_SUB, asRegs(targets...), asRegs(sources...), constant)
}

// WriteWom constructs a write instruction for a write-once memory.  The data
// registers are written to the row located at the address given by the address
// registers, in the memory identified by id.
func WriteWom(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: WOM_WRITE, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// WriteRam constructs a write instruction for a (small) random-access memory.
// The data registers are written to the row located at the address given by the
// address registers, in the memory identified by id.
func WriteRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: SRAM_WRITE, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// WritePagedRam constructs a write instruction for a paged random-access
// memory.  The data registers are written to the row located at the address
// given by the address registers, in the memory identified by id.
func WritePagedRam(id uint16, address []register.Id, data []register.Id) *ReadWrite {
	return &ReadWrite{Mode: PRAM_WRITE, Id: id, Address: asRegs(address...), Data: asRegs(data...)}
}

// NewBitwise constructs a bitwise instruction (and/or/xor) computing
// "target = left op right".
func NewBitwise(op BitwiseOp, target, left, right register.Id, bitwidth uint16) *Bitwise {
	return &Bitwise{Op: op, Target: asReg(target), Left: asReg(left), Right: asReg(right), Bitwidth: bitwidth}
}

// NewCheckCast constructs a check-cast instruction asserting that the given
// target register fits within the given bit width.
func NewCheckCast(target register.Id, bitwidth uint16) *CheckCast {
	//
	return &CheckCast{Bitwidth: bitwidth, Target: asReg(target)}
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
func NewDivHint(quotient, remainder, witness, dividend, divisor register.Id) *DivHint {
	return &DivHint{Quotient: asReg(quotient), Remainder: asReg(remainder), Witness: asReg(witness),
		Dividend: asReg(dividend), Divisor: asReg(divisor)}
}

// NewDivRem constructs a division/remainder instruction computing
// "target = dividend op divisor".
func NewDivRem(op uint32, target, dividend, divisor register.Id) *DivRem {
	return &DivRem{Opcode: op, Target: asReg(target), Dividend: asReg(dividend), Divisor: asReg(divisor)}
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
func NewFieldArith[W word.Word[W]](op uint32, target register.Id, sources []register.Id, constant W) *FieldArith[W] {
	return &FieldArith[W]{Op: op, Target: asReg(target), Sources: asRegs(sources...), Constant: constant}
}

// NewRet constructs a return instruction with the given frame width and return
// offset.
func NewRet() *Ret {
	return &Ret{}
}

// Concat constructs a concatenation instruction which joins the source
// registers into the target register vector.
func Concat(targets []register.Id, sources []register.Id) *Cat {
	return &Cat{Targets: asRegs(targets...), Sources: asRegs(sources...)}
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
func IsUnusedConstant[W word.Word[W]](op ArithOp, constant W) bool {
	switch op {
	case ARITHOP_ADD:
		return constant.Cmp64(0) == 0
	case ARITHOP_SUB:
		return constant.Cmp64(0) == 0
	case ARITHOP_MUL:
		return constant.Cmp64(1) == 0
	default:
		panic("unknown arithmetic operation")
	}
}
