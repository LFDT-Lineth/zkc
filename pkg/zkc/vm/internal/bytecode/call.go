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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Call invokes another function module.
type Call[W word.Word[W]] struct {
	// address of target function
	Target ModuleId
	// Arguments are caller-frame registers copied into callee inputs.
	Arguments []RegisterId
	// Returns are caller-frame registers receiving callee outputs.
	Returns []RegisterId
}

// Uses implementation for Bytecode interface.  A call reads the argument
// registers passed into the callee.
func (p *Call[W]) Uses() []RegisterId {
	return p.Arguments
}

// Definitions implementation for Bytecode interface.  A call writes the callee's
// outputs into the return registers of the caller's frame.
func (p *Call[W]) Definitions() []RegisterId {
	return p.Returns
}

// Validate implementation for Bytecode interface.
func (p *Call[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errors := validateOperands(env, p.Arguments, p.Returns)

	module := env.Module(p.Target)
	if module.IsEmpty() {
		return append(errors, fmt.Errorf("call target %d does not exist", p.Target))
	}

	callee := module.Unwrap()
	if !callee.IsFunction() {
		return append(errors, fmt.Errorf("call target %d (%s) is not a function", p.Target, callee.Name()))
	}

	if len(p.Arguments) != int(callee.NumInputs()) {
		errors = append(errors, fmt.Errorf("call to %s expects %d arguments (found %d)",
			callee.Name(), callee.NumInputs(), len(p.Arguments)))
	}

	if len(p.Returns) > int(callee.NumOutputs()) {
		errors = append(errors, fmt.Errorf("call to %s provides only %d returns (found %d)",
			callee.Name(), callee.NumOutputs(), len(p.Returns)))
	}

	return errors
}

func (p *Call[W]) String(env Environment[W]) string {
	var (
		builder strings.Builder
		// Determine enclosing module
		mod = env.Module(p.Target)
	)
	//
	builder.WriteString("call ")
	//
	if len(p.Returns) > 0 {
		builder.WriteString(RegistersToString(p.Returns, env, ","))
		builder.WriteString(" = ")
	}
	//
	name := "???"
	if mod.HasValue() {
		name = mod.Unwrap().Name()
	}

	fmt.Fprintf(&builder, "%s(%s)", name, RegistersToString(p.Arguments, env, ","))
	//
	return builder.String()
}
