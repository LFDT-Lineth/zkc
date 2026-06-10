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
package gogen_test

import (
	"math/big"
	"os/exec"
	"reflect"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
)

// fieldMachine hand-builds a word machine exercising the WordTypeF mod-P ops
// (no ZkC surface syntax produces them yet):
//
//	result[0] = x ⊕ y      (INT_ADDMOD_P, constant 0)
//	result[1] = x ⊖ y      (INT_SUBMOD_P, constant 0)
//	result[2] = x ⊗ y ⊗ 2  (INT_MULMOD_P, constant 2)
//
// over KoalaBear's prime, with u32 inputs read from data[0], data[1].
func fieldMachine() *vm.WordMachine[vm.Uint] {
	var (
		padding big.Int
		two     vm.Uint
		zero    vm.Uint
	)

	two = two.SetUint64(2)

	memRegs := func() []register.Register {
		return []register.Register{
			register.NewInput("address", 8, padding),
			register.NewOutput("word", 32, padding),
		}
	}
	// Function registers: x, y, the result z, and the materialised address
	// constants a0/a1/a2 (the compiler materialises constants via INT_ADD into
	// computed registers; the zero/one "const" registers read as 0 at runtime).
	regs := []register.Register{
		register.NewComputed("x", 32, padding), // r0
		register.NewComputed("y", 32, padding), // r1
		register.NewComputed("z", 32, padding), // r2
		register.NewComputed("a0", 8, padding), // r3
		register.NewComputed("a1", 8, padding), // r4
		register.NewComputed("a2", 8, padding), // r5
	}

	rid := func(i uint) register.Id { return register.NewId(i) }
	ids := func(is ...uint) []register.Id {
		out := make([]register.Id, len(is))
		for i, v := range is {
			out[i] = rid(v)
		}

		return out
	}

	uconst := func(v uint64) vm.Uint {
		var w vm.Uint
		return w.SetUint64(v)
	}
	loadConst := func(target uint, v uint64) instruction.Word {
		return instruction.NewWordTypeA(opcode.INT_ADD, register.NewVector(rid(target)), nil, uconst(v))
	}

	// One vector ending in RETURN: the machine only reloads the active vector
	// on non-sequential control flow, so sequential code must stay within a
	// single vector (exactly how the LowerBitwise helper bodies are built).
	code := []instruction.Vector[instruction.Word]{{Codes: []instruction.Word{
		loadConst(3, 0),
		loadConst(4, 1),
		loadConst(5, 2),
		instruction.NewMemRead(0, ids(3), ids(0)), // x = data[0]
		instruction.NewMemRead(0, ids(4), ids(1)), // y = data[1]
		// result[0] = x ⊕ y
		instruction.NewWordTypeF(opcode.INT_ADDMOD_P, rid(2), ids(0, 1), zero),
		instruction.NewMemWrite(1, ids(3), ids(2)),
		// result[1] = x ⊖ y
		instruction.NewWordTypeF(opcode.INT_SUBMOD_P, rid(2), ids(0, 1), zero),
		instruction.NewMemWrite(1, ids(4), ids(2)),
		// result[2] = x ⊗ y ⊗ 2
		instruction.NewWordTypeF(opcode.INT_MULMOD_P, rid(2), ids(0, 1), two),
		instruction.NewMemWrite(1, ids(5), ids(2)),
		instruction.NewReturn(),
	}}}

	return vm.NewWordMachine[vm.Uint](field.KOALABEAR_16,
		vm.NewInputMemory[vm.Uint]("data", true, memRegs()),
		vm.NewOutputMemory[vm.Uint]("result", true, memRegs()),
		vm.NewFunction("main", false, regs, code),
	)
}

// TestGenFieldOps differentially checks the mod-P chains against the reference
// executor, including operands at and above the modulus.
func TestGenFieldOps(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	const koalabear = 0x7f000001 // 2^31 - 2^24 + 1

	src, err := vm.GenerateGo(fieldMachine(), vm.GoGenConfig{})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

	prog := buildProgram(t, goBin, src)

	vectors := []map[string][]uint64{
		{"data": {0, 0}},
		{"data": {5, 7}},
		{"data": {3, 9}},                         // sub wraps: 3 - 9 mod p
		{"data": {koalabear - 1, koalabear - 1}}, // p-1 in both lanes
		{"data": {koalabear, 1}},                 // operand equal to p (reduces to 0)
		{"data": {0xFFFFFFFF, 0xFFFFFFFF}},       // operands above p (reduced by the ops)
	}

	for _, in := range vectors {
		t.Run(inputName(in), func(t *testing.T) {
			refOut, refErr := referenceRun(t, fieldMachine(), in)

			genOut, genErr := runProgram(t, prog, in)
			if refErr != genErr {
				t.Fatalf("error mismatch: reference err=%v, generated err=%v (in=%v)", refErr, genErr, in)
			}

			if refErr {
				return
			}

			if !reflect.DeepEqual(refOut, genOut) {
				t.Fatalf("output mismatch (in=%v):\n  reference=%v\n  generated=%v", in, refOut, genOut)
			}
		})
	}
}
