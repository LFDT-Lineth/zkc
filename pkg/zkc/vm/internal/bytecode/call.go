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
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Call invokes another function module.
type Call struct {
	// Id is the target function module identifier.
	Id uint16
	// Arguments are caller-frame registers copied into callee inputs.
	Arguments []Reg
	// Returns are caller-frame registers receiving callee outputs.
	Returns []Reg
}

func (p *Call) String(mapping SystemMap) string {
	var builder strings.Builder
	//
	if len(p.Returns) > 0 {
		builder.WriteString(registersToString(p.Returns, mapping, ","))
		builder.WriteString(" = ")
	}
	//
	name := "??"
	if mapping != nil {
		name = mapping.Module(uint(p.Id)).Name()
	}
	//
	fmt.Fprintf(&builder, "%s(%s)", name, registersToString(p.Arguments, mapping, ","))
	//
	return builder.String()
}

// Codes implementation for Bytecode interface.
func (p *Call) Codes(_ uint32) []uint32 {
	return encodeCall(p.Id, p.Arguments, p.Returns)
}

func decodeCall[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		id, args, returns, n = decodeCallOperands(pc, codes)
	)
	//
	return &Call{id, args, returns}, n
}

// ============================================================================
// CALL instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// | nrets  | nargs  |   id   | opcode |
// +--------+--------+--------+--------+
// | arg3   | arg2   | arg1   | arg0   |
// +--------+--------+--------+--------+
// | ... packed return registers ...    |
// +------------------------------------+
//
// The id, counts and register operands are all small u8 values.
// ============================================================================

func encodeCall(id uint16, args []Reg, returns []Reg) []uint32 {
	var (
		_id   = uint32(id) << 8
		nargs = uint32(len(args)) << 16
		nrets = uint32(len(returns)) << 24
		codes = []uint32{nrets | nargs | _id | CALL}
		bytes = append(regsAsBytes(args), regsAsBytes(returns)...)
	)
	//
	if id >= 256 {
		panic("wide call instructions not supported")
	}
	//
	return append(codes, packRegsIntoCodes(bytes)...)
}

func decodeCall_sn(pc uint32, codes []uint32) (id uint16, nargs, nrets uint, regs Op8Iter, n uint32) {
	nargs = uint((codes[pc] >> 16) & 0xff)
	nrets = uint((codes[pc] >> 24) & 0xff)
	id = uint16((codes[pc] >> 8) & 0xff)
	regs = NewOp8Iter(0, codes[pc+1:])
	//
	return id, nargs, nrets, regs, 1 + nCodesPackedSmall(uint32(nargs+nrets))
}

func decodeCallOperands(pc uint32, codes []uint32) (id uint16, args, returns []Reg, n uint32) {
	var (
		nargs uint
		nrets uint
		iter  Op8Iter
		regs  []Reg
	)
	//
	id, nargs, nrets, iter, n = decodeCall_sn(pc, codes)
	// Operands are stored as args first, then return targets.
	regs = OpIterToArray[uint16](nargs+nrets, iter)
	//
	return id, regs[:nargs], regs[nargs:], n
}

// NewCall constructs a function-call bytecode.
func NewCall(id uint16, args []register.Id, returns []register.Id) *Call {
	return &Call{id, asRegs(args...), asRegs(returns...)}
}
