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

	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// DEFAULT_BENCH_CONFIG provides a default configuration for bench tests.
var DEFAULT_BENCH_CONFIG = util.DEFAULT_CONFIG.
	Words(vm.WORD_UINT, vm.WORD_UINT64).
	Bytecode(true)

// ===================================================================
// Benchmark Tests
// ===================================================================
func Test_ZkcBench_Blake(t *testing.T) {
	checkZkcBench(t, "zkc/bench/blake", DEFAULT_BENCH_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_BinarySearchTree(t *testing.T) {
	checkZkcBench(t, "zkc/bench/bsearch_tree", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_FastPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fast_pow", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_Fnv1aHash(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fnv1a_hash", DEFAULT_BENCH_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_Keccakf(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccakf", DEFAULT_BENCH_CONFIG.Words(vm.WORD_UINT))
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

func Test_ZkcBench_Sort(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sort", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_LongDivision(t *testing.T) {
	checkZkcBench(t, "zkc/bench/long_division", DEFAULT_BENCH_CONFIG)
}

func Test_ZkcBench_DivRem(t *testing.T) {
	checkZkcBench(t, "zkc/bench/div_rem", DEFAULT_BENCH_CONFIG)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcBench(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config)
}
