// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIN, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0
package io

import (
	"fmt"
	"strings"

	"github.com/consensys/go-corset/pkg/util/collection/array"
)

// CheckTargetRegisters performs some simple checks on a set of target registers
// being written.  Firstly, they cannot be input registers (as this are always
// constant).  Secondly, we cannot write to the same register more than once
// (i.e. a conflicting write).
func CheckTargetRegisters(targets []RegisterId, regs []Register) error {
	for i, id := range targets {
		//
		if regs[targets[i].Unwrap()].IsInput() {
			return fmt.Errorf("cannot write input %s", regs[id.Unwrap()].Name())
		}
		//
		for j := i + 1; j < len(targets); j++ {
			if targets[i] == targets[j] {
				return fmt.Errorf("conflicting write to %s", regs[id.Unwrap()].Name())
			}
		}
	}
	//
	return nil
}

// RegistersToString returns a string representation for zero or more registers
// separated by a comma.
func RegistersToString(rids []RegisterId, regs []Register) string {
	var builder strings.Builder
	//
	for i := 0; i < len(rids); i++ {
		var rid = rids[i]
		//
		if i != 0 {
			builder.WriteString(", ")
		}
		//
		if i < len(regs) {
			builder.WriteString(regs[rid.Unwrap()].Name())
		} else {
			fmt.Fprintf(&builder, "?%d", rid)
		}
	}
	//
	return builder.String()
}

// RegistersReversedToString returns a string representation for zero or more
// registers in reverse order, separated by a comma.  This is useful, for
// example, when printing the left-hand side of an assignment.
func RegistersReversedToString(rids []RegisterId, regs []Register) string {
	return RegistersToString(array.Reverse(rids), regs)
}
