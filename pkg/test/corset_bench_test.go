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
	util.CheckCorset(t, "corset/bench/counter", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_ByteDecomp(t *testing.T) {
	util.CheckCorset(t, "corset/bench/byte_decomposition", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_BitDecomp(t *testing.T) {
	util.CheckCorset(t, "corset/bench/bit_decomposition", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_BitShift(t *testing.T) {
	util.CheckCorset(t, "corset/bench/bit_shift", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_ByteSorting(t *testing.T) {
	util.CheckCorset(t, "corset/bench/byte_sorting", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_WordSorting(t *testing.T) {
	util.CheckCorset(t, "corset/bench/word_sorting", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Multiplier(t *testing.T) {
	util.CheckCorset(t, "corset/bench/multiplier", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Adder(t *testing.T) {
	util.CheckCorset(t, "corset/bench/adder", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Bench_Fields(t *testing.T) {
	util.CheckCorset(t, "corset/bench/fields", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// NOTE: the real modules below are not tested for GF_8209 for performance
// reasons, which is reasonable given that they all use large registers (e.g.
// u128, etc).  This distinguishes them from those tests above, which often user
// small registers (e.g. u16).

func Test_Bench_Add(t *testing.T) {
	util.CheckCorset(t, "corset/bench/add", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Euc(t *testing.T) {
	util.CheckCorset(t, "corset/bench/euc", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Bench_Gas(t *testing.T) {
	util.CheckCorset(t, "corset/bench/gas", field.BLS12_377, field.KOALABEAR_16)
}
