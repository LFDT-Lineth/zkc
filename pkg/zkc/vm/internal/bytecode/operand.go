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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Operand represents a operand (either a register or constant) which can
// be split into multiple components.
type Operand[W word.Word[W]] struct {
	Operand util.Union[RegisterVector, []W]
}

// NewRegisterOperand creates a new operand from a set of one or more register
// limbs, where the most significant limb comes first (i.e. has the lowest
// index).
func NewRegisterOperand[W word.Word[W]](limbs ...RegisterId) Operand[W] {
	vec := NewRegisterVector(limbs...)
	return Operand[W]{util.Union1[RegisterVector, []W](vec)}
}

// NewRegisterVectorOperand creates a new operand from a register vector.
func NewRegisterVectorOperand[W word.Word[W]](limbs RegisterVector) Operand[W] {
	return Operand[W]{util.Union1[RegisterVector, []W](limbs)}
}

// NewConstantOperand creates a new operand from a set of or more constant
// limbs, where the most significant limb comes first (i.e. has the lowest
// index).
func NewConstantOperand[W word.Word[W]](constant ...W) Operand[W] {
	return Operand[W]{util.Union2[RegisterVector](constant)}
}

// AsConstant accesses the underlying constant as a single constant, rather than
// a vector.  This will panic, however, if either: (1) the operand is a
// register; or (2) it is a constant with more than one limb.
func (p Operand[W]) AsConstant() W {
	var limbs = p.Operand.Second()
	//
	if len(limbs) > 1 {
		panic("constant operand has multiple limbs")
	}
	//
	return limbs[0]
}

// AsConstants access the underlying constant for this operand.  This will panic
// if the operand is, in fact, a register.
func (p Operand[W]) AsConstants() []W {
	return p.Operand.Second()
}

// AsRegister returns the underlying register for this operand.  This will
// panic, however, if either: (1) the operand is a constant; or (2) it is a
// register with more than one limb.
func (p Operand[W]) AsRegister() RegisterId {
	var limbs = p.Operand.First()
	//
	if limbs.Len > 1 {
		panic("register operand has multiple limbs")
	}
	//
	return limbs.Base
}

// AsRegisterVector returns the underlying register vector for this operand.  This
// will panic if the operand is, in fact, a constant.
func (p Operand[W]) AsRegisterVector() RegisterVector {
	return p.Operand.First()
}

// AsRegisters returns the underlying registers for this operand.  This will
// panic if the operand is, in fact, a constant.
func (p Operand[W]) AsRegisters() []RegisterId {
	return p.Operand.First().Registers()
}

// IsConstant determines whether or not this operand is constant.
func (p Operand[W]) IsConstant() bool {
	return p.Operand.HasSecond()
}

// IsRegisterVector determines whether or not this operand is a register.
func (p Operand[W]) IsRegisterVector() bool {
	return p.Operand.HasFirst()
}

func (p Operand[W]) String(env Environment[W]) string {
	if p.IsRegisterVector() {
		return RegisterVectorToString(p.AsRegisterVector(), env)
	} else if len(p.AsConstants()) == 0 {
		return "0"
	}
	//
	return constantVectorToString(p.AsConstants())
}

func constantVectorToString[W word.Word[W]](constant []W) string {
	var (
		n     = len(constant)
		first = constant[0].Text(16)
	)
	switch n {
	case 1:
		return first
	case 2:
		var second = constant[1].Text(16)
		return fmt.Sprintf("%s;%s", first, second)
	default:
		var last = constant[n-1].Text(16)
		return fmt.Sprintf("%s;,,;%s", first, last)
	}
}
