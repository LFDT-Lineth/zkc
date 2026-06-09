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
package bytecode

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

func TestArithVectorRoundTripWithConstant(t *testing.T) {
	insn := MulVecConst(
		registerIds(4, 5),
		registerIds(1, 2),
		word.Const64[word.Uint64](7),
	)
	codes := insn.Codes(0)
	decoded, n := decodeArith[word.Uint64](0, codes)
	arith := decoded.(*Arith[word.Uint64])
	//
	if n != uint32(len(codes)) {
		t.Fatalf("invalid decoded width: got %d, want %d", n, len(codes))
	}
	if arith.Op != arithop_MUL {
		t.Fatalf("invalid op: got %v, want %v", arith.Op, arithop_MUL)
	}
	if arith.Constant.Cmp64(7) != 0 {
		t.Fatalf("invalid constant: got %s, want 7", arith.Constant.Text(10))
	}
	assertRegs(t, arith.Target, []Reg{4, 5})
	assertRegs(t, arith.Source, []Reg{1, 2})
}

func TestArithVectorRoundTripWithNoSources(t *testing.T) {
	insn := AddVecConst(
		registerIds(4, 5),
		nil,
		word.Const64[word.Uint64](0x1234),
	)
	codes := insn.Codes(0)
	decoded, n := decodeArith[word.Uint64](0, codes)
	arith := decoded.(*Arith[word.Uint64])
	//
	if n != uint32(len(codes)) {
		t.Fatalf("invalid decoded width: got %d, want %d", n, len(codes))
	}
	if arith.Constant.Cmp64(0x1234) != 0 {
		t.Fatalf("invalid constant: got %s, want 0x1234", arith.Constant.Text(16))
	}
	assertRegs(t, arith.Target, []Reg{4, 5})
	assertRegs(t, arith.Source, nil)
}

func assertRegs(t *testing.T, got, want []Reg) {
	t.Helper()
	//
	if len(got) != len(want) {
		t.Fatalf("invalid registers: got %v, want %v", got, want)
	}
	//
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("invalid registers: got %v, want %v", got, want)
		}
	}
}

func registerIds(ids ...uint) []register.Id {
	regs := make([]register.Id, len(ids))
	//
	for i, id := range ids {
		regs[i] = register.NewId(id)
	}
	//
	return regs
}
