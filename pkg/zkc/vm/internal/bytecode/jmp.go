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

// Jmp (unconditional branch) instruction
type Jmp struct{ Target Address }

// Clone implementation for Bytecode / Patched interfaces.
func (p *Jmp) Clone() Patched {
	var c = *p
	return &c
}

// Uses implementation for Bytecode interface.
func (p *Jmp) Uses() []RegisterId {
	return nil
}

// Definitions implementation for Bytecode interface.
func (p *Jmp) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *Jmp) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

// Patch implementation for Patchable interface
func (p *Jmp) Patch(labels []Address) Patched {
	return &Jmp{labels[p.Target]}
}

func (p *Jmp) String(_ Environment) string {
	return fmt.Sprintf("jmp 0x%x", p.Target)
}
