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
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ArithOp identifies an arithmetic operation (add, subtract or multiply).
type ArithOp struct{ tag uint8 }

// String returns the infix operator symbol for this operation.
func (p ArithOp) String() string {
	switch p {
	case ARITHOP_ADD:
		return " + "
	case ARITHOP_SUB:
		return " - "
	case ARITHOP_MUL:
		return " * "
	default:
		panic("unknown arithmetic operation")
	}
}

// Tag returns the underlying tag for this operation.
func (p ArithOp) Tag() uint8 {
	return p.tag
}

// Prefix returns the mnemonic prefix for this operation.
func (p ArithOp) Prefix() string {
	switch p {
	case ARITHOP_ADD:
		return "add"
	case ARITHOP_SUB:
		return "sub"
	case ARITHOP_MUL:
		return "mul"
	default:
		panic("unknown arithmetic operation")
	}
}

// ARITHOP_ADD, ARITHOP_SUB and ARITHOP_MUL identify the arithmetic operation
// performed by an Arith instruction.
var (
	ARITHOP_ADD = ArithOp{0}
	ARITHOP_SUB = ArithOp{1}
	ARITHOP_MUL = ArithOp{2}
)

// NewArith constructs a new arithmetic instruction computing
// "targets = sources[0] op sources[1] op ... op constant".
func NewArith[W word.Word[W]](op ArithOp, targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	return &Arith[W]{op, constant, sources, targets}
}

// Arith (arithmetic) instruction encodes a wide range of related arithmetic
// operations (e.g. +,-,*) including various bitwise operations.
type Arith[W word.Word[W]] struct {
	Op       ArithOp
	Constant W
	Source   []RegisterId
	Target   []RegisterId
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *Arith[W]) Clone() Patched {
	return &Arith[W]{p.Op, p.Constant, slices.Clone(p.Source), slices.Clone(p.Target)}
}

// Uses implementation for Bytecode interface.
func (p *Arith[W]) Uses() []RegisterId {
	return p.Source
}

// Definitions implementation for Bytecode interface.
func (p *Arith[W]) Definitions() []RegisterId {
	return p.Target
}

// Validate implementation for Bytecode interface.
func (p *Arith[W]) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *Arith[W]) String(env Environment) string {
	var (
		builder strings.Builder
		cz      = IsUnusedConstant(p.Op, p.Constant)
		cstr    = fmt.Sprintf("0x%s", p.Constant.Text(16))
		prefix  string
	)
	//
	switch {
	case len(p.Source) == 0:
		prefix = "ldc"
	case len(p.Source) == 1 && cz:
		prefix = "mov"
	default:
		prefix = p.Op.Prefix()
	}
	//
	builder.WriteString(prefix)
	builder.WriteString(" ")
	builder.WriteString(RegistersToString(array.Reverse(p.Target), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(p.Source, env, p.Op.String()))
	//
	if len(p.Source) == 0 {
		builder.WriteString(cstr)
	} else if !cz {
		builder.WriteString(p.Op.String())
		builder.WriteString(cstr)
	}
	//
	return builder.String()
}
