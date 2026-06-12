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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// DEFAULT_UNIT_CONFIG provides a default configuration for unit tests.
var DEFAULT_UNIT_CONFIG = util.DEFAULT_CONFIG.
	Words(vm.WORD_UINT, vm.WORD_UINT64).
	Constraints(true).
	Bytecode(true)

// ===================================================================
// Basic Tests
// ===================================================================

func Test_ZkcUnit_Basic_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_01", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_02", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_03", DEFAULT_UNIT_CONFIG.Splitting(true))
}
func Test_ZkcUnit_Basic_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_05(t *testing.T) {
	// TODO: support static memory for constraints
	checkZkcUnit(t, "zkc/unit/basic_05", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_06", DEFAULT_UNIT_CONFIG)
}
func Test_ZkcUnit_Basic_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_09(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_09", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_10", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_11(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_11", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_12(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_12", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_13(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_13", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_14(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_14", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_15(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_15", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_16(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_16", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_17", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_18", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_19(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_19", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_20(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_20", DEFAULT_UNIT_CONFIG)
}
func Test_ZkcUnit_Basic_21(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_21", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_22(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_22", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_23(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_23", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_24(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_24", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_25(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_25", DEFAULT_UNIT_CONFIG)
}
func Test_ZkcUnit_Basic_26(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_26", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_27(t *testing.T) {
	// TODO: register splitting (runs under bytecode interpreter on a wide field)
	checkZkcUnit(t, "zkc/unit/basic_27", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_28(t *testing.T) {
	// TODO: register splitting (runs under bytecode interpreter on a wide field)
	checkZkcUnit(t, "zkc/unit/basic_28", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_29(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_29", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_30(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_30", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_31(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_31", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_32(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_32", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_33(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_33", DEFAULT_UNIT_CONFIG.Splitting(true))
}

func Test_ZkcUnit_Basic_34(t *testing.T) {
	// TODO: register splitting (runs under bytecode interpreter on a wide field)
	checkZkcUnit(t, "zkc/unit/basic_34", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_35(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_35", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_36(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_36", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_37(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_37", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_38(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_38", DEFAULT_UNIT_CONFIG)
}

// NOTE: this is a tricky test case.  Its not clear whether we want to support
// this test case or not.
//
// func Test_ZkcUnit_Basic_39(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/basic_39", UNIT_CONFIG)
// }

func Test_ZkcUnit_Basic_40(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_40", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_41(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_41", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_42(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_42", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_43(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_43", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_44(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_44", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_45(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_45", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// ===================================================================
// If-Else-If Tests
// ===================================================================

func Test_ZkcUnit_IfElse_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_IfElse_04(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_04", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_IfElse_05(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_05", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_IfElse_06(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_06", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_IfElse_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_07", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_IfElse_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_08", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Constant Tests
// ===================================================================

func Test_ZkcUnit_Const_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/const_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Const_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_05", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Const_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_06", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Const_07(t *testing.T) {
	// NOTE: u128 registers cannot be lowered to a 64-bit word machine, hence
	// this test is restricted to fields where the Uint64 run is skipped.
	checkZkcUnit(t, "zkc/unit/const_07", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// ===================================================================
// Fixed-size array Tests
// ===================================================================

func Test_ZkcUnit_FixedArray_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_09", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_10", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_11", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_12", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_13(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_13", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_14(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_14", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_15", DEFAULT_UNIT_CONFIG)
}

// Destructuring test, issue #1818
/*func Test_ZkcUnit_FixedArray_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_16", UNIT_CONFIG)
}*/

func Test_ZkcUnit_FixedArray_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_17", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_18", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_19(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_19", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_20(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_20", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_FixedArray_21(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_21", DEFAULT_UNIT_CONFIG)
}

// Issue #1820, cmp with extern access
// func Test_ZkcUnit_FixedArray_22(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/fixed_array_22", UNIT_CONFIG)
// }

func Test_ZkcUnit_FixedArray_23(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_23", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Type Tests
// ===================================================================

func Test_ZkcUnit_Type_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_09", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Type_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_10", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Control Flow Tests
// ===================================================================

func Test_ZkcUnit_Cfg_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cfg_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Cfg_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cfg_02", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Loop Tests
// ===================================================================

func Test_ZkcUnit_While_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/while_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_While_02(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/while_02", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_While_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/while_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_For_01(t *testing.T) {
	// TODO: bitwise destruct
	checkZkcUnit(t, "zkc/unit/for_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_For_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/for_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_For_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/for_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_For_04(t *testing.T) {
	// TODO: duplicate variable declaration #1801
	checkZkcUnit(t, "zkc/unit/for_04", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// ===================================================================
// Break Tests
// ===================================================================

func Test_ZkcUnit_Break_01(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/break_01", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// ===================================================================
// Continue Tests
// ===================================================================

func Test_ZkcUnit_Continue_01(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/continue_01", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// ===================================================================
// Bitwise Tests
// ===================================================================

func Test_ZkcUnit_Bitwise_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_05(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_05", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Bitwise_06(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_06", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Bitwise_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_07", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Bitwise_08(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_08", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Bitwise_09(t *testing.T) {
	// TODO: duplicate module #1802
	checkZkcUnit(t, "zkc/unit/bitwise_09", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Bitwise_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_10", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_11", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_12", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_13(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_13", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Bitwise_14(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_14", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Bitwise_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_15", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Bitwise_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_16", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Bitwise_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_17", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Bitwise_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_18", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT).Constraints(false))
}

func Test_ZkcUnit_Bitwise_19(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_19", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT).Constraints(false))
}

// ===================================================================
// Shift Tests
// ===================================================================

func Test_ZkcUnit_Shift_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Shift_04(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_04", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Shift_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_08(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_08", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Shift_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_09", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_10", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_11(t *testing.T) {
	// TODO: unexpected instruction #1803
	checkZkcUnit(t, "zkc/unit/shift_11", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_12", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// ===================================================================
// Static Initialiser Tests
// ===================================================================

func Test_ZkcUnit_Static_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_01", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Static_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_02", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Cast Tests
// ===================================================================

func Test_ZkcUnit_Cast_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Cast_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Cast_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Cast_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Cast_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_05", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Division Tests
// ===================================================================

func Test_ZkcUnit_Div_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_01", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// TODO: register splitting
func Test_ZkcUnit_Div_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_02", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// TODO: KoalaBear once register splitting is working
func Test_ZkcUnit_Div_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_03", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// ===================================================================
// Remainder Tests
// ===================================================================

func Test_ZkcUnit_Rem_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Rem_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_02", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Rem_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_03", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Call Tests
// ===================================================================

func Test_ZkcUnit_Call_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Call_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_02", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Ternary Tests
// ===================================================================

func Test_ZkcUnit_Ternary_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Ternary_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Ternary_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Ternary_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Ternary_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Ternary_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_06", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Switch Tests
// ===================================================================

func Test_ZkcUnit_Switch_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/switch_07", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

func Test_ZkcUnit_Switch_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_09(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/switch_09", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377))
}

// ===================================================================
// Printf Tests
// ===================================================================

func Test_ZkcUnit_Printf_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Printf_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Printf_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Printf_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_04", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Debug Function Tests
// ===================================================================

func Test_ZkcUnit_Debug_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/debug_01", DEFAULT_UNIT_CONFIG.Bytecode(true))
}

func Test_ZkcUnit_Debug_02(t *testing.T) {
	// Quiet mode elides the call to the #[debug] function, whose body would
	// otherwise fail.
	checkZkcUnit(t, "zkc/unit/debug_02", DEFAULT_UNIT_CONFIG.Quiet(true).Bytecode(true))
}

func Test_ZkcUnit_Debug_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/debug_03", DEFAULT_UNIT_CONFIG.Bytecode(true))
}

func Test_ZkcUnit_Debug_04(t *testing.T) {
	// As Debug_03, but in quiet mode (all debug calls elided).
	checkZkcUnit(t, "zkc/unit/debug_03", DEFAULT_UNIT_CONFIG.Quiet(true).Bytecode(true))
}

// ===================================================================
// Include Tests
// ===================================================================

func Test_ZkcUnit_Include_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/include_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Include_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/include_02", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Skip If (VM inst) Tests
// ===================================================================

func Test_ZkcUnit_SkipIf_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_SkipIf_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_SkipIf_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_SkipIf_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_SkipIf_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_05", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcUnit(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config.Bytecode(true))
}
