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

func Test_AsmBench_Add(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/add", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Bin(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/bin", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Blake2f(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/blake2f", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Euc(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/euc", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
func Test_AsmBench_Exp(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/exp", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Gas(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/gas", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
func Test_AsmBench_Mul(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/mul", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Oob(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/oob", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Shf(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/shf", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Mod(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/mod", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Stp(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/stp", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Trm(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/trm", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_Wcp(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/wcp", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmBench_RlpUtils(t *testing.T) {
	util.CheckWithFields(t, false, "asm/bench/rlp_utils", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
