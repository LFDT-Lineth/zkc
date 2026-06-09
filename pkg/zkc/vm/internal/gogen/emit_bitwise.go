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

package gogen

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// emitBitwise emits a WordTypeB bitwise/shift op into its single target register,
// mirroring executeAnd/Or/Xor/Not/Shl/Shr.  AND/OR/XOR and SHR map to the plain Go
// operators; NOT and SHL additionally mask to the operation bit-width (matching
// word.Not / word.Shl).  The result is then bit-width-checked against the target
// via emitStore (i.e. frame.Store).
func (g *generator) emitBitwise(c *code, fn *wordFunction, x *instruction.WordTypeB) error {
	store, err := g.buildStore(fn, register.NewVector(x.Target))
	if err != nil {
		return err
	}

	lhs, err := g.operand(fn, x.LeftSource)
	if err != nil {
		return err
	}

	// BIT_NOT is unary; the rest read a right operand.
	var rhs string
	if x.Op != opcode.BIT_NOT {
		if rhs, err = g.operand(fn, x.RightSource); err != nil {
			return err
		}
	}

	var valExpr string

	switch x.Op {
	case opcode.BIT_AND:
		valExpr = fmt.Sprintf("%s & %s", lhs, rhs)
	case opcode.BIT_OR:
		valExpr = fmt.Sprintf("%s | %s", lhs, rhs)
	case opcode.BIT_XOR:
		valExpr = fmt.Sprintf("%s ^ %s", lhs, rhs)
	case opcode.BIT_NOT:
		valExpr = maskExpr(fmt.Sprintf("^%s", lhs), x.Bitwidth)
	case opcode.BIT_SHL:
		valExpr = maskExpr(fmt.Sprintf("%s << %s", lhs, rhs), x.Bitwidth)
	case opcode.BIT_SHR:
		valExpr = fmt.Sprintf("%s >> %s", lhs, rhs)
	default:
		// INT_DIV / INT_REM are not on the supported path yet (§6.5).
		return fmt.Errorf("gogen: unsupported bitwise op %s", opName(x.Op))
	}

	g.emitStoreExpr(c, store, valExpr)

	return nil
}

// maskExpr masks expr to the low bitwidth bits, mirroring word.mask64: a width of
// 64 or more needs no mask (the full word is already in range, and Go's shift
// already yields 0 when the count reaches the word width).
func maskExpr(expr string, bitwidth uint) string {
	if bitwidth >= 64 {
		return expr
	}

	return fmt.Sprintf("(%s) & ((1 << %d) - 1)", expr, bitwidth)
}
