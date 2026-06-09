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
	"strconv"
	"strings"
)

type checkKind uint8

const (
	overflowCheck checkKind = iota
	underflowCheck
)

type checkExpr struct {
	expr  string
	known bool
	value bool
}

type valueInfo struct {
	known bool
	value uint64
}

func runtimeCheck(expr string) checkExpr {
	return checkExpr{expr: expr}
}

func knownCheck(expr string, value bool) checkExpr {
	return checkExpr{expr: expr, known: true, value: value}
}

func unknownValue() valueInfo {
	return valueInfo{}
}

func knownValue(value uint64) valueInfo {
	return valueInfo{known: true, value: value}
}

func widthCheckExpr(expr string, width uint) checkExpr {
	cond := fmt.Sprintf("%s >= (1 << %d)", expr, width)
	if value, ok := uint64Literal(expr); ok {
		return knownCheck(cond, uint64ExceedsWidth(value, width))
	}

	return runtimeCheck(cond)
}

func (v valueInfo) widthCheckExpr(expr string, width uint) checkExpr {
	if v.known {
		return knownCheck(fmt.Sprintf("%s >= (1 << %d)", expr, width), uint64ExceedsWidth(v.value, width))
	}

	return widthCheckExpr(expr, width)
}

func (g *generator) emitOverflowCheck(c *code, cond checkExpr, msg string) {
	g.emitCheck(c, overflowCheck, cond, msg)
}

func (g *generator) emitUnderflowCheck(c *code, cond checkExpr, msg string) {
	g.emitCheck(c, underflowCheck, cond, msg)
}

func (g *generator) emitCheck(c *code, kind checkKind, cond checkExpr, msg string) {
	if cond.known {
		// The outcome is fixed at generation time: a check that can never fire is
		// omitted entirely (no runtime test, no dead comment); one that always fires
		// becomes an unconditional error.
		if cond.value {
			c.commentf("compiled optimized: %s is always true, so this %s always fails.",
				cond.expr, kind.String())
			c.line(g.returnErr(fmt.Sprintf("fmt.Errorf(%q)", msg)))
		}

		return
	}

	helper := "checkOverflow"
	if kind == underflowCheck {
		helper = "checkUnderflow"
		g.usesUnderflowCheck = true
	} else {
		g.usesOverflowCheck = true
	}

	c.linef("%s(%s, %q)", helper, cond.expr, msg)
}

func (g *generator) emitCheckHelpers(c *code) {
	if g.usesCheckPanic() {
		c.line("type checkError string")
		c.line("")
		c.line("func (e checkError) Error() string {")
		c.line("return string(e)")
		c.line("}")
		c.line("")
	}

	if g.usesOverflowCheck {
		c.line("func checkOverflow(overflow bool, msg string) {")
		c.line("if overflow {")
		c.line("panic(checkError(msg))")
		c.line("}")
		c.line("}")
		c.line("")
	}

	if g.usesUnderflowCheck {
		c.line("func checkUnderflow(underflow bool, msg string) {")
		c.line("if underflow {")
		c.line("panic(checkError(msg))")
		c.line("}")
		c.line("}")
		c.line("")
	}
}

func (g *generator) usesCheckPanic() bool {
	return g.usesOverflowCheck || g.usesUnderflowCheck
}

func (k checkKind) String() string {
	switch k {
	case underflowCheck:
		return "underflow"
	default:
		return "overflow"
	}
}

func uint64Literal(expr string) (uint64, bool) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "uint64(") && strings.HasSuffix(expr, ")") {
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "uint64("), ")"))
	}

	value, err := strconv.ParseUint(expr, 0, 64)

	return value, err == nil
}

func uint64ExceedsWidth(value uint64, width uint) bool {
	return width < 64 && value >= (uint64(1)<<width)
}
