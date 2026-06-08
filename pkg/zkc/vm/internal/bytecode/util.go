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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

func registersToString(registers []Reg, mapping SystemMap, separator string) string {
	var builder strings.Builder
	//
	for i, r := range registers {
		if i != 0 {
			builder.WriteString(separator)
		}
		//
		builder.WriteString(registerToString(r, mapping))
	}
	//
	return builder.String()
}

func registerVectorToString(reg RegVec, mapping SystemMap) string {
	var (
		first = registerToString(reg.Base, mapping)
	)
	switch reg.Len {
	case 1:
		return first
	case 2:
		var second = registerToString(reg.Base+1, mapping)
		return fmt.Sprintf("%s;%s", first, second)
	default:
		var last = registerToString(reg.Base+reg.Len-1, mapping)
		return fmt.Sprintf("%s;,,;%s", first, last)
	}
}

func registerToString(reg Reg, mapping SystemMap) string {
	if mapping == nil {
		return fmt.Sprintf("?%d", reg)
	}
	//
	return mapping.Register(register.NewId(uint(reg))).Name()
}
