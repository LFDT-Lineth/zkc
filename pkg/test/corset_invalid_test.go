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

	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// ===================================================================
// Basic Tests
// ===================================================================

func Test_Invalid_Basic_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_01")
}

func Test_Invalid_Basic_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_02")
}

func Test_Invalid_Basic_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_03")
}

func Test_Invalid_Basic_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_04")
}

func Test_Invalid_Basic_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_05")
}

func Test_Invalid_Basic_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_06")
}

func Test_Invalid_Basic_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_07")
}

func Test_Invalid_Basic_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_08")
}

func Test_Invalid_Basic_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_09")
}

func Test_Invalid_Basic_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_10")
}

func Test_Invalid_Basic_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_11")
}

func Test_Invalid_Basic_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_12")
}

func Test_Invalid_Basic_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_13")
}

func Test_Invalid_Basic_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_14")
}

func Test_Invalid_Basic_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_15")
}

func Test_Invalid_Basic_16(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_16")
}
func Test_Invalid_Basic_17(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_17")
}
func Test_Invalid_Basic_18(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_18")
}

func Test_Invalid_Basic_19(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_19")
}
func Test_Invalid_Basic_20(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/basic_invalid_20")
}
func Test_Invalid_Logic_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/logic_invalid_01")
}

func Test_Invalid_Logic_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/logic_invalid_02")
}

func Test_Invalid_Logic_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/logic_invalid_03")
}

// ===================================================================
// Constant Tests
// ===================================================================
func Test_Invalid_Constant_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_01")
}

func Test_Invalid_Constant_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_02")
}

func Test_Invalid_Constant_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_03")
}

func Test_Invalid_Constant_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_04")
}

func Test_Invalid_Constant_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_05")
}

/* Recursive --- #406
  func Test_Invalid_Constant_06(t *testing.T) {
	CheckInvalid(t, "corset/invalid/constant_invalid_06")
} */

/* Recursive --- #406
  func Test_Invalid_Constant_07(t *testing.T) {
	CheckInvalid(t, "corset/invalid/constant_invalid_07")
}
*/
/* Recursive --- #406
  func Test_Invalid_Constant_08(t *testing.T) {
	CheckInvalid(t, "corset/invalid/constant_invalid_08")
} */

func Test_Invalid_Constant_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_09")
}

func Test_Invalid_Constant_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_10")
}

func Test_Invalid_Constant_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_11")
}

func Test_Invalid_Constant_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_12")
}

func Test_Invalid_Constant_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_13")
}

func Test_Invalid_Constant_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_14")
}

func Test_Invalid_Constant_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_15")
}

func Test_Invalid_Constant_16(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_16")
}

func Test_Invalid_Constant_17(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_17")
}

func Test_Invalid_Constant_18(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_18")
}

func Test_Invalid_Constant_19(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_19")
}

func Test_Invalid_Constant_20(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_20")
}

func Test_Invalid_Constant_21(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_21")
}

func Test_Invalid_Constant_22(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_22")
}

func Test_Invalid_Constant_23(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_23")
}

func Test_Invalid_Constant_24(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_24")
}

func Test_Invalid_Constant_25(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_25")
}

func Test_Invalid_Constant_26(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_26")
}

// ===================================================================
// Alias Tests
// ===================================================================
func Test_Invalid_Alias_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_01")
}

func Test_Invalid_Alias_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_02")
}

func Test_Invalid_Alias_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_03")
}

func Test_Invalid_Alias_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_04")
}

func Test_Invalid_Alias_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_05")
}

func Test_Invalid_Alias_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_06")
}

func Test_Invalid_Alias_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/alias_invalid_07")
}

// ===================================================================
// Property Tests
// ===================================================================
func Test_Invalid_Property_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/property_invalid_01")
}

func Test_Invalid_Property_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/property_invalid_02")
}

// ===================================================================
// Shift Tests
// ===================================================================

func Test_Invalid_Shift_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/shift_invalid_01")
}

func Test_Invalid_Shift_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/shift_invalid_02")
}

// ===================================================================
// Normalisation Tests
// ===================================================================

func Test_Invalid_Norm_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/norm_invalid_01")
}

// ===================================================================
// If-Zero
// ===================================================================

func Test_Invalid_If_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/if_invalid_01")
}

func Test_Invalid_If_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/if_invalid_02")
}

func Test_Invalid_If_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/if_invalid_03")
}

// ===================================================================
// Types
// ===================================================================

func Test_Invalid_Type_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_01")
}

func Test_Invalid_Type_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_02")
}

func Test_Invalid_Type_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_03")
}

func Test_Invalid_Type_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_04")
}

func Test_Invalid_Type_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_05")
}

func Test_Invalid_Type_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_06")
}

