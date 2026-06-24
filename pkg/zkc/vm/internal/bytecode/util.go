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
	"math"
	"strings"
)

// RegistersToString formats a slice of registers as a string, joining their
// individual representations with the given separator.
func RegistersToString(registers []RegisterId, mapping Environment, separator string) string {
	var builder strings.Builder
	//
	for i, r := range registers {
		if i != 0 {
			builder.WriteString(separator)
		}
		//
		builder.WriteString(RegisterToString(r, mapping))
	}
	//
	return builder.String()
}

// RegisterVectorToString formats a register vector as a string, abbreviating
// vectors of more than two limbs.
func RegisterVectorToString(reg RegVec, mapping Environment) string {
	var (
		first = RegisterToString(reg.Base, mapping)
	)
	switch reg.Len {
	case 1:
		return first
	case 2:
		var second = RegisterToString(reg.Base+1, mapping)
		return fmt.Sprintf("%s;%s", first, second)
	default:
		var last = RegisterToString(reg.Base+reg.Len-1, mapping)
		return fmt.Sprintf("%s;,,;%s", first, last)
	}
}

// RegisterToString formats a single register as a string, using the given
// mapping to resolve its name (falling back to a numeric placeholder).
func RegisterToString(reg RegisterId, env Environment) string {
	if env == nil {
		return fmt.Sprintf("?%d", reg)
	}
	//
	return env.Register(reg).Name()
}

// ============================================================================
// Helpers
// ============================================================================

// CheckSmallArgs panics if the given arguments cannot be encoded as a "small"
// (single-byte) operand list, since wide read/write instructions are unsupported.
func CheckSmallArgs(args []RegisterId) {
	//
	if len(args) > math.MaxUint8 {
		panic("too many arguments")
	}
	//
	for _, r := range args {
		if r > math.MaxUint8 {
			panic("support wide read/write instructions")
		}
	}
}
