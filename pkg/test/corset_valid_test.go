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
	util.CheckCorset(t, false, "corset/valid/basic_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// Ignored because uses a negative constant.
//
// func Test_Valid_Basic_05(t *testing.T) {
// 	util.Check(t, false, "corset/valid/basic_05", field.BLS12_377, field.KOALABEAR_16)
// }

func Test_Valid_Basic_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Basic_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Basic_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Basic_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Basic_14(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Basic_15(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/basic_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Constants Tests
// ===================================================================
func Test_Valid_Constant_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_14(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_15(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Constant_16(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constant_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Alias Tests
// ===================================================================
func Test_Valid_Alias_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Alias_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Alias_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Alias_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Alias_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Alias_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/alias_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Domain Tests
// ===================================================================

func Test_Valid_Domain_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/domain_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Domain_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/domain_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Domain_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/domain_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Block Tests
// ===================================================================

func Test_Valid_Block_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/block_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Block_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/block_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Block_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/block_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Block_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/block_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Logical Tests
// ===================================================================

func Test_Valid_Logic_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/logic_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Logic_02(t *testing.T) {
	// Performance
	util.CheckCorset(t, false, "corset/valid/logic_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Property Tests
// ===================================================================

func Test_Valid_Property_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/property_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Property_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/property_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Property_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/property_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Property_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/property_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Shift Tests
// ===================================================================

func Test_Valid_Shift_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Shift_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Shift_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/shift_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Spillage Tests
// ===================================================================

func Test_Valid_Spillage_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Spillage_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/spillage_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Normalisation Tests
// ===================================================================

func Test_Valid_Norm_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Norm_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/norm_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// If-Zero
// ===================================================================

func Test_Valid_If_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_If_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_If_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_14(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_15(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_16(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_17(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_17", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_18(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_18", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_19(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_19", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_20(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_20", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_21(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_21", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_If_22(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/if_22", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Guards
// ===================================================================

func Test_Valid_Guard_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/guard_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/guard_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/guard_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/guard_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Guard_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/guard_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Types
// ===================================================================

func Test_Valid_Type_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Type_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/type_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Range Constraints
// ===================================================================

func Test_Valid_Range_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/range_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Range_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/range_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Range_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/range_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Range_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/range_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// #1247 --- This is an issue as we have a potentially negative expression used
// in a range constraint.
//
// func Test_Valid_Range_05(t *testing.T) {
//  util.Check(t, false, "corset/valid/range_05", field.BLS12_377, field.KOALABEAR_16)
// }

// ===================================================================
// Constant Propagation
// ===================================================================

func Test_Valid_ConstExpr_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constexpr_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constexpr_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constexpr_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constexpr_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_ConstExpr_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/constexpr_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Modules
// ===================================================================

func Test_Valid_Module_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Module_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/module_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// NOTE: uses conditional module
//
// func Test_Valid_Module_11(t *testing.T) {
// 	test_util.Check(t, false, "corset/valid/module_11", field.BLS12_377, field.KOALABEAR_16)
// }

// ===================================================================
// Permutations
// ===================================================================

func Test_Valid_Permute_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Permute_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/permute_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Sorting Constraints
// ===================================================================

func Test_Valid_Sorted_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Sorted_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Sorted_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/sorted_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_StrictSorted_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/strictsorted_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_StrictSorted_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/strictsorted_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_StrictSorted_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/strictsorted_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_StrictSorted_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/strictsorted_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_StrictSorted_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/strictsorted_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Lookups
// ===================================================================

func Test_Valid_Lookup_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_14(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_15(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_16(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_17(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_17", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_18(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_18", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_19(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_19", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_20(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_20", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_21(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_21", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_22(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_22", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_23(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_23", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_24(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_24", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_25(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_25", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Lookup_26(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_26", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_27(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_27", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_28(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_28", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_29(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_29", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Lookup_30(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/lookup_30", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Interleaving
// ===================================================================

func Test_Valid_Interleave_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Interleave_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Interleave_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Interleave_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Interleave_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Interleave_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Interleave_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/interleave_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Functions
// ===================================================================

func Test_Valid_Fun_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Fun_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/fun_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Pure Functions
// ===================================================================

func Test_Valid_PureFun_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_PureFun_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_PureFun_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/purefun_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// For Loops
// ===================================================================

func Test_Valid_For_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_For_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_For_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_For_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_For_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_For_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/for_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Arrays
// ===================================================================

func Test_Valid_Array_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Array_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/array_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Reduce
// ===================================================================

func Test_Valid_Reduce_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/reduce_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Reduce_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/reduce_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Reduce_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/reduce_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Reduce_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/reduce_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Reduce_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/reduce_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Debug
// ===================================================================

func Test_Valid_Debug_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/debug_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Debug_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/debug_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Debug_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/debug_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Perspectives
// ===================================================================

func Test_Valid_Perspective_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_14(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_14", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_15(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_15", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_16(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_16", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_17(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_17", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_18(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_18", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_19(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_19", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_20(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_20", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_21(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_21", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_22(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_22", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_23(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_23", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_24(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_24", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_26(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_26", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_27(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_27", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_28(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_28", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_29(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_29", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_30(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_30", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Perspective_31(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/perspective_31", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Let
// ===================================================================

func Test_Valid_Let_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_06(t *testing.T) {
	// FIXME: GF_8209 [???]
	util.CheckCorset(t, false, "corset/valid/let_06", field.BLS12_377, field.KOALABEAR_16)
}

func Test_Valid_Let_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Let_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Let_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/let_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Native Computations
// ===================================================================

func Test_Valid_Native_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Native_04(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Native_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_Native_06(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_07(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_08(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_09(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_09", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_10(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_10", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_11(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_11", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_12(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_12", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Native_13(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/native_13", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// defcomputedcolumn
// ===================================================================

func Test_Valid_ComputedColumn_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/computedcolumn_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_ComputedColumn_02(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/computedcolumn_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
func Test_Valid_ComputedColumn_03(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/computedcolumn_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// func Test_Valid_ComputedColumn_04(t *testing.T) {
// 	util.CheckCorset(t, false, "corset/valid/computedcolumn_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
// }

func Test_Valid_ComputedColumn_05(t *testing.T) {
	util.CheckCorset(t, false, "corset/valid/computedcolumn_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Standard Library Tests
// ===================================================================

func Test_Valid_Stdlib_01(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/stdlib_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_02(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/stdlib_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_03(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/stdlib_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_04(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/stdlib_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Valid_Stdlib_05(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/stdlib_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// ===================================================================
// Misc
// ===================================================================

func Test_Valid_Cfg_01(t *testing.T) {
	util.CheckCorset(t, true, "corset/valid/cfg_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
