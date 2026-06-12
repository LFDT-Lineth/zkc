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

// A three-way comparison of the same compiled ZkC keccak program run by
//   1. the WordMachine — the slow, analysis-grade interpreter (uint64-narrowed
//      where possible, over Uint otherwise);
//   2. the bytecode interpreter — the current "fast" tier;
//   3. the generated-Go executor — which consumes the Uint machine directly.
//
// Two machine shapes are exercised:
//
//   - "plain" (native bitwise ops): the fastest execution shape, where all
//     three tiers run;
//   - "lowered" (LowerNatives on — the prover shape): comparison/division
//     lowering widens temporaries to u65, so the uint64-narrowed tiers
//     (WordMachine-u64, bytecode) cannot narrow the machine and skip; only
//     the Uint WordMachine and gogen (which lowers register widths itself)
//     run it.
//
// For now the gogen number includes `go build` (reported as a separate metric)
// and subprocess execution; an in-process AOT path comes later.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

const (
	keccakV2SourcePath = "../../testdata/zkc/bench/keccakf_v2.zkc"
	// keccakExecBudget is the per-Execute chunk size (ExecuteAll loops until the
	// program returns, so this only bounds chunk granularity, not total steps).
	keccakExecBudget = 1 << 22
)

// Sinks prevent the compiler from optimising the benchmarked work away.
var (
	keccakByteSink map[string][]byte
	keccakWordSink map[string][]uint64
)

