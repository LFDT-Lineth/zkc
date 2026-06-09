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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// labelName renders the Go label for a 2-D PC position.
func labelName(p pos) string { return fmt.Sprintf("L_%d_%d", p.macro, p.micro) }

// skipTarget computes the destination of a skip/skip_if at (vi, ci) skipping
// `skip` micro-instructions.  Per the VM (machine/base.go), a skip advances the
// micro counter to ci+skip and then falls through one step, so the destination
// is ci+skip+1; if that lands past the end of the vector it falls through to the
// start of the next macro vector.
func skipTarget(vi, ci, skip, vecLen uint) pos {
	micro := ci + skip + 1
	if micro >= vecLen {
		return pos{vi + 1, 0}
	}

	return pos{vi, micro}
}

// collectLabels gathers every 2-D PC position targeted by a skip or jump, so the
// emitter knows exactly which positions need a Go label (Go rejects unused
// labels, so we must not over-emit).
func collectLabels(code wordCode) map[pos]bool {
	labels := map[pos]bool{}

	for vi, vec := range code {
		n := uint(len(vec.Codes))
		for ci, insn := range vec.Codes {
			switch x := insn.(type) {
			case *instruction.Skip:
				labels[skipTarget(uint(vi), uint(ci), x.Skip, n)] = true
			case *instruction.SkipIf:
				labels[skipTarget(uint(vi), uint(ci), x.Skip, n)] = true
			case *instruction.Jump:
				labels[pos{x.Immediate, 0}] = true
			}
		}
	}

	return labels
}

// condExpr renders the boolean Go expression under which a SkipIf takes its
// skip.  Vectors are compared lexicographically with the most-significant
// register at the highest index, matching machine/base.go's cmp.
func (g *generator) condExpr(fn *wordFunction, x *instruction.SkipIf) (string, error) {
	lhs, err := g.operands(fn, x.Left.Registers())
	if err != nil {
		return "", err
	}

	rhs, err := g.operands(fn, x.Right.Registers())
	if err != nil {
		return "", err
	}

	if len(lhs) != len(rhs) {
		return "", fmt.Errorf("gogen: skip_if compares vectors of differing length (%d vs %d)", len(lhs), len(rhs))
	}

	switch x.Cond {
	case opcode.EQ:
		return eqExpr(lhs, rhs), nil
	case opcode.NEQ:
		return "!(" + eqExpr(lhs, rhs) + ")", nil
	case opcode.LT:
		return ordExpr(lhs, rhs, "<"), nil
	case opcode.GT:
		return ordExpr(lhs, rhs, ">"), nil
	case opcode.LTEQ:
		return "!(" + ordExpr(lhs, rhs, ">") + ")", nil
	case opcode.GTEQ:
		return "!(" + ordExpr(lhs, rhs, "<") + ")", nil
	default:
		return "", fmt.Errorf("gogen: unsupported skip condition 0x%x", uint(x.Cond))
	}
}

// eqExpr renders elementwise equality of two operand lists.
func eqExpr(lhs, rhs []string) string {
	parts := make([]string, len(lhs))
	for i := range lhs {
		parts[i] = fmt.Sprintf("%s == %s", lhs[i], rhs[i])
	}

	return strings.Join(parts, " && ")
}

// ordExpr renders a strict lexicographic comparison (op is "<" or ">") of two
// operand lists, most significant register first.
func ordExpr(lhs, rhs []string, op string) string {
	var build func(i int) string

	build = func(i int) string {
		if i == 0 {
			return fmt.Sprintf("(%s %s %s)", lhs[0], op, rhs[0])
		}

		return fmt.Sprintf("(%s %s %s || (%s == %s && %s))",
			lhs[i], op, rhs[i], lhs[i], rhs[i], build(i-1))
	}

	return build(len(lhs) - 1)
}
