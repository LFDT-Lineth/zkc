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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Jmp (unconditional branch) instruction
type Jmp[W word.Word[W]] struct{ Target Address }

// Uses implementation for Bytecode interface.
func (p *Jmp[W]) Uses() []RegisterId {
	return nil
}

// Definitions implementation for Bytecode interface.
func (p *Jmp[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *Jmp[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *Jmp[W]) String(_ Environment[W]) string {
	return fmt.Sprintf("jmp 0x%x", p.Target)
}
