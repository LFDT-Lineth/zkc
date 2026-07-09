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
package vm

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W Word[W]] = bytecode.Bytecode[W]

// BytecodeModule describes a moddule, such as a function or memory
type BytecodeModule[W Word[W]] = descriptor.Module[W]

// Function contains information about an executable function in the system.  A
// function has one or more registers where: the first n registers are the input
// registers (i.e. parameters); the next m registers are the output registers
// (i.e. returns); and all remaining registers are internal (sometimes also
// referred to as computed registers).  Additionally, a function has some number
// of "instructions" which capture its semantics (i.e. intended behaviour).  The
// notion of an instruction is specifically left undefined by this interface to
// support different levels of the compilation pipeline.  For example, a
// compiled function has instructions which are simply bytes (or words) for
// efficient execution.  However, the instructions of an "assembly" level
// function implement the Instruction interface, which is better suited to
// analysis and/or translation into constraints.
type Function[W Word[W]] = descriptor.Function[W]

// BytecodeMemory describes a memory
type BytecodeMemory[W Word[W]] = descriptor.Memory[W]

// Register describes a register
type Register[W Word[W]] = descriptor.Register[W]

// RegisterId identifies a register within a module.  It is the register
// identifier used throughout the bytecode layer (an alias of the internal
// bytecode RegisterId).
type RegisterId = bytecode.RegisterId

// BytecodeVector bundles together one or more bytecode instructions which, with
// restrictions, can be executed by the underlying machine "in parallel".  The
// approach is analoguous to the concept of "Very-Long Instruction Words (VLIW)"
// but taken to more of an extreme --- there is no limit on the number of
// bytecode instructions.
//
// To better understand bytecode vectors, consider two instructions executed in
// sequence (the at pc location 0, the second at pc location 1):
//
// (pc=0) x = y + 1 (pc=1) z = 0
//
// When executing these instructions, there is an intermediate state after the
// first instruction is executed but before the second has been where x has been
// written but z has not.  Alternatively, the two instructions can be composed
// together to form a bytecode vector, written like so:
//
// (pc=0) x = y + 1 ; z = 0
//
// In this case, both instructions are executed together and there is no
// intermediate state where x is written but z is not.
//
// To ensure easy translation into polynomial constraints, there are
// restrictions on how bytecode vectors can be composed.  In particular, no
// variable can be assigned twice on the same execution path.  Thus, for
// example, this is an invalid bytecode vector:
//
// (pc=0) x = 0 ; x = 1
//
// These writes are said to be _conflicting_.  In contrast, the following is a
// valid bytecode vector:
//
// (pc=0) skip_if x != y 2 ; r = 0 ; ret ; r = 1 ; ret
//
// In this case, whilst there are two assignments to register r, neither are on
// the same path.  These writes are said to be _non-conflicting_.  Finally, we
// should note that register forwarding is applied within bytecode vectors.
// Thus, for example, the following is allowed:
//
// (pc=0) x = 0; y = x + 1; ret
//
// Here, the value of x written in the instruction is "forwarded" to the
// assignment for y.  This process is, roughly speaking, analoguous to register
// forwarding as found in CPU architectures.
type BytecodeVector[W Word[W]] = bytecode.Vector[W]

// Interpreter is an optimised bytecode interpreter for executing word
// instructions.
type Interpreter[W Word[W]] = interpreter.Interpreter[W]

// Program represents a bytecode assembly program.
type Program[W Word[W]] = descriptor.Program[W]

// BinaryProgram represents a compiled bytecode program, along with
// accompanying symbolic information.  This is primarily useful for debugging.
type BinaryProgram[W Word[W]] = encoding.Binary[W]

// BytecodeEnvironment provides information about the enclosing environment of a
// bytecode, and is primarily for debugging and validation.
type BytecodeEnvironment = bytecode.Environment

// NewBytecodeInterpreter constructs an interpreter for executing the given
// bytecode program.  The modulus is the prime characteristic of the surrounding
// field, used when executing native field instructions.
func NewBytecodeInterpreter[W word.Word[W]](program Program[W]) *Interpreter[W] {
	return interpreter.New(program, false)
}

// CompileProgram compiles a program descriptor into an binary (i.e. executable)
// bytecode program.
func CompileProgram[W word.Word[W]](p Program[W]) BinaryProgram[W] {
	return interpreter.CompileProgram(p, false)
}

