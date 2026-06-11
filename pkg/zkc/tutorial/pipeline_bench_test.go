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

package tutorial

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
)

func BenchmarkCompileProgram(b *testing.B) {
	for b.Loop() {
		if _, _, err := CompileProgram(); err != nil {
			b.Fatalf("compile program: %v", err)
		}
	}
}

func BenchmarkCompileProverMachine(b *testing.B) {
	for b.Loop() {
		if _, err := CompileMachine(ProverCodegenConfig()); err != nil {
			b.Fatalf("compile machine: %v", err)
		}
	}
}

func BenchmarkExecuteFreshBinary(b *testing.B) {
	input := InputBytes(7, 3, 2)
	expected := []uint16{10, 20, 4}

	b.ResetTimer()

	for b.Loop() {
		output, err := Execute(input)
		if err != nil {
			b.Fatalf("execute: %v", err)
		}

		if got := UnpackU16(output["result"]); got[0] != expected[0] || got[1] != expected[1] || got[2] != expected[2] {
			b.Fatalf("unexpected output: got %v, want %v", got, expected)
		}
	}
}

func BenchmarkTraceAndCheckFreshBinary(b *testing.B) {
	input := InputBytes(7, 3, 2)
	traceConfig := constraints.DEFAULT_TRACE_CONFIG.WithParallelism(false)

	b.ResetTimer()

	for b.Loop() {
		binf, err := NewBinaryFile()
		if err != nil {
			b.Fatalf("new binary file: %v", err)
		}

		tr, errs := binf.Trace(input, traceConfig)
		if len(errs) != 0 {
			b.Fatalf("trace: %v", errs)
		}

		if failures := binf.Check(tr, traceConfig); len(failures) != 0 {
			b.Fatalf("constraint failures: %s", failureMessages(failures))
		}
	}
}

func BenchmarkTraceAndCheckFromSerializedBinary(b *testing.B) {
	template, err := NewBinaryFile()
	if err != nil {
		b.Fatalf("new binary file: %v", err)
	}

	serialized, err := template.MarshalBinary()
	if err != nil {
		b.Fatalf("marshal binary: %v", err)
	}

	input := InputBytes(7, 3, 2)
	traceConfig := constraints.DEFAULT_TRACE_CONFIG.WithParallelism(false)

	b.ResetTimer()

	for b.Loop() {
		var binf constraints.BinaryFile[koalabear.Element]
		if err := binf.UnmarshalBinary(serialized); err != nil {
			b.Fatalf("unmarshal binary: %v", err)
		}

		tr, errs := binf.Trace(input, traceConfig)
		if len(errs) != 0 {
			b.Fatalf("trace: %v", errs)
		}

		if failures := binf.Check(tr, traceConfig); len(failures) != 0 {
			b.Fatalf("constraint failures: %s", failureMessages(failures))
		}
	}
}
