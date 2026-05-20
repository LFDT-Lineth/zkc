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
	"github.com/consensys/go-corset/pkg/zkc/vm"
)

// ===================================================================
// Basic Tests
// ===================================================================

func Test_ZkcUnit_Basic_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_02", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_03", util.DEFAULT_CONFIG.Constraints(true).Splitting(true))
}
func Test_ZkcUnit_Basic_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_05(t *testing.T) {
	// TODO: support static memory for constraints
	checkZkcUnit(t, "zkc/unit/basic_05", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_06", util.DEFAULT_CONFIG.Constraints(true))
}
func Test_ZkcUnit_Basic_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_07", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_08", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_09(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_09", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_10(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_10", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_11(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_11", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_12(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_12", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_13(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_13", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_14(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_14", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_15(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_15", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_16(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_16", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_17", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_18", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_19(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_19", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_20(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_20", util.DEFAULT_CONFIG.Constraints(true))
}
func Test_ZkcUnit_Basic_21(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_21", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_22(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_22", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_23(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_23", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_24(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_24", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_25(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_25", util.DEFAULT_CONFIG.Constraints(true))
}
func Test_ZkcUnit_Basic_26(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_26", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_27(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_27", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_28(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_28", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_29(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_29", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_30(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_30", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_31(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_31", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_32(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_32", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Basic_33(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_33", util.DEFAULT_CONFIG.Constraints(true).Splitting(true))
}

func Test_ZkcUnit_Basic_34(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/basic_34", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Basic_35(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_35", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_36(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_36", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_37(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_37", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_38(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_38", util.DEFAULT_CONFIG)
}

// NOTE: this is a tricky test case.  Its not clear whether we want to support
// this test case or not.
//
// func Test_ZkcUnit_Basic_39(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/basic_39", util.DEFAULT_CONFIG)
// }

func Test_ZkcUnit_Basic_40(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_40", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Basic_41(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_41", util.DEFAULT_CONFIG)
}

// ===================================================================
// If-Else-If Tests
// ===================================================================

func Test_ZkcUnit_IfElse_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_IfElse_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_IfElse_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_03", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_IfElse_04(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_04", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_IfElse_05(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_05", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_IfElse_06(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_06", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_IfElse_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/ifelse_07", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_IfElse_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_08", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Constant Tests
// ===================================================================

func Test_ZkcUnit_Const_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Const_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Const_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/const_03", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Const_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_04", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Fixed-size array Tests
// ===================================================================

func Test_ZkcUnit_FixedArray_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_05", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_06", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_07", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_08", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_09", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_FixedArray_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_10", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_11", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_12", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_13(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_13", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_14(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_14", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_15", util.DEFAULT_CONFIG)
}

// Destructuring test, issue #1818
/*func Test_ZkcUnit_FixedArray_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_16", util.DEFAULT_CONFIG)
}*/

func Test_ZkcUnit_FixedArray_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_17", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_18", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_19(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_19", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_20(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_20", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_FixedArray_21(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_21", util.DEFAULT_CONFIG)
}

// Issue #1820, cmp with extern access
// func Test_ZkcUnit_FixedArray_22(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/fixed_array_22", util.DEFAULT_CONFIG)
// }

func Test_ZkcUnit_FixedArray_23(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/fixed_array_23", util.DEFAULT_CONFIG)
}

// ===================================================================
// Type Tests
// ===================================================================

func Test_ZkcUnit_Type_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_05", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_06", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_07", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_08", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_09", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Type_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/type_10", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Control Flow Tests
// ===================================================================

func Test_ZkcUnit_Cfg_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cfg_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Cfg_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cfg_02", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Loop Tests
// ===================================================================

func Test_ZkcUnit_While_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/while_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_While_02(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/while_02", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_While_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/while_03", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_For_01(t *testing.T) {
	// TODO: bitwise destruct
	checkZkcUnit(t, "zkc/unit/for_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_For_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/for_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_For_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/for_03", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_For_04(t *testing.T) {
	// TODO: duplicate variable declaration #1801
	checkZkcUnit(t, "zkc/unit/for_04", util.DEFAULT_CONFIG)
}

// ===================================================================
// Break Tests
// ===================================================================

func Test_ZkcUnit_Break_01(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/break_01", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

// ===================================================================
// Continue Tests
// ===================================================================

func Test_ZkcUnit_Continue_01(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/continue_01", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

// ===================================================================
// Bitwise Tests
// ===================================================================

func Test_ZkcUnit_Bitwise_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_05(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_05", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Bitwise_06(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_06", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Bitwise_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_07", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Bitwise_08(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/bitwise_08", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Bitwise_09(t *testing.T) {
	// TODO: duplicate module #1802
	checkZkcUnit(t, "zkc/unit/bitwise_09", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Bitwise_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_10", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_11", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_12", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Bitwise_13(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_13", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Bitwise_14(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_14", util.DEFAULT_CONFIG)
}

// ===================================================================
// Shift Tests
// ===================================================================

func Test_ZkcUnit_Shift_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_03(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_03", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Shift_04(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_04", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Shift_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_05", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_06", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_07", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_08(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/shift_08", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Shift_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_09", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_10", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Shift_11(t *testing.T) {
	// TODO: unexpected instruction #1803
	checkZkcUnit(t, "zkc/unit/shift_11", util.DEFAULT_CONFIG)
}

// ===================================================================
// Static Initialiser Tests
// ===================================================================

func Test_ZkcUnit_Static_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Static_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_02", util.DEFAULT_CONFIG)
}

// ===================================================================
// Cast Tests
// ===================================================================

func Test_ZkcUnit_Cast_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Cast_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Cast_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Cast_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Cast_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cast_05", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Division Tests
// ===================================================================

func Test_ZkcUnit_Div_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_01", util.DEFAULT_CONFIG.Constraints(true).Fields(field.BLS12_377))
}

// TODO: register splitting
func Test_ZkcUnit_Div_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_02", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

// TODO: KoalaBear once register splitting is working
func Test_ZkcUnit_Div_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_03", util.DEFAULT_CONFIG.Constraints(true).Fields(field.BLS12_377))
}

// ===================================================================
// Remainder Tests
// ===================================================================

func Test_ZkcUnit_Rem_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Rem_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_02", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Rem_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_03", util.DEFAULT_CONFIG)
}

// ===================================================================
// Call Tests
// ===================================================================

func Test_ZkcUnit_Call_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Call_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_02", util.DEFAULT_CONFIG)
}

// ===================================================================
// Ternary Tests
// ===================================================================

func Test_ZkcUnit_Ternary_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Ternary_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Ternary_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Ternary_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Ternary_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_05", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Ternary_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ternary_06", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Switch Tests
// ===================================================================

func Test_ZkcUnit_Switch_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_05", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_06", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_07(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/switch_07", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

func Test_ZkcUnit_Switch_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_08", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_Switch_09(t *testing.T) {
	// TODO: register splitting
	checkZkcUnit(t, "zkc/unit/switch_09", util.DEFAULT_CONFIG.Fields(field.BLS12_377).Constraints(true))
}

// ===================================================================
// Printf Tests
// ===================================================================

func Test_ZkcUnit_Printf_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Printf_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_02", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Printf_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_03", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Printf_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/printf_04", util.DEFAULT_CONFIG)
}

// ===================================================================
// Include Tests
// ===================================================================

func Test_ZkcUnit_Include_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/include_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Include_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/include_02", util.DEFAULT_CONFIG)
}

// ===================================================================
// Skip If (VM inst) Tests
// ===================================================================

func Test_ZkcUnit_SkipIf_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_01", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_SkipIf_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_02", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_SkipIf_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_03", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_SkipIf_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_04", util.DEFAULT_CONFIG.Constraints(true))
}

func Test_ZkcUnit_SkipIf_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/skip_if_05", util.DEFAULT_CONFIG.Constraints(true))
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcUnit(t *testing.T, test string, config util.Config) {
	// FIXME: temporarily disable all constraints checks for now
	config = config.Constraints(false)
	util.CheckValid(t, test, "zkc", config.Words(vm.WORD_UINT, vm.WORD_UINT64))
}
