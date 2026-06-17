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
)

// CheckCast instruction.
type CheckCast struct {
	Bitwidth uint16
	Target   RegisterId
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *CheckCast) Clone() Patched {
	var c = *p
	return &c
}

// Uses implementation for Bytecode interface.  A check-cast reads its target to
// assert the held value fits within the given bit width.
func (p *CheckCast) Uses() []RegisterId {
	return []RegisterId{p.Target}
}

// Definitions implementation for Bytecode interface.  A check-cast asserts a
// property of an existing register without writing a new value, so it defines
// nothing.  In particular, it does not redefine its target: doing so would
// conflict with the definition it is paired with to validate (e.g. the
// preceding arithmetic write).
func (p *CheckCast) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *CheckCast) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *CheckCast) String(env Environment) string {
	return fmt.Sprintf("check %s:u%d", RegisterToString(p.Target, env), p.Bitwidth)
}
