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

// FieldArith encodes a modular field-arithmetic operation, computing
// "target = sources[0] op ... op sources[n-1] op constant" reduced modulo the
// surrounding machine's prime characteristic.  The operation is identified by
// Op, which is one of ADDMOD_P, SUBMOD_P or MULMOD_P.  Unlike the integer Arith
// instruction the result always fits within a single (native) target register,
// so no cast check is ever required.
type FieldArith[W word.Word[W]] struct {
	// Op selects the operation (ADDMOD_P, SUBMOD_P or MULMOD_P).
	Op Operation
	// Target receives the result.
	Target RegisterId
	// Sources are the operand registers, with Sources[0] the leftmost operand.
	Sources []RegisterId
	// Constant is folded into the operation (the identity element when unused:
	// zero for ADDMOD_P / SUBMOD_P, one for MULMOD_P).
	Constant W
}

// Uses implementation for Bytecode interface.
func (p *FieldArith[W]) Uses() []RegisterId {
	return p.Sources
}

// Definitions implementation for Bytecode interface.
func (p *FieldArith[W]) Definitions() []RegisterId {
	return []RegisterId{p.Target}
}

// Validate implementation for Bytecode interface.
func (p *FieldArith[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *FieldArith[W]) String(env Environment[W]) string {
	var (
		builder        strings.Builder
		symbol, prefix = p.Op.Symbol(), p.Op.Prefix()
		cz             = IsUnusedConstant(p.Op, p.Constant)
	)
	//
	builder.WriteString(prefix)
	builder.WriteString(" ")
	builder.WriteString(RegisterToString(p.Target, env))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(p.Sources, env, symbol))
	// Append the constant operand unless it is the (elided) identity element.
	if !cz {
		if len(p.Sources) > 0 {
			builder.WriteString(symbol)
		}
		//
		fmt.Fprintf(&builder, "0x%s", p.Constant.Text(16))
	}
	//
	return builder.String()
}
