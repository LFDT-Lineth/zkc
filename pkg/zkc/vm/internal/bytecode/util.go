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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// RegistersToString formats a slice of registers as a string, joining their
// individual representations with the given separator.
func RegistersToString[W word.Word[W]](registers []RegisterId, mapping Environment[W], separator string) string {
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
func RegisterVectorToString[W word.Word[W]](reg RegisterVector, mapping Environment[W]) string {
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
// mapping to resolve its name (falling back to a numeric placeholder).  When
// the environment can supply a current value for the register (see
// Environment.ValueOf), that value is appended inline as "[0xVAL]"; this is how
// the debugger renders register values within an instruction's string.
func RegisterToString[W word.Word[W]](reg RegisterId, env Environment[W]) string {
	if env == nil {
		return fmt.Sprintf("?%d", reg)
	}
	//
	var name = env.Register(reg).Name()
	// Append the current value, when one is available.
	if val := env.ValueOf(reg); val.HasValue() {
		return fmt.Sprintf("%s[0x%s]", name, val.Unwrap().Text(16))
	}
	//
	return name
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
