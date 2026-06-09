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
)

// emitCall emits a Go call to the callee function, mirroring CallStack.Enter /
// Leave: arguments are width-checked against the callee's input registers, and
// returns are width-checked against the caller's target registers.
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

	if len(x.Returns) != int(callee.NumOutputs()) {
		return fmt.Errorf("gogen: CALL return count mismatch (%d vs %d) for %q",
			len(x.Returns), callee.NumOutputs(), callee.Name())
	}

	args, err := g.operands(fn, x.Arguments)
	if err != nil {
		return err
	}

	var inner error

	c.block(func() {
		// Argument width checks against the callee parameter widths.
		for i, arg := range args {
			if w := calleeInputs[i].Width(); !calleeInputs[i].IsNative() && w < 64 {
				g.emitOverflowCheck(c, widthCheckExpr(arg, w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
			}
		}

		call := fmt.Sprintf("%s(%s)", goFuncName(callee), strings.Join(args, ", "))
		if len(x.Returns) == 0 {
			c.linef("if e := %s; e != nil {", call)
			c.line(g.returnErr("e"))
			c.line("}")

			return
		}
		// Capture returns in temporaries, then width-check and assign them into
		// the caller's target registers.
		tmps := make([]string, len(x.Returns))
		for i := range x.Returns {
			tmps[i] = fmt.Sprintf("ret%d", i)
		}

		c.linef("%s, e := %s", strings.Join(tmps, ", "), call)
		c.line("if e != nil {")
		c.line(g.returnErr("e"))
		c.line("}")

		for i, target := range x.Returns {
			w, e := g.regWidth(fn, target)
			if e != nil {
				inner = e
				return
			}

			if w < 64 {
				g.emitOverflowCheck(c, widthCheckExpr(tmps[i], w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
			}

			c.linef("%s = %s", reg(target), tmps[i])
		}
	})

	return inner
}
