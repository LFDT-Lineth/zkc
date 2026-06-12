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

// A cross-workload comparison of the three executors (Uint WordMachine,
// bytecode interpreter, generated Go) on programs with contrasting
// instruction mixes, on both machine shapes (plain and lowered/prover).
// Keccak (zkc_exec_keccak_bench_test.go) is bitwise-rotation heavy; the
// programs here cover the other corners:
//
//   - sort    — memory traffic, comparisons, recursion; LowerNatives barely
//     touches it (~1.7x more instructions);
//   - fnv1a   — u128 multiply + xor per byte (exercises gogen's two-limb
//     registers); lowering explodes it ~306x;
//   - blake2f — 64-bit xor/rotate rounds over a RAM-backed state; ~232x.
//
// Every sub-benchmark reports Minstr/s (millions of executed ZkC
// micro-instructions per second) next to ns/op, so results can be
// interpolated to workloads of any size: estimated time = instrs / rate.
// The instruction count is the canonical per-shape count measured once on
// the reference interpreter for the exact benchmark input — all tiers of a
// shape execute that same instruction stream, so rates are comparable
// across tiers and programs.
//
// Like the keccak benchmark, gogen numbers include the subprocess harness
// (spawn + JSON decode of inputs); cells whose pure execution is below
// ~10ms are dominated by that fixed ~6-8ms cost, which understates the
// gogen rate (see perf_gogen.html).

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// sweepCases are the benchmark programs plus a synthetic input large enough
// that per-run setup (boot, subprocess spawn) does not dominate, yet small
// enough that the Uint WordMachine can interpret the lowered shape in
// seconds.
var sweepCases = []struct {
	name  string
	path  string
	input map[string][]byte
}{
	{"sort_20k", "../../testdata/zkc/bench/sort.zkc", syntheticSortInput(20000)},
	{"fnv1a_100k", "../../testdata/zkc/bench/fnv1a_hash.zkc", syntheticFnv1aInput(100000)},
	{"blake_1k", "../../testdata/zkc/bench/blake.zkc", syntheticBlakeInput(1000)},
}

