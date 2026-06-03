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
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/array"
)

type Encoder struct {
	bytecodes []uint32
	//
	labels []uint
}

// Label marks a jump label.
func (p *Encoder) Label(label uint) {
	p.labels = array.Expand(p.labels, label+1)
	p.labels[label] = uint(len(p.bytecodes))
}

// Fail encodes a FAIL instruction to a given label.
func (p *Encoder) Fail() {
	p.bytecodes = append(p.bytecodes, FAIL)
}

// Jmp encodes a JMP instruction to a given label.
func (p *Encoder) Jmp(label uint) {
	var insn uint32
	//
	if label >= 0x8000000 {
		panic("jump label out of bounds")
	}
	//
	insn = (uint32(label) << 5) | JMP
	//
	p.bytecodes = append(p.bytecodes, insn)
}

// JmpIf encodes a JIF instruction to a given label.
func (p *Encoder) JmpIf(label uint, condition Condition, left, right register.Id) {
	var (
		insn, opcode uint32
		l            = uint32(left.Unwrap())
		r            = uint32(right.Unwrap())
	)
	// sanity checks
	if label >= 256 {
		panic("jump label out of bounds")
	} else if l >= 256 {
		panic("left register out of bounds")
	} else if r >= 256 {
		panic("right register out of bounds")
	}
	//
	opcode = (uint32(condition) << 5) | JIF
	insn = (uint32(label) << 24) | (l << 16) | (r << 8) | opcode
	//
	p.bytecodes = append(p.bytecodes, insn)
}

// Call encodes a CALL instruction to a given label.
func (p *Encoder) Call(label uint, inputs uint) {
	panic("todo")
}

// Ret encodes a RET instruction from a given function call.
func (p *Encoder) Ret(ninputs, width uint) {
	panic("todo")
}