// NewBytecodeProgram assembles a bytecode program directly from pre-lowered
// descriptor modules, bypassing the word-machine round trip.
func NewBytecodeProgram[W word.Word[W]](field field.Config, modules ...BytecodeModule[W]) Program[W] {
	return descriptor.NewProgram(field, modules...)
}

// NewBytecodeVector constructs a bytecode vector (single trace line) from the
// given bytecodes.
func NewBytecodeVector[W word.Word[W]](codes ...Bytecode[W]) BytecodeVector[W] {
	return bytecode.NewVector(codes...)
}

// NewBytecodeFunction constructs a bytecode (descriptor) function module from its
// registers and a body of bytecode vectors.
func NewBytecodeFunction[W word.Word[W]](name string, native bool, registers []Register[W],
	code ...BytecodeVector[W]) *Function[W] {
	return descriptor.NewFunction[W](name, registers, native, code)
}

// NewRegister constructs a new register descriptor, where native
// registers are indicated by the absence of any specific bitwidth.
func NewRegister[W word.Word[W]](kind register.Type, name string, bitwidth util.Option[uint],
	padding W) Register[W] {
	//
	return descriptor.NewRegister(kind, name, bitwidth, padding)
}

// NewComputedRegister constructs a computed register descriptor of the given
// name and bit-width.  A bit-width of math.MaxUint yields a native (field)
// register, which carries no fixed bit-width.
func NewComputedRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	return descriptor.NewRegister(register.COMPUTED_REGISTER, name, bitwidth, padding)
}

// NewInputRegister constructs an input register descriptor of the given name
// and bit-width.  A bit-width of math.MaxUint yields a native (field) register,
// which carries no fixed bit-width.
func NewInputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	return descriptor.NewRegister(register.INPUT_REGISTER, name, bitwidth, padding)
}

// NewOutputRegister constructs an output register descriptor of the given name
// and bit-width.  A bit-width of math.MaxUint yields a native (field) register,
// which carries no fixed bit-width.
func NewOutputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	return descriptor.NewRegister(register.OUTPUT_REGISTER, name, bitwidth, padding)
}

// ============================================================================
// Bytecode construction
// ============================================================================

// The aliases and constructors below re-export the bytecode construction API so
// that external packages can build bytecode instructions without reaching into
// the internal bytecode package.  For now, each constructor returns the general
// Bytecode[W] interface rather than the specific underlying instruction type.

// Cond provides a convenient alias for the comparison condition used by
// conditional skip instructions.
type Cond = bytecode.Cond

// Address provides a convenient alias for a branch target address.
type Address = bytecode.Address

// ModuleId provides a convenient alias for a module identifier.
type ModuleId = bytecode.ModuleId

// SwitchCase is a single (value, target) entry of a multiway-skip dispatch
// table.
type SwitchCase[W any] = bytecode.SwitchCase[W]

// FormattedChunk describes a single chunk of a formatted (debug/fail) message.
type FormattedChunk = base.FormattedChunk

// NewFormattedChunk constructs a formatted (debug/fail) message chunk from its
// text, format directive and (optional) argument registers.  The argument
// registers are bundled into the chunk's register vector internally, so callers
// work purely in terms of RegisterId.
func NewFormattedChunk(text string, format zkc_util.Format, args ...RegisterId) FormattedChunk {
	ids := make([]register.Id, len(args))
	//
	for i, a := range args {
		ids[i] = register.NewId(uint(a))
	}
	//
	return FormattedChunk{Text: text, Format: format, Argument: register.NewVector(ids...)}
}

// The instruction constructors below are deliberately named to mirror the
// word-instruction constructors in pkg/zkc/vm/instruction (e.g. UintAdd, BitAnd,
// NewJump), so that the same operation carries the same name at both the
// bytecode and word layers.

// LoadConst constructs an instruction assigning a constant into a single
// target register.
func LoadConst[W Word[W]](target RegisterId, constant W) Bytecode[W] {
	return bytecode.LoadConst(target, constant)
}

// Add constructs an addition instruction computing
// "target = sum(sources) + constant" into a single target register.
func Add[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.AddConst(target, sources, constant)
}

