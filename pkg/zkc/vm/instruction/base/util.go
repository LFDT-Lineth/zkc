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
package base

import (
	"fmt"
	"math"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Module represents an either a function or memory within the machine.
type Module interface {
	// Name returns the name given to the enclosing entity (i.e. memory or
	// function).
	Name() string
	// HasRegister checks whether a register with the given name exists and, if
	// so, returns its register identifier.  Otherwise, it returns false.
	HasRegister(name string) (register.Id, bool)
	// IsNative reports whether this module is a function backed by a native
	// circuit.  Memory modules always return false.
	IsNative() bool
	// Access a given register in this module.
	Register(register.Id) register.Register
	// Registers providers access to the underlying registers of this map.
	Registers() []register.Register
	// Width returns the number of registers declared in this module.
	Width() uint
}

// SystemMap provides a global view of modules in the systemn.
type SystemMap interface {
	register.Map
	//
	Module(id uint) Module
}

// RegisterBitwidth returns the bitwidth of a register.  This function is
// resilient to situations where, for example, the environment is missing or the
// register Id is invalid.
func RegisterBitwidth(env register.Map, reg register.Id) uint {
	// support nil environment (for debugging)
	if env != nil && reg.Unwrap() < uint(len(env.Registers())) {
		return env.Register(reg).Width()
	}
	//
	return math.MaxUint
}

// RegistersToString returns a string representation for zero or more registers
// separated by a comma.
func RegistersToString(env register.Map, regs ...register.Id) string {
	var (
		builder strings.Builder
		n       uint
	)
	// support nil environment (for debugging)
	if env != nil {
		n = uint(len(env.Registers()))
	}
	//
	for i := 0; i < len(regs); i++ {
		var rid = regs[i]
		//
		if i != 0 {
			builder.WriteString(", ")
		}
		//
		if rid.Unwrap() < n {
			builder.WriteString(env.Register(rid).Name())
		} else {
			builder.WriteString("??")
		}
	}
	//
	return builder.String()
}

// ExpressionToString returns a string representation for an arithmetic expression involving a constant
func ExpressionToString[W word.Base[W]](op string, regs []register.Id, constant W, env register.Map) string {
	var builder strings.Builder
	//
	for i := 0; i < len(regs); i++ {
		var rid = regs[i]
		//
		builder.WriteString(RegistersToString(env, rid))
		builder.WriteString(" ")
		builder.WriteString(op)
		builder.WriteString(" ")
	}
	//
	fmt.Fprintf(&builder, "0x%s", constant.Text(16))
	//
	return builder.String()
}

// ExpressionToStringWithoutConst returns a string representation for an arithmetic expression
func ExpressionToStringWithoutConst(op string, regs []register.Id, env register.Map) string {
	var builder strings.Builder
	//
	for i := 0; i < len(regs); i++ {
		var rid = regs[i]
		//
		if i != 0 {
			builder.WriteString(" ")
			builder.WriteString(op)
			builder.WriteString(" ")
		}
		//
		builder.WriteString(RegistersToString(env, rid))
	}
	//
	return builder.String()
}
