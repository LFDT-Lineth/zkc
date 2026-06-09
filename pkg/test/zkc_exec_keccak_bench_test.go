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

// Phase 7.3: a three-way comparison of the same compiled ZkC keccak program run by
//   1. the (uint64-lowered) WordMachine — the slow, analysis-grade interpreter;
//   2. the bytecode interpreter — the current "fast" tier;
//   3. the generated-Go executor — this branch.
//
// The point of the branch is to show (3) is faster than (2) while being lean enough
// to replace it.  For now the gogen number includes `go build` (reported as a
// separate metric) and subprocess execution; an in-process AOT path comes later.

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	zkcutil "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

const (
	keccakV2SourcePath = "../../testdata/zkc/bench/keccakf_v2.zkc"
	keccakV2InputPath  = "../../testdata/zkc/bench/keccak_50000_v2.accepts"
	// keccakExecBudget is the per-Execute chunk size (ExecuteAll loops until the
	// program returns, so this only bounds chunk granularity, not total steps).
	keccakExecBudget = 1 << 22
)

// Sinks prevent the compiler from optimising the benchmarked work away.
var (
	keccakByteSink map[string][]byte
	keccakWordSink map[string][]uint64
)

// TestZkcExecKeccakV2 checks the executors agree on the full 50000-block fixture,
// using the (slow) WordMachine as the reference oracle.  The fixture carries inputs
// only, so correctness is established differentially — gogen must match the
// WordMachine exactly.  It is heavy, so it is skipped under -short and is not part
// of the standard zkc-test selection.
func TestZkcExecKeccakV2(t *testing.T) {
	if testing.Short() {
		t.Skip("keccak v2 cross-check is heavy; skipped in -short")
	}

	m64 := compileKeccakV2(t)
	inputBytes := loadKeccakV2Input(t)
	inputWords := decodeKeccakInputs(t, m64, inputBytes)

	// Reference: the slow word machine.
	want := runKeccakWordCore(t, m64, inputWords)["result"]

	// Generated Go must match the reference exactly.
	if got := runKeccakGogen(t, m64, inputBytes); !bytes.Equal(got, want) {
		t.Fatalf("gogen result mismatch")
	}

	// Bytecode interpreter, if it can encode this program (see tryBytecode).
	if bci, err := tryBytecodeInterpreter(m64); err != nil {
		t.Logf("bytecode interpreter unavailable for this program: %v", err)
	} else if got := runKeccakWordCore(t, bci, inputWords)["result"]; !bytes.Equal(got, want) {
		t.Fatalf("bytecode result mismatch")
	}
}

// TestZkcExecKeccakV2SmokeAgree is a fast correctness check on a small synthetic
// input (2 pre-padded blocks): it asserts the three executors agree, without
// needing the large fixture or a known-good digest.  This exercises gogen's RAM,
// SROM, bitwise/shift/concat and call support end-to-end.
func TestZkcExecKeccakV2SmokeAgree(t *testing.T) {
	m64 := compileKeccakV2(t)
	inputBytes := syntheticKeccakInput(2)
	inputWords := decodeKeccakInputs(t, m64, inputBytes)

	want := runKeccakWordCore(t, m64, inputWords)["result"]

	if bci, err := tryBytecodeInterpreter(m64); err != nil {
		t.Logf("bytecode interpreter unavailable for this program: %v", err)
	} else if got := runKeccakWordCore(t, bci, inputWords)["result"]; !bytes.Equal(got, want) {
		t.Fatalf("bytecode disagrees with wordmachine")
	}

	if got := runKeccakGogen(t, m64, inputBytes); !bytes.Equal(got, want) {
		t.Fatalf("gogen disagrees with wordmachine")
	}
}

// syntheticKeccakInput builds nBlocks pre-padded blocks of arbitrary data: the
// keccak-f permutation is exercised regardless of the block contents.
func syntheticKeccakInput(nBlocks int) map[string][]byte {
	nb := make([]byte, 8)
	nb[7] = byte(nBlocks)
	// Each block is 17 u64 lanes = 136 bytes.
	blocks := make([]byte, nBlocks*17*8)
	for i := range blocks {
		blocks[i] = byte(i)
	}

	return map[string][]byte{"n_blocks": nb, "blocks": blocks}
}

