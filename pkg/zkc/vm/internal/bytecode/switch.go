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

// SwitchCase is a single (value, target) entry of a multiway-skip dispatch table:
// when the source register holds Value, control transfers to Target.
type SwitchCase[W any] struct {
	// Value compared against the source register.
	Value W
	// Skip amount.
	Skip uint16
}

// Switch (skip multiway) dispatches on the value of a source register: it is
// compared, in order, against each case's value and, on the first match,
// control transfers to that case's target.  If no case matches, control falls
// through to the following instruction.
//
// Encoding (1 + 3*len(Cases) words):
//
//	31      24 23       8 7      0
//	+---------+----------+--------+
//	|  count  |  source  | opcode |   word 0
//	+---------+----------+--------+
//	| .......... value_lo .......|   word 1   (case 0)
//	| .......... value_hi .......|   word 2
//	| .......... target .........|   word 3
//	|             ...            |            (further cases)
//
// Here count is the number of cases (<= 255), source is the dispatch register,
// and each case occupies three words: a 64-bit value (low word then high word)
// followed by an absolute target address.
type Switch[W word.Word[W]] struct {
	Source RegisterId
	Cases  []SwitchCase[W]
}

// Uses implementation for Bytecode interface.  A multiway skip reads the
// dispatch (source) register it compares against each case value.
func (p *Switch[W]) Uses() []RegisterId {
	return []RegisterId{p.Source}
}

// Definitions implementation for Bytecode interface.
func (p *Switch[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.  This mirrors
// base.SkipMulti.MicroValidate: every dispatch value must be unique (since the
// first match wins, a duplicate is unreachable and almost certainly a mistake)
// and must fit within the source register's width.  A native source register
// holds arbitrary-width values, so no value can overflow it.
func (p *Switch[W]) Validate(_ uint, _ FieldConfig, env Environment[W]) []error {
	var (
		errors []error
		seen   = make(map[string]bool)
		width  = env.Register(p.Source).Bitwidth()
	)
	//
	for _, c := range p.Cases {
		var key = c.Value.Text(16)
		// Detect duplicate dispatch values.
		if seen[key] {
			errors = append(errors, fmt.Errorf("duplicate dispatch value 0x%s", key))
		}
		//
		seen[key] = true
		// Check the value fits within the (non-native) source register.
		if width.HasValue() && !c.Value.FitsWithin(width.Unwrap()) {
			errors = append(errors, fmt.Errorf("dispatch value 0x%s overflows u%d", key, width.Unwrap()))
		}
	}
	//
	return errors
}

func (p *Switch[W]) String(_ Environment[W]) string {
	var b strings.Builder
	//
	fmt.Fprintf(&b, "switch r%d [", p.Source)
	//
	for i, c := range p.Cases {
		if i != 0 {
			b.WriteString(", ")
		}
		//
		fmt.Fprintf(&b, "0x%s:%d", c.Value.Text(16), c.Skip)
	}
	//
	b.WriteString("]")
	//
	return b.String()
}
