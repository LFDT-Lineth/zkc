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
	checkZkcBench(t, "zkc/bench/blake", DEFAULT_BENCH_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcBench_BinarySearchTree(t *testing.T) {
	checkZkcBench(t, "zkc/bench/bsearch_tree", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_FastPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fast_pow", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_Gcd(t *testing.T) {
	checkZkcBench(t, "zkc/bench/gcd", DEFAULT_BENCH_CONFIG.GoGen(false))
}

func Test_ZkcBench_Fnv1aHash(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fnv1a_hash", DEFAULT_BENCH_CONFIG.Constraints(false))
}

func Test_ZkcBench_Keccakf(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccakf", DEFAULT_BENCH_CONFIG.Checkpoints("keccakf", 2).Constraints(false))
}

// func Test_ZkcBench_KeccakfWithPadding(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_with_padding",
// 		DEFAULT_BENCH_CONFIG.Words(vm.WORD_UINT).Bytecode(false))
// }

// func Test_ZkcBench_KeccakfLe(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_le", DEFAULT_BENCH_CONFIG.Words(vm.WORD_UINT))
// }

// Same as Test_ZkcBench_Keccakf, but the loop is in Zkc and we have 20k test vectors
// a single line in .accepts that packs all test vectors
// func Test_ZkcBench_KeccakfBatched(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_batched", DEFAULT_BENCH_CONFIG)
// }
// ===================================================================
// Poseidon tests (self-contained benchmarks; KoalaBear-only)
// ===================================================================

func Test_ZkcBench_Poseidon_Felt_Small(t *testing.T) {
	checkZkcBench(t, "zkc/bench/poseidon_felt_small", DEFAULT_BENCH_CONFIG)
}
func Test_ZkcBench_Poseidon_Felt_Big(t *testing.T) {
	checkZkcBench(t, "zkc/bench/poseidon_felt_big", DEFAULT_BENCH_CONFIG)
}
func Test_ZkcBench_Poseidon_U32_Small(t *testing.T) {
	checkZkcBench(t, "zkc/bench/poseidon_u32_small", DEFAULT_BENCH_CONFIG)
}
func Test_ZkcBench_Poseidon_U32_Big(t *testing.T) {
	checkZkcBench(t, "zkc/bench/poseidon_u32_big", DEFAULT_BENCH_CONFIG)
}

// ===================================================================
// Other tests
// ===================================================================

func Test_ZkcBench_Sort(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sort", DEFAULT_BENCH_CONFIG.Checkpoints("sort_slice", 5).Constraints(false))
}

func Test_ZkcBench_LongDivision(t *testing.T) {
	checkZkcBench(t, "zkc/bench/long_division", DEFAULT_BENCH_CONFIG.Constraints(false))
}

func Test_ZkcBench_DivRem(t *testing.T) {
	checkZkcBench(t, "zkc/bench/div_rem", DEFAULT_BENCH_CONFIG.GoGen(false))
}

func Test_ZkcBench_ModExp32(t *testing.T) {
	t.Skip("tracing failure")
	//
	checkZkcBench(t, "zkc/bench/modexp32",
		DEFAULT_BENCH_CONFIG.GoGen(false).Constraints(false).FastModeSplitting(false))
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcBench(t *testing.T, test string, config test_util.Config) {
	test_util.CheckValid(t, test, "zkc", config)
}
