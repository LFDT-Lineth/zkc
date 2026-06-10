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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// emitMemRead emits a read from a readable memory: decode the address, then
// load each data word into its target register (executeMemRead: each load is a
// frame.Store, i.e. checked against the TARGET register width).  What is known
// about the loaded value depends on the role:
//
//   - input ROM: untrusted (raw program input) — full uint64 range;
//   - static ROM: contents are baked, so the bound is their maximum;
//   - RAM: cells were width-checked against the data registers when written
//     (and unwritten cells read 0), so the data width bounds the value.
func (g *generator) emitMemRead(c *code, fn *wordFunction, x *instruction.MemRead) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_READ from unknown module id %d", x.Id)
	}

	if mi.role == womOutput {
		return fmt.Errorf("gogen: MEMORY_READ from write-once memory %q", mi.name)
	}

	start, err := g.addrExpr(fn, mi, x.Address())
	if err != nil {
		return err
	}

	dataRegs := mi.geom.DataRegisters()

	var inner error

	c.block(func() {
		c.linef("start := %s", start)

		for i, d := range x.Data() {
			l, e := g.limbOf(fn, d)
			if e != nil {
				inner = e
				return
			}

			var (
				bound *big.Int
				expr  string
			)

			switch mi.role {
			case sromStatic:
				// Baked contents: the read is a direct index (OOB panics, like
				// StaticArray.Read) and the bound is the largest baked value.
				expr = fmt.Sprintf("%s[start+%d]", mi.varName, i)
				bound = maxContents(mi.contents)
			case ramScratch:
				expr = fmt.Sprintf("memGet(%s, start+%d)", mi.varName, i)
				bound = widthMax(dataRegs[i].Width())
			case bramScratch:
				expr = fmt.Sprintf("%s.get(start + %d)", mi.varName, i)
				bound = widthMax(dataRegs[i].Width())
			default: // input ROM: untrusted contents
				expr = fmt.Sprintf("%s[start+%d]", mi.varName, i)
				bound = widthMax(64)
			}

			g.assignSingle(c, l, operand{expr: expr, max: bound})
		}
	})

	return inner
}

// emitMemWrite emits a write to a writable memory (executeMemWrite): decode
// the address, width-check each value against the memory's data register, and
// store grow-on-write.
func (g *generator) emitMemWrite(c *code, fn *wordFunction, x *instruction.MemWrite) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_WRITE to unknown module id %d", x.Id)
	}

	switch mi.role {
	case womOutput, ramScratch, bramScratch:
	default:
		return fmt.Errorf("gogen: MEMORY_WRITE to read-only memory %q", mi.name)
	}

	start, err := g.addrExpr(fn, mi, x.Address())
	if err != nil {
		return err
	}

	dataRegs := mi.geom.DataRegisters()
	if len(x.Data()) != len(dataRegs) {
		return fmt.Errorf("gogen: MEMORY_WRITE data lines mismatch (%d vs %d)", len(x.Data()), len(dataRegs))
	}

	var inner error

	c.block(func() {
		c.linef("start := %s", start)

		for i, s := range x.Data() {
			src, e := g.operand(fn, s)
			if e != nil {
				inner = e
				return
			}

			if !dataRegs[i].IsNative() {
				g.checkWidth(c, src, dataRegs[i].Width())
			}

			if mi.role == bramScratch {
				c.linef("%s.set(start+%d, %s)", mi.varName, i, src.expr)
			} else {
				c.linef("%s = memGrow(%s, start+%d, %s)", mi.varName, mi.varName, i, src.expr)
			}
		}
	})

	return inner
}

// addrExpr mirrors memory.Geometry.Decode: fold the address registers
// big-endian by their geometry widths (in uint64 arithmetic, as the oracle
// does), then multiply by the number of data lines.
func (g *generator) addrExpr(fn *wordFunction, mi memInfo, addr []register.Id) (string, error) {
	addrRegs := mi.geom.AddressRegisters()
	if len(addr) != len(addrRegs) {
		return "", fmt.Errorf("gogen: address lines mismatch (%d vs %d) for %q", len(addr), len(addrRegs), mi.name)
	}

	expr := "uint64(0)"

	for i, id := range addr {
		src, err := g.operand(fn, id)
		if err != nil {
			return "", err
		}
		// The oracle packs addresses in uint64 arithmetic; a wide register
		// whose high limb cannot be proven empty is out of scope.
		if src.wide() {
			return "", fmt.Errorf("gogen: address register wider than 64 bits unsupported")
		}

		if i == 0 {
			expr = src.expr
		} else {
			expr = fmt.Sprintf("(%s<<%d | %s)", expr, addrRegs[i].Width(), src.expr)
		}
	}
	// A constant-only address must still be typed uint64.
	if _, isLit := new(big.Int).SetString(expr, 0); isLit {
		expr = fmt.Sprintf("uint64(%s)", expr)
	}

	if lines := mi.geom.DataLines(); lines != 1 {
		expr = fmt.Sprintf("(%s) * %d", expr, lines)
	}

	return expr, nil
}

// maxContents bounds the values of a baked static memory.
func maxContents(contents []uint64) *big.Int {
	max := uint64(0)
	for _, v := range contents {
		if v > max {
			max = v
		}
	}

	return new(big.Int).SetUint64(max)
}
