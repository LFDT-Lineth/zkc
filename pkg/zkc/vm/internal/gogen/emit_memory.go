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

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// emitMemRead emits a read from a readable memory (input ROM, static ROM or RAM
// scratch): decode the address, then load each data word into its target register
// (bit-width-checked).
func (g *generator) emitMemRead(c *code, fn *wordFunction, x *instruction.MemRead) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_READ from unknown module id %d", x.Id)
	}

	switch mi.role {
	case romInput, sromStatic, ramScratch:
	default:
		return fmt.Errorf("gogen: MEMORY_READ from non-readable memory %q", mi.name)
	}

	start, err := g.addrExpr(fn, mi, x.Address())
	if err != nil {
		return err
	}

	var inner error

	c.block(func() {
		c.linef("start := %s", start)

		for i, d := range x.Data() {
			w, e := g.regWidth(fn, d)
			if e != nil {
				inner = e
				return
			}

			idx := i

			c.block(func() {
				if mi.role == ramScratch {
					// RAM is zero-initialised: an unwritten cell reads 0.
					c.linef("val := memGet(%s, start+%d)", mi.varName, idx)
				} else {
					// ROM/SROM read is data[address]; OOB panics, like StaticArray.Read.
					c.linef("val := %s[start+%d]", mi.varName, idx)
				}

				if w < 64 {
					g.emitOverflowCheck(c, runtimeCheck(fmt.Sprintf("val >= (1 << %d)", w)),
						fmt.Sprintf("bit overflow (value exceeds u%d)", w))
				}

				c.linef("%s = val", reg(d))
			})
		}
	})

	return inner
}

// emitMemWrite emits a write to a writable memory (output WOM or RAM scratch):
// decode the address, then store each data word (checked against the memory's
// data-register width).  Both back onto memGrow (grow-on-write).
func (g *generator) emitMemWrite(c *code, fn *wordFunction, x *instruction.MemWrite) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_WRITE to unknown module id %d", x.Id)
	}

	switch mi.role {
	case womOutput, ramScratch:
	default:
		return fmt.Errorf("gogen: MEMORY_WRITE to non-writable memory %q", mi.name)
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

			idx := i
			w := dataRegs[i].Width() // mem write checks the memory data-register width

			c.block(func() {
				c.linef("val := %s", src)

				if w < 64 {
					g.emitOverflowCheck(c, widthCheckExpr(src, w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
				}

				c.linef("%s = memGrow(%s, start+%d, val)", mi.varName, mi.varName, idx)
			})
		}
	})

	return inner
}