// AddVec constructs a vectored addition instruction computing
// "targets = sum(sources)" (i.e. with no constant addend), where targets is a
// multi-limb register vector.
func AddVec[W Word[W]](targets []RegisterId, sources []RegisterId) Bytecode[W] {
	return bytecode.AddVec[W](targets, sources)
}

// AddVecConst constructs a vectored addition instruction computing
// "targets = sum(sources) + constant", where targets is a multi-limb register
// vector.
func AddVecConst[W Word[W]](targets []RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.AddVecConst(targets, sources, constant)
}

// Assign constructs a move instruction which copies the source register into
// the target register.
func Assign[W Word[W]](target RegisterId, source RegisterId) Bytecode[W] {
	return bytecode.Move[W](target, source)
}

// CallFlags captures boolean properties of a call (checkpoint / unconditional).
type CallFlags = bytecode.CallFlags

// Call constructs a function-call bytecode with the given flags.
func Call[W Word[W]](target ModuleId, flags CallFlags, args []RegisterId, returns []RegisterId) Bytecode[W] {
	return bytecode.CallFun(target, flags, args, returns)
}

// Jump creates an unconditional jump instruction transferring control to the
// given target address.
func Jump[W Word[W]](target Address) Bytecode[W] {
	return bytecode.Jump(target)
}

// Skip constructs an uncondition skip instruction which skips over n
// instructions.
func Skip[W Word[W]](skip uint16) Bytecode[W] {
	return bytecode.NewSkip(skip)
}

// SkipTargets returns, for a skip-like bytecode located at bytecode index
// `from` within its enclosing vector, the index of each bytecode to which
// control may transfer when the skip is taken.  A skip over n bytecodes
// transfers to `from + n + 1` (mirroring the interpreter's control flow).  For
// non-skip bytecodes, nil is returned.
func SkipTargets[W Word[W]](b Bytecode[W], from uint) []uint {
	switch b := b.(type) {
	case *bytecode.Skip:
		return []uint{from + uint(b.Skip) + 1}
	case *bytecode.SkipIf:
		return []uint{from + uint(b.Skip) + 1}
	case *bytecode.Switch[W]:
		targets := make([]uint, len(b.Cases))
		//
		for i, c := range b.Cases {
			targets[i] = from + uint(c.Skip) + 1
		}
		//
		return targets
	default:
		return nil
	}
}

// SkipIf constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing single registers.
func SkipIf[W Word[W]](op Cond, skip uint16, left, right RegisterId) Bytecode[W] {
	return bytecode.NewSkipIf(op, skip, left, right)
}

// SkipIfVec constructs a conditional branch instruction which jumps to the
// target address when "left op right" holds, comparing multi-limb register
// vectors.
func SkipIfVec[W Word[W]](op Cond, skip uint16, left, right register.Vector) Bytecode[W] {
	return bytecode.NewSkipIfVec(op, skip, left, right)
}

// Switch constructs a multiway-skip (SMW) instruction which dispatches
// on the value of the source register against the given (value, target) table.
func Switch[W Word[W]](source RegisterId, cases []SwitchCase[W]) Bytecode[W] {
	return bytecode.MultiwaySkip(source, cases)
}

// Mul constructs a multiplication instruction computing
// "target = product(sources) * constant" into a single target register.
func Mul[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.MulConst(target, sources, constant)
}

// MulVec constructs a vectored multiplication instruction computing
// "targets = product(sources) * constant", where targets is a multi-limb
// register vector.
func MulVec[W Word[W]](targets []RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.MulVecConst(targets, sources, constant)
}

// MemRead constructs a memory-read instruction.  The data registers receive
// the row located at the address given by the address registers, in the memory
// identified by id.  The kind of memory being read (ROM, static ROM, RAM, paged
// RAM) is resolved from the environment when the instruction is encoded.
func MemRead[W Word[W]](id uint16, address []RegisterId, data []RegisterId) Bytecode[W] {
	return bytecode.NewMemRead(id, address, data)
}

// Sub constructs a subtraction instruction computing
// "target = sources[0] - ... - constant" into a single target register.
func Sub[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.SubConst(target, sources, constant)
}

// SubVec constructs a vectored subtraction instruction computing
// "targets = sources[0] - ... - constant", where targets is a multi-limb
// register vector.
func SubVec[W Word[W]](targets []RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.SubVecConst(targets, sources, constant)
}

