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

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// emitCall emits a Go call to the callee function, mirroring CallStack.Enter /
// Leave: arguments are width-checked against the callee's input registers, and
// returns are width-checked against the caller's target registers.  Checks the
// bounds prove dead are omitted, so the common case is a plain Go call.  A
// two-limb register expands to two parameters / results.
func (g *generator) emitCall(c *code, fn *wordFunction, x *instruction.Call) error {
	callee, ok := g.funcByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: CALL to non-function module id %d", x.Id)
	}

	calleeInputs := callee.Inputs()
	if len(x.Arguments) != len(calleeInputs) {
		return fmt.Errorf("gogen: CALL argument count mismatch (%d vs %d) for %q",
			len(x.Arguments), len(calleeInputs), callee.Name())
	}
	// A call may take FEWER returns than the callee has outputs: CallStack.Leave
	// copies per call.Returns entry, silently discarding trailing outputs.
	if len(x.Returns) > int(callee.NumOutputs()) {
		return fmt.Errorf("gogen: CALL return count mismatch (%d vs %d) for %q",
			len(x.Returns), callee.NumOutputs(), callee.Name())
	}

	args, err := g.operands(fn, x.Arguments)
	if err != nil {
		return err
	}
	// Argument width checks against the callee parameter widths (Enter), then
	// expansion: a wide parameter takes both limbs, a narrow one takes the low
	// limb (the check just proved the high limb empty).
	argExprs := []string{}

	for i, arg := range args {
		in := calleeInputs[i]
		if in.IsNative() {
			return fmt.Errorf("gogen: native parameter in %q unsupported", callee.Name())
		}

		g.checkWidth(c, arg, in.Width())
		argExprs = append(argExprs, arg.expr)

		if in.Width() > 64 {
			argExprs = append(argExprs, arg.hiOr0())
		}
	}

	call := fmt.Sprintf("%s(%s)", goFuncName(callee), strings.Join(argExprs, ", "))
	if callee.NumOutputs() == 0 {
		c.line(call)
		return nil
	}
	// Map each taken return onto its caller target; trailing outputs (and the
	// extra tuple slots of wide outputs feeding narrow logic) are planned
	// below.  The plain tuple assignment applies only when every taken return
	// matches its target's shape with no check; anything else routes through
	// block-scoped temporaries and storeValue.
	type ret struct {
		target   limb
		outWidth uint
		outWide  bool
	}

	rets := make([]ret, len(x.Returns))
	direct := true

	for i, target := range x.Returns {
		l, err := g.limbOf(fn, target)
		if err != nil {
			return err
		}

		out := callee.Register(calleeOutput(callee, i))
		if out.IsNative() {
			return fmt.Errorf("gogen: native output in %q unsupported", callee.Name())
		}

		ow := out.Width()
		rets[i] = ret{target: l, outWidth: ow, outWide: ow > 64}
		// Shape mismatch or a surviving width check forces the temp path.
		if (ow > 64) != (l.width > 64) || ow > l.width {
			direct = false
		}
	}
	// Discarded trailing outputs become blanks (one per tuple slot).
	discards := []string{}

	for i := len(x.Returns); i < int(callee.NumOutputs()); i++ {
		discards = append(discards, "_")
		if callee.Register(calleeOutput(callee, i)).Width() > 64 {
			discards = append(discards, "_")
		}
	}

	if direct {
		targets := []string{}

		for _, r := range rets {
			targets = append(targets, r.target.lo())
			if r.target.width > 64 {
				targets = append(targets, r.target.hiName())
			}

			g.iv.assign(r.target.id, widthMax(r.outWidth))
		}

		c.linef("%s = %s", strings.Join(append(targets, discards...), ", "), call)

		return nil
	}

	var inner error

	c.block(func() {
		tmps := []string{}
		vals := make([]operand, len(rets))

		for i, r := range rets {
			lo := fmt.Sprintf("t%d", i)
			vals[i] = operand{expr: lo, max: widthMax(r.outWidth)}
			tmps = append(tmps, lo)

			if r.outWide {
				hi := fmt.Sprintf("t%d_1", i)
				vals[i].hi = hi
				tmps = append(tmps, hi)
			}
		}

		c.linef("%s := %s", strings.Join(append(tmps, discards...), ", "), call)

		for i, r := range rets {
			if inner = g.storeValue(c, storeView{single: &rets[i].target, total: r.target.width}, vals[i]); inner != nil {
				return
			}
		}
	})

	return inner
}

// calleeOutput returns the register id of the callee's i-th output register
// (outputs follow inputs in the register file).
func calleeOutput(callee *wordFunction, i int) register.Id {
	return register.NewId(callee.NumInputs() + uint(i))
}
