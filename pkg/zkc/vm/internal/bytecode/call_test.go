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
	"math"
	"testing"
)

func TestEncodeCallRejectsWideFields(t *testing.T) {
	expectPanic(t, func() {
		encodeCall(math.MaxUint8+1, nil, nil)
	})
	expectPanic(t, func() {
		encodeCall(0, make([]Reg, math.MaxUint8+1), nil)
	})
	expectPanic(t, func() {
		encodeCall(0, nil, make([]Reg, math.MaxUint8+1))
	})
}

func TestEncodeCallRoundTripMaxCounts(t *testing.T) {
	var (
		args    = make([]Reg, math.MaxUint8)
		returns = make([]Reg, math.MaxUint8)
	)
	//
	for i := range args {
		args[i] = Reg(i)
		returns[i] = Reg(math.MaxUint8 - i)
	}
	//
	codes := encodeCall(math.MaxUint8, args, returns)
	id, decodedArgs, decodedReturns, n := decodeCallOperands(0, codes)
	//
	if n != uint32(len(codes)) {
		t.Fatalf("invalid decoded width: got %d, want %d", n, len(codes))
	}
	if id != math.MaxUint8 {
		t.Fatalf("invalid call id: got %d, want %d", id, math.MaxUint8)
	}
	assertRegs(t, decodedArgs, args)
	assertRegs(t, decodedReturns, returns)
}

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	//
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	//
	fn()
}
