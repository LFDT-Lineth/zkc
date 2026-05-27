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

func Test_AsmUnit_BitShift(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/bit_shift", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_ByteAt(t *testing.T) {
	// ignore GF_8209 for performance reasons
	util.CheckWithFields(t, false, "asm/unit/byte_at", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16)
}

func Test_AsmUnit_ByteShift(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/byte_shift", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Cast(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/cast", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Counter(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/counter", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Counter256(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/counter256", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Dec4(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/dec4", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Dec251(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/dec251", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Diamond(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/diamond", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_ParseNonDecimal(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/parse_nondecimal", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Diff(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/diff", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Div(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/div", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_DivSeq(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/div_seq", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_Div7(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/div7", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_Div16383(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/div16383", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_FastPow(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fast_pow", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_FnCall_01(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_01", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_FnCall_02(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_02", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_FnCall_03(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_03", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_FnCall_04(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_04", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_FnCall_05(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_05", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_FnCall_06(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/fncall_06", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_Gf251(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/gf251", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Inc(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/inc", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Max(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/max", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MixedLookup_01(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_01", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_MixedLookup_02(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_02", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MixedLookup_03(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_03", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MixedLookup_04(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_04", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MixedLookup_05(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_05", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MixedLookup_06(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/mixed_lookup_06", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MultiLine_01(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/multiline_01", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_MultiLine_02(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/multiline_02", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_NonUniformIf(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/nonuniform_if", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_RecPow(t *testing.T) {
	// GF_8209 causes malformed assignment
	util.CheckWithFields(t, false, "asm/unit/rec_pow", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16)
}
func Test_AsmUnit_SlowPow(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/slow_pow", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Sub(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/sub", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Ternary(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/ternary", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}
func Test_AsmUnit_Ternary2(t *testing.T) {
	util.CheckWithFields(t, false, "asm/unit/ternary2", util.ASM_MAX_PADDING, field.BLS12_377,
		field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Misc
// ===================================================================

func Test_AsmUnit_Cfg_01(t *testing.T) {
	// Ignoring KOALABEAR_16 and GF_8209 for now
	util.CheckCorset(t, true, "asm/unit/cfg_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_AsmUnit_Cfg_02(t *testing.T) {
	util.CheckCorset(t, true, "asm/unit/cfg_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
