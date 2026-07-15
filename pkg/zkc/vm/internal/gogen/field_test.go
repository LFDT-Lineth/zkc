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
	"reflect"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// fieldTestProgram hand-builds a word machine exercising the WordTypeF mod-P ops
// (no ZkC surface syntax produces them yet):
//
//	result[0] = x ⊕ y      (INT_ADDMOD_P, constant 0)
//	result[1] = x ⊖ y      (INT_SUBMOD_P, constant 0)
//	result[2] = x ⊗ y ⊗ 2  (INT_MULMOD_P, constant 2)
//
// over KoalaBear's prime, with u32 inputs read from data[0], data[1].
func fieldTestProgram[W vm.Word[W]]() vm.Program[W] {
	var (
		padding W
		two     W
		zero    W
		u8      = util.Some(uint(8))
		u32     = util.Some(uint(32))
	)

	two = vm.Const64[W](2)

	memRegs := func() []vm.Register[W] {
		return []vm.Register[W]{
			vm.NewInputRegister("address", u8, padding),
			vm.NewOutputRegister("word", u32, padding),
		}
	}
	// Function registers: x, y, the result z, and the materialised address
	// constants a0/a1/a2 (the compiler materialises constants via INT_ADD into
	// computed registers; the zero/one "const" registers read as 0 at runtime).
	regs := []vm.Register[W]{
		vm.NewComputedRegister("x", u32, padding), // r0
		vm.NewComputedRegister("y", u32, padding), // r1
		vm.NewComputedRegister("z", u32, padding), // r2
		vm.NewComputedRegister("a0", u8, padding), // r3
		vm.NewComputedRegister("a1", u8, padding), // r4
		vm.NewComputedRegister("a2", u8, padding), // r5
	}

	ids := func(is ...uint) []vm.RegisterId {
		out := make([]vm.RegisterId, len(is))
		for i, v := range is {
			out[i] = vm.RegisterId(v)
		}

		return out
	}

	loadConst := func(target uint, v uint64) vm.Bytecode[W] {
		return vm.LoadConst(vm.RegisterId(target), vm.Const64[W](v))
	}

	// One vector ending in RETURN: the machine only reloads the active vector
	// on non-sequential control flow, so sequential code must stay within a
	// single vector (exactly how the LowerBitwise helper bodies are built).
	code := vm.NewBytecodeVector[W](
		loadConst(3, 0),
		loadConst(4, 1),
		loadConst(5, 2),
		vm.MemRead[W](0, ids(3), ids(0)), // x = data[0]
		vm.MemRead[W](0, ids(4), ids(1)), // y = data[1]
		// result[0] = x ⊕ y
		vm.AddModP(vm.RegisterId(2), ids(0, 1), zero),
		vm.MemWrite[W](1, ids(3), ids(2)),
		// result[1] = x ⊖ y
		vm.SubModP(vm.RegisterId(2), ids(0, 1), zero),
		vm.MemWrite[W](1, ids(4), ids(2)),
		// result[2] = x ⊗ y ⊗ 2
		vm.MulModP(vm.RegisterId(2), ids(0, 1), two),
		vm.MemWrite[W](1, ids(5), ids(2)),
		vm.Return[W](),
	)

	return vm.NewBytecodeProgram(
		field.KOALABEAR_16,
		vm.NewBytecodeMemory[W]("data", vm.PUBLIC_READ_ONLY_MEMORY, memRegs()),
		vm.NewBytecodeMemory[W]("result", vm.PUBLIC_WRITE_ONCE_MEMORY, memRegs()),
		vm.NewBytecodeFunction("main", false, regs, code),
	)
}

// TestGenFieldOps differentially checks the mod-P chains against the reference
// executor, including operands at and above the modulus.
func TestGenFieldOps(t *testing.T) {
	const koalabear = 0x7f000001 // 2^31 - 2^24 + 1

	src, err := vm.GenerateGo(fieldTestProgram[vm.Uint](), vm.GoGenConfig{})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

	prog := buildProgram(t, src)

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
			inBytes := encodeInputs(fieldTestProgram[vm.Uint](), in)

			refOut, refErr := referenceRun(t, fieldTestProgram[vm.Uint](), inBytes)

			genOut, genErr := runProgram(t, prog, inBytes)
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
