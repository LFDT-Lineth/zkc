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
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

func Test_Bench_Counter(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/counter", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_ByteDecomp(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/byte_decomposition", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_BitDecomp(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/bit_decomposition", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_BitShift(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/bit_shift", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_ByteSorting(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/byte_sorting", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_WordSorting(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/word_sorting", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Multiplier(t *testing.T) {
	util.CheckCorset(t, false, "corset/bench/multiplier", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Memory16u8(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/memory16u8", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Memory32u32(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/memory32u32", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Memory32u64(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/memory32u64", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Bench_Adder(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/adder", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Fields(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/fields", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

//#834
// func Test_Bench_TicTacToe(t *testing.T) {
// 	util.Check(t, true, "corset/bench/tic_tac_toe", field.BLS12_377, field.KOALABEAR_16)
// }

// NOTE: the real modules below are not tested for GF_8209 for performance
// reasons, which is reasonable given that they all use large registers (e.g.
// u128, etc).  This distinguishes them from those tests above, which often user
// small registers (e.g. u16).

func Test_Bench_Add(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/add", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_BinStatic(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/bin-static", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Bin(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/bin", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Wcp(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/wcp", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Mxp(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/mxp", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Shf(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/shf", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Euc(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/euc", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Oob(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/oob", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Stp(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/stp", field.BLS12_377)
}

func Test_Bench_Mmio(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/mmio", field.BLS12_377)
}

func Test_Bench_Rom(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/rom", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Gas(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/gas", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Exp(t *testing.T) {
	util.CheckCorset(t, true, "corset/bench/exp", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Mul(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/mul", field.BLS12_377)
}

func Test_Bench_Mod(t *testing.T) {
	// FIXME: KOALABEAR_16 [PERF]
	util.CheckCorset(t, true, "corset/bench/mod", field.BLS12_377)
}
