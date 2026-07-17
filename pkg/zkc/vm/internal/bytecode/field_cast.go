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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FieldCast converts a value between a native field register and a uint register
// vector, in either direction.  It is the sole bridge across the 𝔽 / uint
// boundary: every other bytecode operates either purely on uint registers or
// (for field arithmetic) purely on native registers.
//
// Exactly one of Target / Source is native; the other is a uint register vector
// (a single register before splitting, several limbs afterwards).  The value is
// copied unchanged and asserted to lie in range:
//
//   - to 𝔽   (uint Source → native Target): the value must be canonical, i.e.
//     strictly less than the field modulus P, upholding the invariant that a
//     native register always holds a value in [0, P).
//   - from 𝔽 (native Source → uint Target): the (canonical) value must fit
//     within the total bit width of the target, exactly as for a narrowing
//     integer cast.
//
// Registers are listed least-significant limb first (matching Cat).
type FieldCast[W word.Word[W]] struct {
	// Target receives the converted value, least-significant limb first.
	Target []RegisterId
	// Source holds the value being converted, least-significant limb first.
	Source []RegisterId
}

// Uses implementation for Bytecode interface.
func (p *FieldCast[W]) Uses() []RegisterId {
	return p.Source
}

// Definitions implementation for Bytecode interface.
func (p *FieldCast[W]) Definitions() []RegisterId {
	return p.Target
}

// Validate implementation for Bytecode interface.
func (p *FieldCast[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *FieldCast[W]) String(env Environment[W]) string {
	var builder strings.Builder
	//
	builder.WriteString("cast ")
	builder.WriteString(RegistersToString(array.Reverse(p.Target), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(array.Reverse(p.Source), env, "::"))
	//
	return builder.String()
}
