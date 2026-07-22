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

// CheckCast instruction.
type CheckCast[W word.Word[W]] struct {
	Bitwidth uint16
	Target   RegisterId
}

// Uses implementation for Bytecode interface.  A check-cast reads its target to
// assert the held value fits within the given bit width.
func (p *CheckCast[W]) Uses() []RegisterId {
	return []RegisterId{p.Target}
}

// Definitions implementation for Bytecode interface.  A check-cast asserts a
// property of an existing register without writing a new value, so it defines
// nothing.  In particular, it does not redefine its target: doing so would
// conflict with the definition it is paired with to validate (e.g. the
// preceding arithmetic write).
func (p *CheckCast[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *CheckCast[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	return validateOperands(env, p.Uses())
}

func (p *CheckCast[W]) String(env Environment[W]) string {
	return fmt.Sprintf("check %s:u%d", RegisterToString(p.Target, env), p.Bitwidth)
}
