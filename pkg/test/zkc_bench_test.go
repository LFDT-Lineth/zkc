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

// ===================================================================
// Benchmark Tests
// ===================================================================
func Test_ZkcBench_Blake(t *testing.T) {
	checkZkcBench(t, "zkc/bench/blake", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}
func Test_ZkcBench_BinarySearchTree(t *testing.T) {
	checkZkcBench(t, "zkc/bench/bsearch_tree", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_FastPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fast_pow", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_Fnv1aHash(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fnv1a_hash", util.DEFAULT_CONFIG.Words(vm.WORD_UINT))
}

// func Test_ZkcBench_Keccakf(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf",
// 		util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64))
// }

// Keccakf with padding, little-endian input and output, and batched
// Will be used for later benchmarks

// func Test_ZkcBench_KeccakfWithPadding(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_with_padding",
//     util.DEFAULT_CONFIG.Words(vm.WORD_UINT,vm.WORD_UINT64).Bytecode(true))
// }

func Test_ZkcBench_KeccakfLe(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccakf_le",
		util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

// Same as Test_ZkcBench_Keccakf, but the loop is in Zkc and we have 20k test vectors
// a single line in .accepts that packs all test vectors
/*func Test_ZkcBench_KeccakfBatched(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccakf_batched",
	  util.DEFAULT_CONFIG.Words(vm.WORD_UINT,vm.WORD_UINT64).Bytecode(true))
}*/

func Test_ZkcBench_Sort(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sort", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}
func Test_ZkcBench_SgnExtend(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sgn_extension_u32_u64",
		util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_Lo32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/lo_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_Hi32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/hi_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_Mul32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mul_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_Mulh32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulh_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_Mulhu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulhu_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_Mulhsu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulhsu_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT))
}

func Test_ZkcBench_LongDivisionU32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/long_division_u32",
		util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_DivuRemu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/divu_remu_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_DivRem32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/div_rem_32", util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

func Test_ZkcBench_LeftShiftAndTypeBug(t *testing.T) {
	checkZkcBench(t, "zkc/bench/left_shift_and_type_bug",
		util.DEFAULT_CONFIG.Words(vm.WORD_UINT, vm.WORD_UINT64).Bytecode(true))
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcBench(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config)
}
