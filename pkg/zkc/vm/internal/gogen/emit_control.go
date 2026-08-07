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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// labelName renders the Go label for a 2-D PC position.
func labelName(p pos) string { return fmt.Sprintf("L_%d_%d", p.macro, p.micro) }

// skipTarget computes the destination of a skip/skip_if at (vi, ci) skipping
// `skip` micro-instructions.  Per the VM (encoding.ProgramPoint.Skip), a skip
// advances the micro counter to ci+skip and then falls through one step, so the
// destination is ci+skip+1; if that lands past the end of the vector it falls
// through to the start of the next macro vector.
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
func collectLabels(code BytecodeVector) map[pos]bool {
	labels := map[pos]bool{}

	for vi, vec := range code {
		n := uint(len(vec.Bytecodes))
		for ci, insn := range vec.Bytecodes {
			switch x := insn.(type) {
			case *bytecode.Skip[word.Uint]:
				labels[skipTarget(uint(vi), uint(ci), uint(x.Skip), n)] = true
			case *bytecode.SkipIf[word.Uint]:
				labels[skipTarget(uint(vi), uint(ci), uint(x.Skip), n)] = true
			case *bytecode.Jmp[word.Uint]:
				labels[pos{uint(x.Target), 0}] = true
			case *bytecode.Switch[word.Uint]:
				for _, cse := range x.Cases {
					labels[skipTarget(uint(vi), uint(ci), uint(cse.Skip), n)] = true
				}
			case *bytecode.Dispatch[word.Uint]:
				for _, cse := range x.Cases {
					labels[skipTarget(uint(vi), uint(ci), uint(cse.Skip), n)] = true
				}
			}
		}
	}

	return labels
}

// condExpr renders the boolean Go expression under which a SkipIf takes its
// skip.  Register splitting lays limbs out most-significant first, so the
// lowest-indexed register (Base) holds the most-significant limb; vectors are
// therefore compared lexicographically from Base downwards, matching
// executeSkipIf_rv.  Two-limb elements compare their high limbs first.
func (g *generator) condExpr(fn *descFunction, x *bytecode.SkipIf[word.Uint]) (string, error) {
	lhsOps, err := g.registerOperands(fn, x.Left.Registers())
	if err != nil {
		return "", err
	}

	rhsOps, err := g.operand(fn, x.Right, uint(len(lhsOps)))
	if err != nil {
		return "", err
	}

	if len(lhsOps) != len(rhsOps) {
		return "", fmt.Errorf("gogen: skip_if compares vectors of differing length (%d vs %d)", len(lhsOps), len(rhsOps))
	}

	switch x.Op {
	case bytecode.CONDITION_EQ:
		return eqExpr(lhsOps, rhsOps), nil
	case bytecode.CONDITION_NEQ:
		return "!(" + eqExpr(lhsOps, rhsOps) + ")", nil
	case bytecode.CONDITION_LT:
		return ordExpr(lhsOps, rhsOps, "<"), nil
	case bytecode.CONDITION_GT:
		return ordExpr(lhsOps, rhsOps, ">"), nil
	case bytecode.CONDITION_LTEQ:
		return "!(" + ordExpr(lhsOps, rhsOps, ">") + ")", nil
	case bytecode.CONDITION_GTEQ:
		return "!(" + ordExpr(lhsOps, rhsOps, "<") + ")", nil
	default:
		return "", fmt.Errorf("gogen: unsupported skip condition 0x%x", uint(x.Op))
	}
}

// emitMultiwaySkip renders a multiway dispatch as a Go switch on the source
// register: each case jumps to its skip target, and a non-matching value falls
// through to the following instruction (so endOfFlow is NOT called).  The
// switch form (over an if-else chain) lets the Go compiler lower the dispatch
// to a jump table or binary search rather than a linear sequence of compares.
//
// A case value fits in 64 bits, so a wide source can only match when its high
// limb is zero; we therefore switch on the low limb under a high-limb-zero
// guard, leaving any wide non-zero source to fall through.
func (g *generator) emitMultiwaySkip(c *code, fn *descFunction, x *bytecode.Switch[word.Uint],
	vi, ci, vecLen uint) error {
	source, err := g.registerOperand(fn, x.Source)
	if err != nil {
		return err
	}
	//
	if source.wide() {
		c.linef("if %s == 0 {", source.hi)
	}
	//
	c.linef("switch %s {", source.expr)
	//
	for _, cse := range x.Cases {
		value, err := uintConst(cse.Value)
		if err != nil {
			return err
		}

		target := skipTarget(vi, ci, uint(cse.Skip), vecLen)
		//
		c.linef("case %d:", value)
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
	}
	//
	c.line("}")
	//
	if source.wide() {
		c.line("}")
	}
	//
	return nil
}

// emitDispatch renders a one-hot dispatch as a tagless switch over the case
// bits: control transfers to the target of the first case whose (1-bit)
// register is set, and falls through when none is.  This mirrors
// interpreter.executeDispatch.
func (g *generator) emitDispatch(c *code, fn *descFunction, x *bytecode.Dispatch[word.Uint],
	vi, ci, vecLen uint) error {
	c.line("switch {")
	//
	for _, cse := range x.Cases {
		bit, err := g.registerOperand(fn, cse.Bit)
		if err != nil {
			return err
		}

		target := skipTarget(vi, ci, uint(cse.Skip), vecLen)
		//
		c.linef("case %s != 0:", bit.expr)
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
	}
	//
	c.line("}")
	//
	return nil
}

// elemEq / elemOrd compare one (possibly two-limb) element pair as full values.
func elemEq(a, b operand) string {
	if !a.wide() && !b.wide() {
		return fmt.Sprintf("%s == %s", a.expr, b.expr)
	}

	return fmt.Sprintf("(%s == %s && %s == %s)", a.expr, b.expr, a.hiOr0(), b.hiOr0())
}

func elemOrd(a, b operand, op string) string {
	if !a.wide() && !b.wide() {
		return fmt.Sprintf("(%s %s %s)", a.expr, op, b.expr)
	}

	return fmt.Sprintf("(%s %s %s || (%s == %s && %s %s %s))",
		a.hiOr0(), op, b.hiOr0(), a.hiOr0(), b.hiOr0(), a.expr, op, b.expr)
}

// eqExpr renders elementwise equality of two operand lists.
func eqExpr(lhs, rhs []operand) string {
	parts := make([]string, len(lhs))
	for i := range lhs {
		parts[i] = elemEq(lhs[i], rhs[i])
	}

	return strings.Join(parts, " && ")
}

// ordExpr renders a strict lexicographic comparison (op is "<" or ">") of two
// operand lists.  The operands are ordered most-significant register first
// (index 0 is Base, the most-significant limb), so the comparison is decided by
// the most-significant differing element, falling through to less-significant
// elements only while the more-significant ones are equal.
func ordExpr(lhs, rhs []operand, op string) string {
	var build func(i int) string

	build = func(i int) string {
		if i == len(lhs)-1 {
			return elemOrd(lhs[i], rhs[i], op)
		}

		return fmt.Sprintf("(%s || (%s && %s))",
			elemOrd(lhs[i], rhs[i], op), elemEq(lhs[i], rhs[i]), build(i+1))
	}

	return build(0)
}
