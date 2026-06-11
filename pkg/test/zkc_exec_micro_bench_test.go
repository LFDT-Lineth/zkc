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
package test

// Micro-benchmarks comparing the WordMachine against the generated-Go executor
// on three workload shapes beyond keccak: branch-heavy (fibonacci loop),
// memory-heavy (RAM churn) and call-heavy (a tiny callee per iteration).  Each
// program takes its iteration count from data[0], so the per-step cost is
// comparable across executors (the gogen number includes the subprocess and
// its JSON I/O; build time is excluded by building before the timer).

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// branch-heavy: iterative fibonacci over u32 lanes with a carry destructure
// (the ADDW shape) inside a loop.
const microFibSrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main() {
    var n:u64 = data[0]
    var a:u32 = 0
    var b:u32 = 1
    var c:u1
    var s:u32
    for i:u64 = 0; i<n; i = i + 1 {
        c::s = (a + b) as u33
        a = b
        b = s
    }
    result[0] = a as u64
    return
}
`

// memory-heavy: a RAM histogram — read-modify-write a rotating cell.
const microMemSrc = `memory cells(address:u8) -> (word:u64)
pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main<cells>() {
    var n:u64 = data[0]
    var k:u8 = 0
    for i:u64 = 0; i<n; i = i + 1 {
        cells[k] = cells[k] + 1
        if k == 199 {
            k = 0
        } else {
            k = k + 1
        }
    }
    result[0] = cells[0]
    return
}
`

// call-heavy: a tiny callee invoked every iteration.
const microCallSrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn step(x:u32) -> (r:u32) {
    r = (x ^ 0x9E37) & 0xFFFF
    return
}
fn main() {
    var n:u64 = data[0]
    var acc:u32 = 1
    for i:u64 = 0; i<n; i = i + 1 {
        acc = step(acc) + 1
    }
    result[0] = acc as u64
    return
}
`

const microSteps = 200_000

func BenchmarkZkcExecMicro(b *testing.B) {
	cases := []struct {
		name string
		src  string
	}{
		{"fib", microFibSrc},
		{"mem", microMemSrc},
		{"call", microCallSrc},
	}

	for _, tc := range cases {
		b.Run(tc.name+"/wordmachine", func(b *testing.B) {
			wm := compileMicro(b, tc.src)
			inputs := map[string][]vm.Uint{"data": {uintWord(microSteps)}}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				keccakByteSink = runKeccakCore(b, compileMicro(b, tc.src), inputs)
				_ = wm
			}
		})

		b.Run(tc.name+"/gogen", func(b *testing.B) {
			src, err := vm.GenerateGo(compileMicro(b, tc.src), vm.GoGenConfig{})
			if err != nil {
				b.Fatalf("GenerateGo: %v", err)
			}

			prog, err := gogen.Build(src)
			if err != nil {
				b.Fatal(err)
			}

			inJSON, err := json.Marshal(map[string][]uint64{"data": {microSteps}})
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				out, errored, err := gogen.RunRaw(prog, inJSON)
				if err != nil {
					b.Fatal(err)
				}

				if errored {
					b.Fatal("gogen reported an execution error")
				}

				keccakWordSink = out
			}
		})
	}
}

// TestZkcExecMicroAgree cross-checks the micro fixtures (gogen vs the Uint
// reference) on a small iteration count, both shapes.
func TestZkcExecMicroAgree(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"fib", microFibSrc},
		{"mem", microMemSrc},
		{"call", microCallSrc},
	}

	for _, tc := range cases {
		for _, lowered := range []bool{false, true} {
			wm := compileMicroShape(t, tc.src, lowered)
			inputs := map[string][]vm.Uint{"data": {uintWord(500)}}
			want := runKeccakCore(t, wm, inputs)["result"]

			src, err := vm.GenerateGo(compileMicroShape(t, tc.src, lowered), vm.GoGenConfig{})
			if err != nil {
				t.Fatalf("%s (lowered=%t): GenerateGo: %v", tc.name, lowered, err)
			}

			prog, err := gogen.Build(src)
			if err != nil {
				t.Fatal(err)
			}

			out, errored, err := gogen.Run(prog, map[string][]uint64{"data": {500}})
			if err != nil || errored {
				t.Fatalf("%s (lowered=%t): gogen run failed: %v %t", tc.name, lowered, err, errored)
			}

			got := encodeMicroResult(t, wm, out["result"])
			if !bytes.Equal(got, want) {
				t.Fatalf("%s (lowered=%t): gogen disagrees with the Uint reference", tc.name, lowered)
			}
		}
	}
}

func compileMicro(tb testing.TB, src string) *vm.WordMachine[vm.Uint] {
	tb.Helper()
	return compileMicroShape(tb, src, false)
}

func compileMicroShape(tb testing.TB, src string, lowered bool) *vm.WordMachine[vm.Uint] {
	tb.Helper()

	sf := source.NewSourceFile("micro.zkc", []byte(src))

	program, _, errs := compiler.Compile(field.KOALABEAR_16, *sf)
	if len(errs) > 0 {
		tb.Fatalf("compile: %v", errs)
	}

	cfg := codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).LowerNatives(lowered).Vectorize(true).Quiet(true)

	wm, errs := program.Compile(cfg)
	if len(errs) > 0 {
		tb.Fatalf("codegen: %v", errs)
	}

	return wm
}

func uintWord(v uint64) vm.Uint {
	var w vm.Uint
	return w.SetUint64(v)
}

func encodeMicroResult(tb testing.TB, wm *vm.WordMachine[vm.Uint], words []uint64) []byte {
	tb.Helper()
	return encodeKeccakResult(tb, wm, words)
}
