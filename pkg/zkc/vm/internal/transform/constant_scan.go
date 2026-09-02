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
package transform

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// scanConstantRegisters identifies registers of a function which hold a
// compile-time constant: registers with exactly one defining bytecode, where
// that bytecode is a single-target ADD/SUB/MUL whose sources (if any) are all
// themselves constant.  This covers plain constant loads (an ADD with no
// sources) and simple constant arithmetic over them (e.g. "64 - n" where n was
// loaded as a constant, as arises after inlining a call with literal
// arguments).  A register whose folded value would not fit its declared
// bitwidth is not reported, since executing the defining bytecode would fail
// anyway.
//
// The result maps each such register to its value.  Registers written more
// than once, written by any other bytecode form, or function inputs (written
// by no bytecode) are never reported.
func scanConstantRegisters[W word.Word[W]](fn *descriptor.Function[W]) map[bytecode.RegisterId]W {
	var (
		regs   = fn.Registers()
		writes = make(map[bytecode.RegisterId]uint)
		defOf  = make(map[bytecode.RegisterId]*bytecode.Arith[W])
	)
	// Record, for every register, its write count and (single-target Arith)
	// defining bytecode.
	for _, vec := range fn.Vectors() {
		for _, insn := range vec.Bytecodes {
			for _, target := range insn.Definitions() {
				writes[target]++
				//
				if a, ok := insn.(*bytecode.Arith[W]); ok && len(a.Target) == 1 {
					defOf[target] = a
				} else {
					defOf[target] = nil
				}
			}
		}
	}
	// Iterate to a fixpoint, folding definitions whose sources are all known.
	consts := make(map[bytecode.RegisterId]W)
	//
	for changed := true; changed; {
		changed = false
		//
		for r, a := range defOf {
			if _, done := consts[r]; done || a == nil || writes[r] != 1 {
				continue
			}
			//
			reg := regs[r]
			if reg.IsNative() {
				continue
			}
			//
			value, ok := foldConstantArith(a, consts)
			if !ok {
				continue
			}
			// Reject values which do not fit the target register.
			if value.Sign() < 0 || value.BitLen() > int(reg.Bitwidth().Unwrap()) {
				continue
			}
			//
			var w W

			consts[r] = w.SetBigInt(value)
			changed = true
		}
	}
	//
	return consts
}

// foldConstantArith evaluates a single-target ADD/SUB/MUL bytecode whose
// sources are all in consts, following the Arith semantics
// "target = sources[0] op sources[1] op ... op constant".
func foldConstantArith[W word.Word[W]](a *bytecode.Arith[W],
	consts map[bytecode.RegisterId]W,
) (*big.Int, bool) {
	switch a.Op {
	case bytecode.OP_ADD, bytecode.OP_SUB, bytecode.OP_MUL:
		// supported
	default:
		return nil, false
	}
	//
	values := make([]*big.Int, 0, len(a.Source)+1)
	//
	for _, src := range a.Source {
		w, ok := consts[src]
		if !ok {
			return nil, false
		}
		//
		values = append(values, w.BigInt())
	}
	//
	values = append(values, a.Constant.BigInt())
	//
	acc := new(big.Int).Set(values[0])
	//
	for _, v := range values[1:] {
		switch a.Op {
		case bytecode.OP_ADD:
			acc.Add(acc, v)
		case bytecode.OP_SUB:
			acc.Sub(acc, v)
		case bytecode.OP_MUL:
			acc.Mul(acc, v)
		}
	}
	//
	return acc, true
}