// TestZkcExecKeccakV2 checks the executors agree on the full 50000-block input,
// using the (slow) WordMachine as the reference oracle.  The input carries no
// expected output, so correctness is established differentially — gogen must
// match the WordMachine exactly.  It is heavy, so it is skipped under -short and
// is not part of the standard zkc-test selection.  (Plain shape only:
// interpreting the lowered shape over 50000 blocks is prohibitively slow; the
// lowered shape is covered by the smoke test below.)
func TestZkcExecKeccakV2(t *testing.T) {
	if testing.Short() {
		t.Skip("keccak v2 cross-check is heavy; skipped in -short")
	}

	wm := compileKeccakV2(t, false)
	inputBytes := syntheticKeccakInput(50000)

	m64, err := tryNarrowKeccak(wm)
	if err != nil {
		t.Fatalf("narrowing the plain shape should succeed: %v", err)
	}

	// Reference: the slow word machine.
	want := runKeccakCore(t, m64, decodeKeccakInputs(t, m64, inputBytes))["result"]

	// Generated Go must match the reference exactly.
	got, err := tryKeccakGogen(t, wm, inputBytes)
	if err != nil {
		t.Fatalf("gogen: %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("gogen result mismatch")
	}

	// Bytecode interpreter, if it can encode this program (see tryBytecode).
	if bci, err := tryBytecodeInterpreter(m64); err != nil {
		t.Logf("bytecode interpreter unavailable for this program: %v", err)
	} else if got := runKeccakCore(t, bci, decodeKeccakInputs(t, bci, inputBytes))["result"]; !bytes.Equal(got, want) {
		t.Fatalf("bytecode result mismatch")
	}
}

// TestZkcExecKeccakV2SmokeAgree is a fast correctness check on a small synthetic
// input (2 pre-padded blocks): it asserts the available executors agree on BOTH
// machine shapes, without needing the large fixture or a known-good digest.  On
// the plain shape this exercises gogen's RAM, SROM, bitwise/shift/concat and
// call support end-to-end.  On the lowered (prover) shape only the Uint
// WordMachine can run today (u65 temporaries); the others are logged-and-skipped.
func TestZkcExecKeccakV2SmokeAgree(t *testing.T) {
	for _, shape := range []struct {
		name    string
		lowered bool
	}{{"plain", false}, {"lowered", true}} {
		t.Run(shape.name, func(t *testing.T) {
			wm := compileKeccakV2(t, shape.lowered)
			inputBytes := syntheticKeccakInput(2)

			// Reference: the Uint word machine (always runs).
			want := runKeccakCore(t, wm, decodeKeccakInputs(t, wm, inputBytes))["result"]

			// The uint64-narrowed peers, when the shape permits narrowing.
			if m64, err := tryNarrowKeccak(compileKeccakV2(t, shape.lowered)); err != nil {
				t.Logf("uint64 narrowing unavailable for this shape: %v", err)
			} else {
				if got := runKeccakCore(t, m64, decodeKeccakInputs(t, m64, inputBytes))["result"]; !bytes.Equal(got, want) {
					t.Fatalf("wordmachine-u64 disagrees with the Uint reference")
				}

				if bci, err := tryBytecodeInterpreter(m64); err != nil {
					t.Logf("bytecode interpreter unavailable for this program: %v", err)
				} else if got := runKeccakCore(t, bci, decodeKeccakInputs(t, bci, inputBytes))["result"]; !bytes.Equal(got, want) {
					t.Fatalf("bytecode disagrees with the Uint reference")
				}
			}

			// Generated Go.
			if got, err := tryKeccakGogen(t, wm, inputBytes); err != nil {
				t.Logf("gogen unavailable for this shape: %v", err)
			} else if !bytes.Equal(got, want) {
				t.Fatalf("gogen disagrees with the Uint reference")
			}
		})
	}
}

// syntheticKeccakInput builds nBlocks pre-padded blocks of arbitrary data: the
// keccak-f permutation is exercised regardless of the block contents.
func syntheticKeccakInput(nBlocks int) map[string][]byte {
	nb := make([]byte, 8)
	nb[6] = byte(nBlocks >> 8)
	nb[7] = byte(nBlocks)
	// Each block is 17 u64 lanes = 136 bytes.
	blocks := make([]byte, nBlocks*17*8)
	for i := range blocks {
		blocks[i] = byte(i)
	}

	return map[string][]byte{"n_blocks": nb, "blocks": blocks}
}

// BenchmarkZkcExecKeccakV2 times the three executors on the plain (native
// bitwise) shape over 50000 blocks.
func BenchmarkZkcExecKeccakV2(b *testing.B) {
	benchmarkKeccakThreeWay(b, false, syntheticKeccakInput(50000))
}

// BenchmarkZkcExecKeccakV2Lowered times the executors on the lowered (prover)
// shape.  Interpreting lowered bitwise ops is orders of magnitude slower, so a
// smaller synthetic input keeps the comparison runnable; the u64-narrowed
// tiers (wordmachine-u64, bytecode) skip — the shape's u65 temporaries cannot
// narrow.
func BenchmarkZkcExecKeccakV2Lowered(b *testing.B) {
	benchmarkKeccakThreeWay(b, true, syntheticKeccakInput(500))
}

func benchmarkKeccakThreeWay(b *testing.B, lowered bool, inputBytes map[string][]byte) {
	b.Helper()

	blockBytes := int64(len(inputBytes["blocks"]))
	// Canonical micro-instruction count for this (shape, input): all tiers
	// execute the same instruction stream, so the Minstr/s rates below are
	// directly comparable across tiers and with the sweep benchmark.
	instrs := keccakMicroInstrs(b, lowered, int(blockBytes)/(17*8))

	b.Run("wordmachine", func(b *testing.B) {
		// Prefer the uint64-narrowed machine (the historical baseline); fall
		// back to the Uint machine where narrowing is impossible.
		wm := compileKeccakV2(b, lowered)

		var run func() map[string][]byte

		if m64, err := tryNarrowKeccak(wm); err == nil {
			inputWords := decodeKeccakInputs(b, m64, inputBytes)
			run = func() map[string][]byte { return runKeccakCore(b, m64, inputWords) }
		} else {
			inputWords := decodeKeccakInputs(b, wm, inputBytes)
			run = func() map[string][]byte { return runKeccakCore(b, wm, inputWords) }
		}

		b.SetBytes(blockBytes)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			keccakByteSink = run()
		}

		reportInstrMetrics(b, instrs)
	})

	b.Run("bytecode", func(b *testing.B) {
		m64, err := tryNarrowKeccak(compileKeccakV2(b, lowered))
		if err != nil {
			b.Skipf("uint64 narrowing unavailable for this shape: %v", err)
		}
		// Compile to bytecode once, mirroring how gogen builds once below.
		bci, err := tryBytecodeInterpreter(m64)
		if err != nil {
			b.Skipf("bytecode interpreter cannot encode this program: %v", err)
		}

		inputWords := decodeKeccakInputs(b, bci, inputBytes)

		b.SetBytes(blockBytes)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			keccakByteSink = runKeccakCore(b, bci, inputWords)
		}

		reportInstrMetrics(b, instrs)
	})

	b.Run("gogen", func(b *testing.B) {
		wm := compileKeccakV2(b, lowered)

		src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
		if err != nil {
			b.Skipf("gogen unsupported: %v", err)
		}
		// Build once; report the (first, real) compile time as a separate
		// metric — see timedGogenBuild for why it must be memoized.
		prog, buildMs, err := timedGogenBuild(src)
		if err != nil {
			b.Fatal(err)
		}
		// Pre-marshal the inputs: the timed loop measures the executor
		// (subprocess + its own JSON decode), not the harness's json.Marshal.
		inJSON, err := json.Marshal(toKeccakU64Map(b, decodeKeccakInputs(b, wm, inputBytes)))
		if err != nil {
			b.Fatal(err)
		}

		// Warm up: the first exec of a freshly built binary pays a one-off OS
		// verification cost (hundreds of ms on macOS).
		if _, _, err := gogen.RunRaw(prog, inJSON); err != nil {
			b.Fatal(err)
		}

		b.SetBytes(blockBytes)
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

		b.StopTimer()
		reportInstrMetrics(b, instrs)
		b.ReportMetric(buildMs, "build_ms")
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// keccakInstrCalibration memoizes the per-shape (setup, marginal) instruction
// counts behind keccakMicroInstrs.
var (
	keccakInstrMu          sync.Mutex
	keccakInstrCalibration = map[bool][2]uint{}
)

// keccakMicroInstrs returns the number of micro-instructions the keccak v2
// program executes over nBlocks blocks.  Counting on the full benchmark input
// would mean interpreting it (up to minutes on the lowered shape), so the
// count is calibrated instead: keccak-f executes an input-independent,
// constant number of instructions per block, so counting 1 and 2 blocks gives
// an exact extrapolation (the linearity is asserted by counting both).
func keccakMicroInstrs(tb testing.TB, lowered bool, nBlocks int) uint {
	tb.Helper()

	keccakInstrMu.Lock()
	defer keccakInstrMu.Unlock()

	cal, ok := keccakInstrCalibration[lowered]
	if !ok {
		c1 := sweepMicroInstrs(tb, keccakV2SourcePath, lowered, syntheticKeccakInput(1))
		c2 := sweepMicroInstrs(tb, keccakV2SourcePath, lowered, syntheticKeccakInput(2))
		cal = [2]uint{c1, c2 - c1} // setup+1st block, marginal per block

		keccakInstrCalibration[lowered] = cal
	}

	return cal[0] + uint(nBlocks-1)*cal[1]
}

// compileKeccakV2 compiles the keccak v2 source into a fresh, vectorised word
// machine over Uint — the machine gogen consumes and the reference executor
// interprets.  `lowered` selects the prover shape.
func compileKeccakV2(tb testing.TB, lowered bool) *vm.WordMachine[vm.Uint] {
	tb.Helper()

	data, err := os.ReadFile(keccakV2SourcePath)
	if err != nil {
		tb.Fatal(err)
	}

	src := source.NewSourceFile(keccakV2SourcePath, data)

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

// tryNarrowKeccak narrows a Uint machine to uint64 words, recovering the panic
// WordToWordMachine raises when the machine holds registers beyond 64 bits
// (which the lowered shape does: comparison lowering widens to u65).
func tryNarrowKeccak(wm *vm.WordMachine[vm.Uint]) (m64 *vm.WordMachine[vm.Uint64], err error) {
	defer func() {
		if r := recover(); r != nil {
			m64, err = nil, fmt.Errorf("%v", r)
		}
	}()

	return vm.WordToWordMachine[vm.Uint, vm.Uint64](wm), nil
}

// decodeKeccakInputs decodes the raw input bytes into word values for the machine.
func decodeKeccakInputs[W vm.Word[W], C vm.Core[W]](tb testing.TB, m C, inputBytes map[string][]byte) map[string][]W {
	tb.Helper()

	inputs, errs := vm.DecodeInputs[W](m, inputBytes)
	if len(errs) > 0 {
		tb.Fatalf("decode inputs: %v", errs)
	}

	return inputs
}

// tryBytecodeInterpreter builds the bytecode interpreter, recovering from a panic
// during bytecode encoding (the encoder panics with "branch target overflow" on
// branches its fixed-width offsets cannot reach).  This keeps the benchmark/test
// usable even where the bytecode tier cannot run.
func tryBytecodeInterpreter(m64 *vm.WordMachine[vm.Uint64]) (core vm.Core[vm.Uint64], err error) {
	defer func() {
		if r := recover(); r != nil {
			core, err = nil, fmt.Errorf("%v", r)
		}
	}()

	return vm.WordToBytecodeInterpreter(m64), nil
}

// runKeccakCore boots and runs a Core to completion, returning encoded outputs.
func runKeccakCore[W vm.Word[W], C vm.Core[W]](tb testing.TB, m C, inputs map[string][]W) map[string][]byte {
	tb.Helper()

	if err := m.Boot("main", inputs); err != nil {
		tb.Fatalf("boot: %v", err)
	}

	if _, err := vm.ExecuteAll(m, keccakExecBudget); err != nil {
		tb.Fatalf("execute: %v", err)
	}

	return vm.EncodeOutputs[W](m)
}

// tryKeccakGogen generates (from the Uint machine), builds and runs the gogen
// executor, returning its "result" output encoded back to bytes for comparison
// — or an error when the generator does not support the program.
func tryKeccakGogen(tb testing.TB, wm *vm.WordMachine[vm.Uint], inputBytes map[string][]byte) ([]byte, error) {
	tb.Helper()

	src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
	if err != nil {
		return nil, err
	}

	prog, err := gogen.Build(src)
	if err != nil {
		tb.Fatalf("gogen build: %v", err)
	}

	inputWords := toKeccakU64Map(tb, decodeKeccakInputs(tb, wm, inputBytes))

	out, errored, err := gogen.Run(prog, inputWords)
	if err != nil {
		tb.Fatalf("gogen run: %v", err)
	}

	if errored {
		tb.Fatal("gogen reported an execution error")
	}

	return encodeKeccakResult(tb, wm, out["result"]), nil
}

// toKeccakU64Map converts decoded word inputs into the plain uint64 form the
// generated program consumes over JSON.
func toKeccakU64Map(tb testing.TB, words map[string][]vm.Uint) map[string][]uint64 {
	tb.Helper()

	out := make(map[string][]uint64, len(words))

	for name, vs := range words {
		us := make([]uint64, len(vs))

		for i, v := range vs {
			if !v.FitsWithin(64) {
				tb.Fatalf("input %q[%d] exceeds 64 bits", name, i)
			}

			us[i] = v.Uint64()
		}

		out[name] = us
	}

	return out
}

// encodeKeccakResult encodes the gogen "result" words back to bytes using the
// result memory's geometry, so it can be compared with the fixture's expected
// bytes.
func encodeKeccakResult(tb testing.TB, wm *vm.WordMachine[vm.Uint], words []uint64) []byte {
	tb.Helper()

	vals := make([]vm.Uint, len(words))
	for i, v := range words {
		vals[i] = vals[i].SetUint64(v)
	}

	for it := wm.Outputs(); it.HasNext(); {
		o := it.Next()
		if o.Name() == "result" {
			return vm.EncodeBytes(vals, o.Geometry())
		}
	}

	tb.Fatal("no 'result' output memory")

	return nil
}