func Test_Invalid_Type_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_07")
}

func Test_Invalid_Type_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_08")
}

func Test_Invalid_Type_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_09")
}

func Test_Invalid_Type_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_10")
}

func Test_Invalid_Type_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_11")
}

func Test_Invalid_Type_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_12")
}

func Test_Invalid_Type_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_13")
}

func Test_Invalid_Type_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/type_invalid_14")
}

// ===================================================================
// Range Constraints
// ===================================================================

func Test_Invalid_Range_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_01")
}

func Test_Invalid_Range_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_02")
}

func Test_Invalid_Range_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_03")
}

func Test_Invalid_Range_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_04")
}
func Test_Invalid_Range_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_05")
}

// ===================================================================
// Modules
// ===================================================================

func Test_Invalid_Module_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/module_invalid_01")
}

// ===================================================================
// Permutations
// ===================================================================

func Test_Invalid_Permute_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_01")
}

func Test_Invalid_Permute_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_02")
}

func Test_Invalid_Permute_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_03")
}

func Test_Invalid_Permute_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_04")
}

/* func Test_Invalid_Permute_05(t *testing.T) {
	CheckInvalid(t, "corset/invalid/permute_invalid_05")
} */

func Test_Invalid_Permute_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_07")
}

func Test_Invalid_Permute_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_08")
}
func Test_Invalid_Permute_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_09")
}
func Test_Invalid_Permute_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_10")
}
func Test_Invalid_Permute_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/permute_invalid_11")
}

// ===================================================================
// Sortings
// ===================================================================

func Test_Invalid_Sorted_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/sorted_invalid_01")
}

func Test_Invalid_Sorted_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/sorted_invalid_02")
}

func Test_Invalid_Sorted_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/sorted_invalid_03")
}
func Test_Invalid_Sorted_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/sorted_invalid_04")
}
func Test_Invalid_Sorted_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/sorted_invalid_05")
}

// ===================================================================
// Lookups
// ===================================================================

func Test_Invalid_Lookup_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_01")
}

func Test_Invalid_Lookup_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_02")
}
func Test_Invalid_Lookup_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_03")
}

func Test_Invalid_Lookup_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_04")
}

func Test_Invalid_Lookup_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_05")
}
func Test_Invalid_Lookup_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_06")
}
func Test_Invalid_Lookup_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_07")
}
func Test_Invalid_Lookup_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_08")
}
func Test_Invalid_Lookup_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_09")
}

func Test_Invalid_Lookup_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_10")
}

func Test_Invalid_Lookup_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_11")
}
func Test_Invalid_Lookup_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_12")
}
func Test_Invalid_Lookup_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_13")
}
func Test_Invalid_Lookup_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_14")
}
func Test_Invalid_Lookup_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_15")
}
func Test_Invalid_Lookup_16(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_16")
}
func Test_Invalid_Lookup_17(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_17")
}
func Test_Invalid_Lookup_18(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_18")
}
func Test_Invalid_Lookup_19(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_19")
}

// ===================================================================
// Interleavings
// ===================================================================

func Test_Invalid_Interleave_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_01")
}

func Test_Invalid_Interleave_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_02")
}

func Test_Invalid_Interleave_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_03")
}

func Test_Invalid_Interleave_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_04")
}

func Test_Invalid_Interleave_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_05")
}

func Test_Invalid_Interleave_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_06")
}

func Test_Invalid_Interleave_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_07")
}

func Test_Invalid_Interleave_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_08")
}

func Test_Invalid_Interleave_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_09")
}

func Test_Invalid_Interleave_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_10")
}

func Test_Invalid_Interleave_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_11")
}

func Test_Invalid_Interleave_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_12")
}
func Test_Invalid_Interleave_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_13")
}

func Test_Invalid_Interleave_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_14")
}
func Test_Invalid_Interleave_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_15")
}
func Test_Invalid_Interleave_16(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/interleave_invalid_16")
}

// ===================================================================
// Functions
// ===================================================================

func Test_Invalid_Fun_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fun_invalid_01")
}

func Test_Invalid_Fun_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fun_invalid_02")
}

func Test_Invalid_Fun_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fun_invalid_03")
}

func Test_Invalid_Fun_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fun_invalid_04")
}

// ===================================================================
// Pure Functions
// ===================================================================

func Test_Invalid_PureFun_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_01")
}

func Test_Invalid_PureFun_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_02")
}

func Test_Invalid_PureFun_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_03")
}

func Test_Invalid_PureFun_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_04")
}

func Test_Invalid_PureFun_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_05")
}

func Test_Invalid_PureFun_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_06")
}

func Test_Invalid_PureFun_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_07")
}

func Test_Invalid_PureFun_08(t *testing.T) {
	// tricky one
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_08")
}

