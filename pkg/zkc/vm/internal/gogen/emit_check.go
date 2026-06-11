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

import "fmt"

// Runtime failures (width-check violations, underflow, division by zero, FAIL)
// are exceptional: generated functions panic with a `failure` value which the
// Run entry point recovers into its error result.  This keeps every generated
// signature free of error plumbing.

// checkWidth enforces a store of `op` into a width-w register, mirroring
// StackFrame.Store, unless the bound proves the check dead (omitted entirely)
// or the exact value proves it always fires (unconditional failure).  Wide
// (lo/hi pair) values and wide targets check the relevant limbs.
func (g *generator) checkWidth(c *code, op operand, w uint) {
	if w >= 128 || fits(op.max, w) {
		return // the value provably fits the target
	}

	msg := widthFailMsg(w)
	if op.val != nil {
		// Exact value, known too wide: the store always fails.
		c.linef("fail(%q) // %s never fits u%d", msg, op.expr, w)
		return
	}

	switch {
	case !op.wide():
		// A narrow value always fits a 64-bit-or-wider target.
		if w >= 64 {
			return
		}

		c.linef("if %s >= 1<<%d {", op.expr, w)
	case w > 64:
		c.linef("if %s >= 1<<%d {", op.hi, w-64)
	case w == 64:
		c.linef("if %s != 0 {", op.hi)
	default:
		c.linef("if %s != 0 || %s >= 1<<%d {", op.hi, op.expr, w)
	}

	c.linef("fail(%q)", msg)
	c.line("}")
}

func widthFailMsg(w uint) string {
	return fmt.Sprintf("bit overflow (value exceeds u%d)", w)
}

// emitFailureHelpers writes the failure type and the fail helper, which every
// generated program uses (the fall-off-the-end guard alone guarantees that).
func emitFailureHelpers(c *code) {
	c.line("// failure is a ZkC execution failure (a rejected trace, not a bug); Run")
	c.line("// recovers it into its error result.")
	c.line("type failure string")
	c.line("")
	c.line("func (e failure) Error() string { return string(e) }")
	c.line("")
	c.line("func fail(msg string) { panic(failure(msg)) }")
	c.line("")
}
