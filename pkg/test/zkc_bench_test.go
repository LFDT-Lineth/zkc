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

import (
	"testing"

	test_util "github.com/LFDT-Lineth/zkc/pkg/test/util"
)

// DEFAULT_BENCH_CONFIG provides a default configuration for bench tests.
var DEFAULT_BENCH_CONFIG = test_util.DEFAULT_CONFIG

// ===================================================================
// Benchmark Tests
// ===================================================================
func Test_ZkcBench_Blake(t *testing.T) {
	checkZkcBench(t, "zkc/bench/blake", DEFAULT_BENCH_CONFIG.Sampling(0.1))
}

func Test_ZkcBench_BinarySearchTree(t *testing.T) {
	checkZkcBench(t, "zkc/bench/bsearch_tree", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_FastPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fast_pow", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_RecPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/rec_pow", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_Gcd(t *testing.T) {
	checkZkcBench(t, "zkc/bench/gcd", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_Fnv1aHash(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fnv1a_hash", DEFAULT_BENCH_CONFIG)
}

// Two keccak implementations are benchmarked side by side: "narrow" keeps
// the state in memory between rounds (narrow call signatures), "wide"
// threads the 25 state lanes through calls as parameters / returns (no
// per-round memory traffic, but 50-value call signatures).
// TODO: restore ParallelTracing("keccak_f", 10) once sharded RAM tracing is
// fixed (see util.Config.ParallelTracing) — sharding any input large enough
// to split (e.g. the 4,236-byte line) violates "active_monotony", already on
// the pre-existing keccak benchmark.
func Test_ZkcBench_KeccakNarrow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccak_narrow", DEFAULT_BENCH_CONFIG.Sampling(0.1))
}

func Test_ZkcBench_KeccakWide(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccak_wide", DEFAULT_BENCH_CONFIG.Sampling(0.1))
}
func Test_ZkcBench_Poseidon(t *testing.T) {
	// #2007: support implicit sign bit
	checkZkcBench(t, "zkc/bench/poseidon/poseidon", DEFAULT_BENCH_CONFIG.
		Constraints(false).GoGen(false))
}

// ===================================================================
// Other tests
// ===================================================================

func Test_ZkcBench_Sort(t *testing.T) {
	// TODO: restore ParallelTracing("sort_slice", 5) once sharded RAM tracing
	// is fixed (see util.Config.ParallelTracing).
	checkZkcBench(t, "zkc/bench/sort", DEFAULT_BENCH_CONFIG.Sampling(0.1))
}

func Test_ZkcBench_LongDivision(t *testing.T) {
	checkZkcBench(t, "zkc/bench/long_division", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_DivRem(t *testing.T) {
	checkZkcBench(t, "zkc/bench/div_rem", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_ModExp32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/modexp32", DEFAULT_BENCH_CONFIG)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcBench(t *testing.T, test string, config test_util.Config) {
	test_util.CheckValid(t, test, "zkc", config)
}