func Test_Invalid_PureFun_09(t *testing.T) {
	// tricky one
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_09")
}

func Test_Invalid_PureFun_10(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_10")
}

func Test_Invalid_PureFun_11(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_11")
}

func Test_Invalid_PureFun_12(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_12")
}

func Test_Invalid_PureFun_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_13")
}

func Test_Invalid_PureFun_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_14")
}
func Test_Invalid_PureFun_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/purefun_invalid_15")
}

// ===================================================================
// For Loops
// ===================================================================
func Test_Invalid_For_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/for_invalid_01")
}

func Test_Invalid_For_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/for_invalid_02")
}

func Test_Invalid_For_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/for_invalid_03")
}

// ===================================================================
// Arrays
// ===================================================================
func Test_Invalid_Array_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_01")
}

func Test_Invalid_Array_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_02")
}

func Test_Invalid_Array_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_03")
}

func Test_Invalid_Array_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_04")
}

func Test_Invalid_Array_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_05")
}

func Test_Invalid_Array_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/array_invalid_06")
}

// ===================================================================
// Reduce
// ===================================================================

func Test_Invalid_Reduce_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/reduce_invalid_01")
}

func Test_Invalid_Reduce_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/reduce_invalid_02")
}

func Test_Invalid_Reduce_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/reduce_invalid_03")
}

func Test_Invalid_Reduce_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/reduce_invalid_04")
}

func Test_Invalid_Reduce_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/reduce_invalid_05")
}

// ===================================================================
// Debug
// ===================================================================

func Test_Invalid_Debug_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/debug_invalid_01")
}

func Test_Invalid_Debug_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/debug_invalid_02")
}

// ===================================================================
// Perspectives
// ===================================================================
func Test_Invalid_Perspective_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_01")
}

func Test_Invalid_Perspective_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_02")
}

func Test_Invalid_Perspective_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_03")
}

func Test_Invalid_Perspective_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_05")
}

func Test_Invalid_Perspective_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_06")
}

func Test_Invalid_Perspective_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/perspective_invalid_08")
}

// ===================================================================
// Perspectives
// ===================================================================
func Test_Invalid_Let_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_01")
}

func Test_Invalid_Let_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_02")
}
func Test_Invalid_Let_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_03")
}
func Test_Invalid_Let_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_04")
}
func Test_Invalid_Let_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_05")
}
func Test_Invalid_Let_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_06")
}

func Test_Invalid_Let_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/let_invalid_07")
}

// ===================================================================
// Computed Columns
// ===================================================================

func Test_Invalid_Compute_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_01")
}

func Test_Invalid_Compute_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_02")
}

func Test_Invalid_Compute_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_03")
}

func Test_Invalid_Compute_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_04")
}

func Test_Invalid_Compute_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_05")
}

func Test_Invalid_Compute_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_06")
}

func Test_Invalid_Compute_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_07")
}
func Test_Invalid_Compute_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/compute_invalid_08")
}

// ===================================================================
// defcomputedcolumn
// ===================================================================

func Test_Invalid_ComputedColumn_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_01")
}

func Test_Invalid_ComputedColumn_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_02")
}

func Test_Invalid_ComputedColumn_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_03")
}

func Test_Invalid_ComputedColumn_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_04")
}

func Test_Invalid_ComputedColumn_05(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_05")
}

func Test_Invalid_ComputedColumn_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_06")
}
func Test_Invalid_ComputedColumn_07(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_07")
}
func Test_Invalid_ComputedColumn_08(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_08")
}

func Test_Invalid_ComputedColumn_09(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/computedcolumn_invalid_09")
}

//
// #1089
// func Test_Invalid_ComputedColumn_10(t *testing.T) {
// 	CheckInvalid(t, "corset/invalid/computedcolumn_invalid_10")
// }
//
// #1089
// func Test_Invalid_ComputedColumn_11(t *testing.T) {
// 	CheckInvalid(t, "corset/invalid/computedcolumn_invalid_11")
// }

// ===================================================================
// Function calls
// ===================================================================

func Test_Invalid_FnCall_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fncall_invalid_01")
}
func Test_Invalid_FnCall_02(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fncall_invalid_02")
}
func Test_Invalid_FnCall_03(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fncall_invalid_03")
}
func Test_Invalid_FnCall_04(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/fncall_invalid_04")
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkCorsetInvalid(t *testing.T, test string) {
	util.CheckInvalid(t, test, "lisp", ";;error", compileCorsetFile)
}

func compileCorsetFile(srcfile source.File) []source.SyntaxError {
	var corsetConfig corset.CompilationConfig
	//
	_, _, errors := corset.CompileSourceFile(corsetConfig, srcfile)
	//
	return errors
}
