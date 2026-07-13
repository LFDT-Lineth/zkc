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

// CallFlags captures boolean properties of a call which do not affect how it
// executes, but which are significant for constraint lowering.
type CallFlags struct {
	// Unconditional indicates whether the corresponding lookup holds
	// unconditionally (i.e. is not gated by a selector), as used (for example)
	// for range checks.  This mirrors instruction.UnconditionalCall.
	Unconditional bool
}

// Call invokes another function module.
type Call[W word.Word[W]] struct {
	// address of target function
	Target ModuleId
	// Flags captures boolean properties of this call (e.g. whether it is an
	// unconditional call).
	Flags CallFlags
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
func (p *Call[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
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
	fmt.Fprintf(&builder, "%s(%s)", mod.Name(), RegistersToString(p.Arguments, env, ","))
	//
	return builder.String()
}
