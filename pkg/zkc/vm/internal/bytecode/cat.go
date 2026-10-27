// Copyright Consensys Incorporated
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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Cat concatenates source register bits and stores the result across targets.
type Cat[W word.Word[W]] struct {
	// Targets receive the concatenated value, least-significant limb first.
	Targets []RegisterId
	// Sources are concatenated with Sources[0] in the least-significant bits.
	Sources []RegisterId
}

// Uses implementation for Bytecode interface.
func (p *Cat[W]) Uses() []RegisterId {
	return p.Sources
}

// Definitions implementation for Bytecode interface.
func (p *Cat[W]) Definitions() []RegisterId {
	return p.Targets
}

// Validate implementation for Bytecode interface.
func (p *Cat[W]) Validate(env Environment[W]) ([]error, bool) {
	var errors, safe = validateOperands(env, p.Sources, p.Targets)
	// A single-source, single-target concatenation is a plain move, which must
	// be expressed as an assignment instead (see AssignV).
	if p.isUnit() {
		errors = append(errors, fmt.Errorf("concatenation is a plain move (use assignment)"))
		safe = false
	}
	//
	if !safe {
		return errors, false
	}
	// validate bitwidths
	errors = append(errors, validateConcatBitwidth(env, p.Targets)...)
	errors = append(errors, validateConcatBitwidth(env, p.Sources)...)
	//
	return errors, safe
}

func (p *Cat[W]) String(env Environment[W]) string {
	var builder strings.Builder
	//
	builder.WriteString("cat ")
	builder.WriteString(RegistersToString(array.Reverse(p.Targets), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(array.Reverse(p.Sources), env, "::"))
	//
	return builder.String()
}

// isUnit returns true if this is a "unit" operation.  That is, an assignment of
// the form "x = y".
func (p *Cat[W]) isUnit() bool {
	return len(p.Targets) == 1 && len(p.Sources) == 1
}
