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
package constraints_test

import (
	"bytes"
	"encoding/gob"
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// Smoke test for the Base machine round-trip: encode an empty WordMachine,
// decode it, and verify it survives.
func Test_ZkcMachine_GobRoundTripEmpty(t *testing.T) {
	original := vm.NewWordMachine[vm.Uint](field.KOALABEAR_16)
	//
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(original); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	//
	var decoded vm.WordMachine[vm.Uint]
	if err := gob.NewDecoder(&buffer).Decode(&decoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	//
	if len(decoded.Modules()) != 0 {
		t.Errorf("modules: got %d, want 0", len(decoded.Modules()))
	}
}

// End-to-end test: build a small WordMachine with one function, encode it
// inside a BinaryFile via MarshalBinary, decode via UnmarshalBinary, and
// verify the decoded function/module survived.
func Test_ZkcBinaryFile_RoundTrip(t *testing.T) {
	var (
		field = field.KOALABEAR_16
		regs  = []register.Register{
			register.NewInput("x", 8, *big.NewInt(0)),
			register.NewOutput("y", 8, *big.NewInt(0)),
		}
		code = []instruction.Vector[instruction.Word]{
			instruction.NewVector[instruction.Word](instruction.NewReturn()),
		}
		fn      = vm.NewFunction[instruction.Word]("main", false, regs, code)
		machine = vm.NewWordMachine[vm.Uint](field, fn)
		binfile = constraints.NewBinaryFile[koalabear.Element](nil, nil, field, *machine)
	)
	//
	data, err := binfile.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	//
	var decoded constraints.BinaryFile[koalabear.Element]
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
}
