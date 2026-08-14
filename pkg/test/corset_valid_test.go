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

// ===================================================================
// Basic Tests
// ===================================================================

func Test_Valid_Basic_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// Ignored because uses a negative constant.
//
// func Test_Valid_Basic_05(t *testing.T) {
// 	util.Check(t, false, "corset/valid/basic_05", field.BLS12_377, field.KOALABEAR_16)
// }

func Test_Valid_Basic_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_11(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Basic_14(t *testing.T) {
	util.CheckCorset(t, "corset/valid/basic_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Constants Tests
// ===================================================================
func Test_Valid_Constant_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_11(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_12(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_13(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_14(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_15(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_16(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constant_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Domain Tests
// ===================================================================

func Test_Valid_Domain_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/domain_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Domain_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/domain_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Domain_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/domain_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Logical Tests
// ===================================================================

func Test_Valid_Logic_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/logic_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Logic_02(t *testing.T) {
	// Performance
	util.CheckCorset(t, "corset/valid/logic_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Shift Tests
// ===================================================================

func Test_Valid_Shift_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Shift_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/shift_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Spillage Tests
// ===================================================================

func Test_Valid_Spillage_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/spillage_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// If-Zero
// ===================================================================

func Test_Valid_If_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_11(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_If_12(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_If_15(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_16(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_18(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_18", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_19(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_19", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_20(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_20", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_21(t *testing.T) {
	util.CheckCorset(t, "corset/valid/if_21", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Guards
// ===================================================================

func Test_Valid_Guard_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/guard_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/guard_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/guard_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/guard_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/guard_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Types
// ===================================================================

func Test_Valid_Type_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_11(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_12(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_13(t *testing.T) {
	util.CheckCorset(t, "corset/valid/type_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Range Constraints
// ===================================================================

func Test_Valid_Range_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/range_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Range_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/range_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Range_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/range_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Constant Propagation
// ===================================================================

func Test_Valid_ConstExpr_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constexpr_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constexpr_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constexpr_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constexpr_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/constexpr_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Modules
// ===================================================================

func Test_Valid_Module_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/module_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// NOTE: uses conditional module
//
// func Test_Valid_Module_11(t *testing.T) {
// 	test_util.Check(t, false, "corset/valid/module_11", field.BLS12_377, field.KOALABEAR_16)
// }

// ===================================================================
// Lookups
// ===================================================================

func Test_Valid_Lookup_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_08(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_09(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_10(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_11(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_12(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_15(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_16(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_17(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_17", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_18(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_18", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_19(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_19", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_20(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_20", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_21(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_21", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_22(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_22", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_23(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_23", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_24(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_24", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_25(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_25", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_26(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_26", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_27(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_27", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_28(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_28", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_29(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_29", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_30(t *testing.T) {
	util.CheckCorset(t, "corset/valid/lookup_30", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Arrays
// ===================================================================

func Test_Valid_Array_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_03(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_06(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_07(t *testing.T) {
	util.CheckCorset(t, "corset/valid/array_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Standard Library Tests
// ===================================================================

func Test_Valid_Stdlib_01(t *testing.T) {
	util.CheckCorset(t, "corset/valid/stdlib_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_02(t *testing.T) {
	util.CheckCorset(t, "corset/valid/stdlib_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_04(t *testing.T) {
	util.CheckCorset(t, "corset/valid/stdlib_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_05(t *testing.T) {
	util.CheckCorset(t, "corset/valid/stdlib_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
