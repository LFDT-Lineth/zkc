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

	"github.com/consensys/go-corset/pkg/test/util"
	"github.com/consensys/go-corset/pkg/util/field"
)

// ===================================================================
// Benchmark Tests
// ===================================================================
func Test_ZkcBench_Blake(t *testing.T) {
	checkZkcBench(t, "zkc/bench/blake", field.BLS12_377, field.KOALABEAR_16)
}
func Test_ZkcBench_BinarySearchTree(t *testing.T) {
	checkZkcBench(t, "zkc/bench/bsearch_tree", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_FastPow(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fast_pow", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Fnv1aHash(t *testing.T) {
	checkZkcBench(t, "zkc/bench/fnv1a_hash", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Keccakf(t *testing.T) {
	checkZkcBench(t, "zkc/bench/keccakf", field.BLS12_377, field.KOALABEAR_16)
}

// Keccakf with padding and little-endian input and output
// Will be used for later benchmarks

// func Test_ZkcBench_KeccakfWithPadding(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_with_padding", field.BLS12_377, field.KOALABEAR_16)
// }

// func Test_ZkcBench_KeccakfLe(t *testing.T) {
// 	checkZkcBench(t, "zkc/bench/keccakf_le", field.BLS12_377, field.KOALABEAR_16)
// }

func Test_ZkcBench_Sort(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sort", field.BLS12_377, field.KOALABEAR_16)
}
func Test_ZkcBench_SgnExtend(t *testing.T) {
	checkZkcBench(t, "zkc/bench/sgn_extension_u32_u64", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Lo32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/lo_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Hi32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/hi_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Mul32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mul_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Mulh32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulh_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Mulhu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulhu_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_Mulhsu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/mulhsu_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_LongDivisionU32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/long_division_u32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_DivuRemu32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/divu_remu_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_DivRem32(t *testing.T) {
	checkZkcBench(t, "zkc/bench/div_rem_32", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcBench_LeftShiftAndTypeBug(t *testing.T) {
	checkZkcBench(t, "zkc/bench/left_shift_and_type_bug", field.BLS12_377, field.KOALABEAR_16)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcBench(t *testing.T, test string, fields ...field.Config) {
	var config = util.DEFAULT_CONFIG.Fields(fields...)
	//
	util.CheckValid(t, test, "zkc", config)
}
