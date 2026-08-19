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

func Test_Invalid_Constant_13(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_13")
}

func Test_Invalid_Constant_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/constant_invalid_14")
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

func Test_Invalid_Range_06(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/range_invalid_06")
}

// ===================================================================
// Modules
// ===================================================================

func Test_Invalid_Module_01(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/module_invalid_01")
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
func Test_Invalid_Lookup_14(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_14")
}
func Test_Invalid_Lookup_15(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_15")
}
func Test_Invalid_Lookup_18(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_18")
}
func Test_Invalid_Lookup_19(t *testing.T) {
	checkCorsetInvalid(t, "corset/invalid/lookup_invalid_19")
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