// MemWrite constructs a memory-write instruction.  The data registers are
// written to the row located at the address given by the address registers, in
// the memory identified by id.  The kind of memory being written (write-once,
// RAM, paged RAM) is resolved from the environment when the instruction is
// encoded.
func MemWrite[W Word[W]](id uint16, address []RegisterId, data []RegisterId) Bytecode[W] {
	return bytecode.NewMemWrite(id, address, data)
}

// BitAnd constructs a bitwise-and instruction computing
// "target = left & right".  bitwidth is the operand/result width in bits.
func BitAnd[W Word[W]](target, left, right RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_AND, target, left, right, bitwidth)
}

// BitOr constructs a bitwise-or instruction computing
// "target = left | right".  bitwidth is the operand/result width in bits.
func BitOr[W Word[W]](target, left, right RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_OR, target, left, right, bitwidth)
}

// BitXor constructs a bitwise-xor instruction computing
// "target = left ^ right".  bitwidth is the operand/result width in bits.
func BitXor[W Word[W]](target, left, right RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_XOR, target, left, right, bitwidth)
}

// BitNot constructs a bitwise-not instruction computing
// "target = ^source".  bitwidth is the width (in bits) the complement is taken
// within, so the result holds only the low bitwidth bits of ^source.
func BitNot[W Word[W]](target, source RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_NOT, target, source, source, bitwidth)
}

// BitShl constructs a logical shift-left instruction computing
// "target = left << right".  bitwidth is the result width in bits; bits shifted
// out beyond it are discarded.
func BitShl[W Word[W]](target, left, right RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_SHL, target, left, right, bitwidth)
}

// BitShr constructs a logical shift-right instruction computing
// "target = left >> right".  bitwidth is the operand/result width in bits.
func BitShr[W Word[W]](target, left, right RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewBitwise(bytecode.OP_SHR, target, left, right, bitwidth)
}

// CheckCast constructs a check-cast instruction asserting that the given
// target register fits within the given bit width.
func CheckCast[W Word[W]](target RegisterId, bitwidth uint16) Bytecode[W] {
	return bytecode.NewCheckCast(target, bitwidth)
}

// Debug constructs a debug instruction carrying the given formatted message.
func Debug[W Word[W]](chunks []FormattedChunk) Bytecode[W] {
	return bytecode.NewDebug(chunks)
}

// Hint constructs a hint instruction performing the given operation op (e.g.
// DIV_HINT) which reads the given source (argument) register vectors and writes
// the given target (return) register vectors.
func Hint[W Word[W]](op bytecode.Operation, targets, sources []bytecode.RegisterVector) Bytecode[W] {
	return bytecode.NewHint(op, targets, sources)
}

// Div constructs an integer-division instruction computing
// "target = dividend / divisor".
func Div[W Word[W]](target, dividend, divisor RegisterId) Bytecode[W] {
	return bytecode.NewDivRem(encoding.DIV, target, dividend, divisor)
}

// Rem constructs an integer-remainder instruction computing
// "target = dividend % divisor".
func Rem[W Word[W]](target, dividend, divisor RegisterId) Bytecode[W] {
	return bytecode.NewDivRem(encoding.REM, target, dividend, divisor)
}

// Fail constructs a fail instruction carrying the given formatted message.
func Fail[W Word[W]](chunks []FormattedChunk) Bytecode[W] {
	return bytecode.NewFail(chunks)
}

// AddModP constructs a field-addition instruction computing
// "target = sources[0] + ... + constant" modulo the field prime.
func AddModP[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.NewFieldArith(bytecode.OP_ADDMOD_P, target, sources, constant)
}

// SubModP constructs a field-subtraction instruction computing
// "target = sources[0] - ... - constant" modulo the field prime.
func SubModP[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.NewFieldArith(bytecode.OP_SUBMOD_P, target, sources, constant)
}

// MulModP constructs a field-multiplication instruction computing
// "target = sources[0] * ... * constant" modulo the field prime.
func MulModP[W Word[W]](target RegisterId, sources []RegisterId, constant W) Bytecode[W] {
	return bytecode.NewFieldArith(bytecode.OP_MULMOD_P, target, sources, constant)
}

// Return constructs a return instruction with the given frame width and
// return offset.
func Return[W Word[W]]() Bytecode[W] {
	return bytecode.NewRet()
}