func BenchmarkZkcExecKeccakV2(b *testing.B) {
	inputBytes := loadKeccakV2Input(b)
	blockBytes := int64(len(inputBytes["blocks"]))

	b.Run("wordmachine", func(b *testing.B) {
		m64 := compileKeccakV2(b)
		inputWords := decodeKeccakInputs(b, m64, inputBytes)

		b.SetBytes(blockBytes)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			keccakByteSink = runKeccakWordCore(b, m64, inputWords)
		}
	})

	b.Run("bytecode", func(b *testing.B) {
		m64 := compileKeccakV2(b)
		// Compile to bytecode once, mirroring how gogen builds once below.
		bci, err := tryBytecodeInterpreter(m64)
		if err != nil {
			b.Skipf("bytecode interpreter cannot encode this program: %v", err)
		}

		inputWords := decodeKeccakInputs(b, m64, inputBytes)

		b.SetBytes(blockBytes)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			keccakByteSink = runKeccakWordCore(b, bci, inputWords)
		}
	})

	b.Run("gogen", func(b *testing.B) {
		m64 := compileKeccakV2(b)

		src, err := vm.WordToGoSource(m64)
		if err != nil {
			b.Skipf("gogen unsupported: %v", err)
		}
		// Build once; report the compile time as a separate metric.
		start := time.Now()

		prog, err := util.GogenBuild(src)
		if err != nil {
			b.Fatal(err)
		}

		buildMs := float64(time.Since(start).Milliseconds())
		inputWords := toKeccakU64Map(decodeKeccakInputs(b, m64, inputBytes))

		b.SetBytes(blockBytes)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			out, errored, err := util.GogenRun(prog, inputWords)
			if err != nil {
				b.Fatal(err)
			}

			if errored {
				b.Fatal("gogen reported an execution error")
			}

			keccakWordSink = out
		}

		b.StopTimer()
		b.ReportMetric(buildMs, "build_ms")
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// compileKeccakV2 compiles the keccak v2 source into a fresh, unlowered,
// vectorised u64 word machine (the shared starting point for all three executors).
func compileKeccakV2(tb testing.TB) *vm.WordMachine[vm.Uint64] {
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

	cfg := codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).LowerNatives(false).Vectorize(true).Quiet(true)

	wm, errs := program.Compile(cfg)
	if len(errs) > 0 {
		tb.Fatalf("codegen: %v", errs)
	}

	return vm.WordToWordMachine[vm.Uint, vm.Uint64](wm)
}

// loadKeccakV2Input parses the fixture into the program inputs.  The fixture
// carries inputs only (n_blocks, blocks) — there is no embedded expected output, so
// correctness is checked differentially against the WordMachine.
func loadKeccakV2Input(tb testing.TB) map[string][]byte {
	tb.Helper()

	data, err := os.ReadFile(keccakV2InputPath)
	if err != nil {
		tb.Fatal(err)
	}

	inputs, err := zkcutil.ParseJsonInputFile(data)
	if err != nil {
		tb.Fatal(err)
	}

	return inputs
}

// decodeKeccakInputs decodes the raw input bytes into word values for the machine.
func decodeKeccakInputs(tb testing.TB, m vm.Core[vm.Uint64], inputBytes map[string][]byte) map[string][]vm.Uint64 {
	tb.Helper()

	inputs, errs := vm.DecodeInputs(m, inputBytes)
	if len(errs) > 0 {
		tb.Fatalf("decode inputs: %v", errs)
	}

	return inputs
}

// tryBytecodeInterpreter builds the bytecode interpreter, recovering from a panic
// during bytecode encoding (the current encoder rejects long backward branches with
// "branch target overflow", which large programs like keccak v2 trigger).  This
// keeps the benchmark/test usable even where the bytecode tier cannot run.
func tryBytecodeInterpreter(m64 *vm.WordMachine[vm.Uint64]) (core vm.Core[vm.Uint64], err error) {
	defer func() {
		if r := recover(); r != nil {
			core, err = nil, fmt.Errorf("%v", r)
		}
	}()

	return vm.WordToBytecodeInterpreter(m64), nil
}

// runKeccakWordCore boots and runs a Core to completion, returning encoded outputs.
func runKeccakWordCore(tb testing.TB, m vm.Core[vm.Uint64], inputs map[string][]vm.Uint64) map[string][]byte {
	tb.Helper()

	if err := m.Boot("main", inputs); err != nil {
		tb.Fatalf("boot: %v", err)
	}

	if _, err := vm.ExecuteAll(m, keccakExecBudget); err != nil {
		tb.Fatalf("execute: %v", err)
	}

	return vm.EncodeOutputs(m)
}

// runKeccakGogen generates, builds and runs the gogen executor, returning its
// "result" output encoded back to bytes for comparison.
func runKeccakGogen(tb testing.TB, m64 *vm.WordMachine[vm.Uint64], inputBytes map[string][]byte) []byte {
	tb.Helper()

	src, err := vm.WordToGoSource(m64)
	if err != nil {
		tb.Fatalf("gogen generate: %v", err)
	}

	prog, err := util.GogenBuild(src)
	if err != nil {
		tb.Fatalf("gogen build: %v", err)
	}

	inputWords := toKeccakU64Map(decodeKeccakInputs(tb, m64, inputBytes))

	out, errored, err := util.GogenRun(prog, inputWords)
	if err != nil {
		tb.Fatalf("gogen run: %v", err)
	}

	if errored {
		tb.Fatal("gogen reported an execution error")
	}

	return encodeKeccakResult(tb, m64, out["result"])
}

// toKeccakU64Map converts decoded word inputs into the plain uint64 form the
// generated program consumes over JSON.
func toKeccakU64Map(words map[string][]vm.Uint64) map[string][]uint64 {
	out := make(map[string][]uint64, len(words))

	for name, vs := range words {
		us := make([]uint64, len(vs))
		for i, v := range vs {
			us[i] = v.Uint64()
		}

		out[name] = us
	}

	return out
}

// encodeKeccakResult encodes the gogen "result" words back to bytes using the
// result memory's geometry, so it can be compared with the fixture's expected
// bytes.
func encodeKeccakResult(tb testing.TB, m64 *vm.WordMachine[vm.Uint64], words []uint64) []byte {
	tb.Helper()

	vals := make([]vm.Uint64, len(words))
	for i, v := range words {
		vals[i] = vals[i].SetUint64(v)
	}

	for it := m64.Outputs(); it.HasNext(); {
		o := it.Next()
		if o.Name() == "result" {
			return vm.EncodeBytes(vals, o.Geometry())
		}
	}

	tb.Fatal("no 'result' output memory")

	return nil
}
