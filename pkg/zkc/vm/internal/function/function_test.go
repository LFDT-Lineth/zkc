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
package function_test

import (
	"bytes"
	"encoding/gob"
	"math/big"
	"testing"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"

	// Side-effect import: registers concrete Word[Uint] instruction types with
	// gob so that gob.Register-dependent encode paths work.
	_ "github.com/consensys/go-corset/pkg/zkc/vm"
)

func Test_ZkcFunction_GobRoundTrip(t *testing.T) {
	var (
		regs = []register.Register{
			register.NewInput("x", 8, *big.NewInt(0)),
			register.NewInput("y", 8, *big.NewInt(0)),
			register.NewOutput("z", 8, *big.NewInt(0)),
			register.NewComputed("t", 8, *big.NewInt(0)),
		}
		code = []instruction.Vector[instruction.Word]{
			instruction.NewVector[instruction.Word](instruction.NewReturn()),
		}
		original = function.New("f", true, regs, code)
	)
	//
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(original); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	//
	var decoded function.Function[instruction.Word]
	if err := gob.NewDecoder(&buffer).Decode(&decoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	//
	if decoded.Name() != "f" {
		t.Errorf("name: got %q, want %q", decoded.Name(), "f")
	}
	//
	if !decoded.IsNative() {
		t.Errorf("native: got false, want true")
	}
	//
	if decoded.NumInputs() != 2 {
		t.Errorf("numInputs: got %d, want 2", decoded.NumInputs())
	}
	//
	if decoded.NumOutputs() != 1 {
		t.Errorf("numOutputs: got %d, want 1", decoded.NumOutputs())
	}
	//
	if decoded.Width() != 4 {
		t.Errorf("width: got %d, want 4", decoded.Width())
	}
	//
	if len(decoded.Code()) != 1 {
		t.Fatalf("code length: got %d, want 1", len(decoded.Code()))
	}
	//
	if len(decoded.Code()[0].Codes) != 1 {
		t.Fatalf("vector length: got %d, want 1", len(decoded.Code()[0].Codes))
	}
	//
	if _, ok := decoded.Code()[0].Codes[0].(*instruction.Return); !ok {
		t.Errorf("first instruction: got %T, want *instruction.Return", decoded.Code()[0].Codes[0])
	}
}