// BenchmarkZkcExecSweep times every (program, shape, executor) combination.
// Recommended: go test -run XXX -bench BenchmarkZkcExecSweep -benchtime 3x
func BenchmarkZkcExecSweep(b *testing.B) {
	for _, tc := range sweepCases {
		for _, shape := range []struct {
			name    string
			lowered bool
		}{{"plain", false}, {"lowered", true}} {
			prefix := tc.name + "/" + shape.name

			b.Run(prefix+"/wordmachine", func(b *testing.B) {
				wm := compileZkc(b, tc.path, shape.lowered)
				inputWords := decodeKeccakInputs(b, wm, tc.input)
				instrs := sweepMicroInstrs(b, tc.path, shape.lowered, tc.input)

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					keccakByteSink = runKeccakCore(b, wm, inputWords)
				}

				reportInstrMetrics(b, instrs)
			})

			b.Run(prefix+"/bytecode", func(b *testing.B) {
				m64, err := tryNarrowKeccak(compileZkc(b, tc.path, shape.lowered))
				if err != nil {
					b.Skipf("uint64 narrowing unavailable for this shape: %v", err)
				}

				bci, err := tryBytecodeInterpreter(m64)
				if err != nil {
					b.Skipf("bytecode interpreter cannot encode this program: %v", err)
				}

				inputWords := decodeKeccakInputs(b, bci, tc.input)
				instrs := sweepMicroInstrs(b, tc.path, shape.lowered, tc.input)

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					keccakByteSink = runKeccakCore(b, bci, inputWords)
				}

				reportInstrMetrics(b, instrs)
			})

			b.Run(prefix+"/gogen", func(b *testing.B) {
				wm := compileZkc(b, tc.path, shape.lowered)

				src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
				if err != nil {
					b.Skipf("gogen unsupported: %v", err)
				}

				prog, buildMs, err := timedGogenBuild(src)
				if err != nil {
					b.Fatal(err)
				}

				inJSON, err := json.Marshal(toKeccakU64Map(b, decodeKeccakInputs(b, wm, tc.input)))
				if err != nil {
					b.Fatal(err)
				}

				instrs := sweepMicroInstrs(b, tc.path, shape.lowered, tc.input)

				// Warm up: the first exec of a freshly built binary pays a
				// one-off OS verification cost (hundreds of ms on macOS).
				if _, _, err := gogen.RunRaw(prog, inJSON); err != nil {
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

				reportInstrMetrics(b, instrs)
				b.ReportMetric(buildMs, "build_ms")
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// compileZkc compiles a .zkc source into a fresh, vectorised word machine over
// Uint; `lowered` selects the prover shape.
func compileZkc(tb testing.TB, path string, lowered bool) *vm.WordMachine[vm.Uint] {
	tb.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	src := source.NewSourceFile(path, data)

	program, _, errs := compiler.Compile(field.KOALABEAR_16, *src)
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

// reportInstrMetrics attaches the executed micro-instruction count and the
// resulting rate to a sub-benchmark, making results comparable across tiers,
// programs and input sizes (estimated time for W instructions = W / rate).
func reportInstrMetrics(b *testing.B, instrs uint) {
	b.Helper()
	b.ReportMetric(float64(instrs), "instrs/op")
	b.ReportMetric(float64(instrs)*float64(b.N)/b.Elapsed().Seconds()/1e6, "Minstr/s")
}

// gogenBuildMs memoizes the wall time of the first (real) gogen.Build per
// source: the benchmark framework re-invokes sub-benchmark closures, and
// later builds hit the process-wide cache in ~0ms — reporting those would
// (and previously did) show build_ms=0.
var (
	gogenBuildMu sync.Mutex
	gogenBuildMs = map[string]float64{}
)

// timedGogenBuild builds the generated source (through the process-wide
// cache), returning the executable path and the wall time of the first real
// build of this source.
func timedGogenBuild(src string) (string, float64, error) {
	gogenBuildMu.Lock()
	defer gogenBuildMu.Unlock()

	if ms, ok := gogenBuildMs[src]; ok {
		prog, err := gogen.Build(src)
		return prog, ms, err
	}

	start := time.Now()
	prog, err := gogen.Build(src)
	ms := float64(time.Since(start).Milliseconds())

	if err == nil {
		gogenBuildMs[src] = ms
	}

	return prog, ms, err
}

// sweepInstrCounts memoizes per (path, shape, input-identity) instruction
// counts, so the three tiers of a shape pay for one counting run.
var (
	sweepInstrMu     sync.Mutex
	sweepInstrCounts = map[string]uint{}
)

// sweepMicroInstrs returns the number of micro-instructions the program
// executes on the given input, measured by running the reference interpreter
// once (the uint64-narrowed machine when the shape permits, the Uint machine
// otherwise — both execute the identical instruction stream).
func sweepMicroInstrs(tb testing.TB, path string, lowered bool, input map[string][]byte) uint {
	tb.Helper()

	inputLen := 0
	for _, v := range input {
		inputLen += len(v)
	}

	key := fmt.Sprintf("%s|%v|%d", path, lowered, inputLen)

	sweepInstrMu.Lock()
	defer sweepInstrMu.Unlock()

	if n, ok := sweepInstrCounts[key]; ok {
		return n
	}

	var (
		steps uint
		err   error
	)

	if m64, nerr := tryNarrowKeccak(compileZkc(tb, path, lowered)); nerr == nil {
		steps, err = countCoreSteps(m64, decodeKeccakInputs(tb, m64, input))
	} else {
		wm := compileZkc(tb, path, lowered)
		steps, err = countCoreSteps(wm, decodeKeccakInputs(tb, wm, input))
	}

	if err != nil {
		tb.Fatalf("instruction count: %v", err)
	}

	sweepInstrCounts[key] = steps

	return steps
}

// countCoreSteps boots and runs a Core to completion, returning the executed
// micro-instruction count.
func countCoreSteps[W vm.Word[W], C vm.Core[W]](m C, inputs map[string][]W) (uint, error) {
	if err := m.Boot("main", inputs); err != nil {
		return 0, err
	}

	return vm.ExecuteAll(m, keccakExecBudget)
}

// ---------------------------------------------------------------------------
// Synthetic inputs
// ---------------------------------------------------------------------------

// syntheticSortInput builds n pseudo-random bytes for the quicksort program.
func syntheticSortInput(n int) map[string][]byte {
	length := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	data := make([]byte, n)

	for i := range data {
		data[i] = byte(i*7 + 13)
	}

	return map[string][]byte{"data_len": length, "data_in": data}
}

// syntheticFnv1aInput builds a 4-byte big-endian length header plus n bytes to
// hash.
func syntheticFnv1aInput(n int) map[string][]byte {
	data := make([]byte, 4+n)
	data[0], data[1], data[2], data[3] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)

	for i := 4; i < len(data); i++ {
		data[i] = byte(i * 31)
	}

	return map[string][]byte{"data": data}
}

// syntheticBlakeInput builds a blake2f input with r rounds (the EIP-152 vector
// geometry: 8x u64 state, 16x u64 message, 2x u64 counter, final flag).
func syntheticBlakeInput(rounds int) map[string][]byte {
	r := []byte{byte(rounds >> 24), byte(rounds >> 16), byte(rounds >> 8), byte(rounds)}
	h := make([]byte, 8*8)
	m := make([]byte, 16*8)
	t := make([]byte, 2*8)

	for i := range h {
		h[i] = byte(i*89 + 7)
	}

	for i := range m {
		m[i] = byte(i*57 + 3)
	}

	t[7] = 128 // t0 = message length

	return map[string][]byte{"r": r, "h": h, "m": m, "t": t, "f": {1}}
}
