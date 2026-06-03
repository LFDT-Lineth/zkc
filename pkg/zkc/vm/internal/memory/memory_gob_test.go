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
package memory_test

import (
	"bytes"
	"encoding/gob"
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"

	// Side-effect import: registers concrete memory[Uint] types as
	// base.Module so they can be encoded through the Module interface.
	_ "github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

func sampleRegs() []register.Register {
	return []register.Register{
		register.NewInput("addr", 4, *big.NewInt(0)),
		register.NewOutput("data0", 8, *big.NewInt(0)),
		register.NewOutput("data1", 8, *big.NewInt(0)),
	}
}

func Test_ZkcMemory_StaticArrayRoundTrip(t *testing.T) {
	var (
		regs     = sampleRegs()
		original = memory.NewStaticArray[word.Uint]("rom", memory.PUBLIC_STATIC_MEMORY, regs)
	)
	//
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(&original); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	//
	var decoded memory.StaticArray[word.Uint]
	if err := gob.NewDecoder(&buffer).Decode(&decoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	//
	if decoded.Name() != "rom" {
		t.Errorf("name: got %q, want %q", decoded.Name(), "rom")
	}
	//
	if !decoded.IsPublic() || !decoded.IsStatic() || !decoded.IsReadOnly() {
		t.Errorf("kind: lost PUBLIC_STATIC_MEMORY flags")
	}
	//
	if decoded.Width() != 3 {
		t.Errorf("width: got %d, want 3", decoded.Width())
	}
	//
	if decoded.Geometry().AddressLines() != 1 {
		t.Errorf("address lines: got %d, want 1", decoded.Geometry().AddressLines())
	}
	//
	if decoded.Geometry().DataLines() != 2 {
		t.Errorf("data lines: got %d, want 2", decoded.Geometry().DataLines())
	}
}

func Test_ZkcMemory_ReadOnlyWithDataRoundTrip(t *testing.T) {
	var (
		regs     = sampleRegs()
		original = memory.NewStaticArray[word.Uint]("rom", memory.PUBLIC_STATIC_MEMORY, regs)
		one      word.Uint
		two      word.Uint
	)
	//
	original.Initialise([]word.Uint{one.SetUint64(0xdeadbeef), two.SetUint64(0xcafebabe)})
	//
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(&original); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	//
	var decoded memory.StaticArray[word.Uint]
	if err := gob.NewDecoder(&buffer).Decode(&decoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	//
	if got := decoded.Registers(); len(got) != 3 {
		t.Errorf("registers: got %d, want 3", len(got))
	}
}

func Test_ZkcMemory_ModuleInterfaceRoundTrip(t *testing.T) {
	// Construct one of each memory shape.
	rom := &memory.ReadOnly[word.Uint]{
		StaticArray: memory.NewStaticArray[word.Uint]("rom", memory.PUBLIC_READ_ONLY_MEMORY, sampleRegs()),
	}
	wom := &memory.WriteOnce[word.Uint]{
		StaticArray: memory.NewStaticArray[word.Uint]("wom", memory.PUBLIC_WRITE_ONCE_MEMORY, sampleRegs()),
	}
	srom := &memory.StaticReadOnly[word.Uint]{
		ReadOnly: memory.ReadOnly[word.Uint]{
			StaticArray: memory.NewStaticArray[word.Uint]("srom", memory.PUBLIC_STATIC_MEMORY, sampleRegs()),
		},
	}
	ra := &memory.RandomAccess[word.Uint]{
		StaticArray: memory.NewStaticArray[word.Uint]("ra", memory.RANDOM_ACCESS_MEMORY, sampleRegs()),
	}
	bram := memory.NewBiPartiteRandomAccess[word.Uint]("bram", sampleRegs())
	input := []base.Module{rom, wom, srom, ra, bram}
	//
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(input); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	//
	var decoded []base.Module
	if err := gob.NewDecoder(&buffer).Decode(&decoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	//
	if len(decoded) != 5 {
		t.Fatalf("decoded length: got %d, want 5", len(decoded))
	}
	//
	if _, ok := decoded[0].(*memory.ReadOnly[word.Uint]); !ok {
		t.Errorf("decoded[0]: got %T, want *memory.ReadOnly[word.Uint]", decoded[0])
	}
	//
	if _, ok := decoded[1].(*memory.WriteOnce[word.Uint]); !ok {
		t.Errorf("decoded[1]: got %T, want *memory.WriteOnce[word.Uint]", decoded[1])
	}
	//
	if _, ok := decoded[2].(*memory.StaticReadOnly[word.Uint]); !ok {
		t.Errorf("decoded[2]: got %T, want *memory.StaticReadOnly[word.Uint]", decoded[2])
	}
	//
	if _, ok := decoded[3].(*memory.RandomAccess[word.Uint]); !ok {
		t.Errorf("decoded[3]: got %T, want *memory.RandomAccess[word.Uint]", decoded[3])
	}
	//
	if _, ok := decoded[4].(*memory.BiPartiteRandomAccess[word.Uint]); !ok {
		t.Errorf("decoded[4]: got %T, want *memory.BiPartiteRandomAccess[word.Uint]", decoded[4])
	}
	//
	for i, name := range []string{"rom", "wom", "srom", "ra", "bram"} {
		if decoded[i].Name() != name {
			t.Errorf("decoded[%d].Name(): got %q, want %q", i, decoded[i].Name(), name)
		}
	}
}