// BitConcat constructs a concatenation instruction which joins the source
// registers into the target register vector.
func BitConcat[W Word[W]](targets []RegisterId, sources []RegisterId) Bytecode[W] {
	return bytecode.Concat(targets, sources)
}

// ============================================================================
// Bytecode instruction types
// ============================================================================

// The aliases below re-export the concrete bytecode instruction types so that
// external packages (in particular the constraint generator in
// pkg/zkc/constraints) can dispatch on, and read the operands of, individual
// bytecodes without reaching into the internal bytecode package.  This
// complements the bytecode *construction* API above: those functions build
// bytecodes (returning the Bytecode[W] interface), whereas these types let a
// consumer take a Bytecode[W] apart again via a type switch.
//
// The concrete type names are prefixed with "Bytecode" to avoid colliding with
// the identically-named construction functions (e.g. the Call function versus
// the BytecodeCall struct).

// Operation identifies the operation performed by an arithmetic / field /
// bitwise / hint bytecode (see the OP_* constants).
type Operation = bytecode.Operation

// RegisterVector is a contiguous span of registers (a multi-limb operand).
type RegisterVector = bytecode.RegisterVector

// Operation constants (mirroring bytecode.OP_*).
const (
	// OP_ADD integer addition.
	OP_ADD = bytecode.OP_ADD
	// OP_SUB integer subtraction.
	OP_SUB = bytecode.OP_SUB
	// OP_MUL integer multiplication.
	OP_MUL = bytecode.OP_MUL
	// OP_AND bitwise conjunction.
	OP_AND = bytecode.OP_AND
	// OP_OR bitwise disjunction.
	OP_OR = bytecode.OP_OR
	// OP_XOR bitwise exclusive-or.
	OP_XOR = bytecode.OP_XOR
	// OP_NOT bitwise negation.
	OP_NOT = bytecode.OP_NOT
	// OP_SHL logical shift left.
	OP_SHL = bytecode.OP_SHL
	// OP_SHR logical shift right.
	OP_SHR = bytecode.OP_SHR
	// OP_ADDMOD_P addition modulo the field prime.
	OP_ADDMOD_P = bytecode.OP_ADDMOD_P
	// OP_SUBMOD_P subtraction modulo the field prime.
	OP_SUBMOD_P = bytecode.OP_SUBMOD_P
	// OP_MULMOD_P multiplication modulo the field prime.
	OP_MULMOD_P = bytecode.OP_MULMOD_P
	// DIV_HINT division hint operation.
	DIV_HINT = bytecode.DIV_HINT
)

// BytecodeArith is an integer arithmetic bytecode (target = op(sources) op constant).
type BytecodeArith[W Word[W]] = bytecode.Arith[W]

// BytecodeFieldArith is a field arithmetic bytecode (target = op(sources) op constant mod P).
type BytecodeFieldArith[W Word[W]] = bytecode.FieldArith[W]

// BytecodeCat is a concatenation bytecode (target vector = sources joined by width).
type BytecodeCat = bytecode.Cat

// BytecodeCall is a function-call bytecode.
type BytecodeCall = bytecode.Call

// BytecodeReadWrite is a memory read/write bytecode.
type BytecodeReadWrite = bytecode.ReadWrite

// BytecodeSkip is an unconditional skip bytecode.
type BytecodeSkip = bytecode.Skip

// BytecodeSkipIf is a conditional skip bytecode.
type BytecodeSkipIf = bytecode.SkipIf

// BytecodeSwitch is a multiway-skip (switch) bytecode.
type BytecodeSwitch[W Word[W]] = bytecode.Switch[W]

// BytecodeJmp is an unconditional jump bytecode.
type BytecodeJmp = bytecode.Jmp

// BytecodeRet is a return bytecode.
type BytecodeRet = bytecode.Ret

// BytecodeFail is a fail bytecode.
type BytecodeFail = bytecode.Fail

// BytecodeDebug is a debug bytecode.
type BytecodeDebug = bytecode.Debug

// BytecodeHint is a (non-deterministic) hint bytecode.
type BytecodeHint = bytecode.Hint

// BytecodeCheckCast is a width-check (cast) bytecode.
type BytecodeCheckCast = bytecode.CheckCast
