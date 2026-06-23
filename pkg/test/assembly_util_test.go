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

func Test_AsmUtil_Byte(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/byte", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_BitRor64(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/bit_ror64", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_BitSar(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/bit_sar", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_BitShr(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/bit_shr", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_BitShl(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/bit_shl", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_ByteCounting(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/byte_counting", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_FillBytes(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/fill_bytes", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_FirstByte(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/first_byte", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_G1G2(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/g1g2", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_Log2(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/log2", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_Log256(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/log256", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
func Test_AsmUtil_Max2(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/max2", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
func Test_AsmUtil_Max3(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/max3", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_Min(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/min", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_SetByte(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/set_byte", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_Signextend(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/signextend", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_Padding(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/padding", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_ByteSize(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/byte_size", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}

func Test_AsmUtil_SwitchEndian(t *testing.T) {
	util.CheckWithFields(t, false, "asm/util/switch_endian", util.ASM_MAX_PADDING, field.BLS12_377, field.KOALABEAR_16)
}
