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
	"fmt"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// DEFAULT_UNIT_CONFIG provides a default configuration for unit tests.
var DEFAULT_UNIT_CONFIG = util.DEFAULT_CONFIG.
	Fields(field.KOALABEAR_16).
	Constraints(true).
	Splitting(true).
	Bytecode(true).
	GoGen(true)

// ===================================================================
// Basic Tests
// ===================================================================

func Test_ZkcUnit_Basic_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_03", DEFAULT_UNIT_CONFIG)
}
func Test_ZkcUnit_Basic_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_05(t *testing.T) {
	// TODO: support static memory for constraints
	checkZkcUnit(t, "zkc/unit/basic_05", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/basic_09", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_10", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_11", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_12", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_13(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_13", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_14(t *testing.T) {
	// TODO: unsupported instruction form
	checkZkcUnit(t, "zkc/unit/basic_14", DEFAULT_UNIT_CONFIG.Constraints(false).Fields(field.BLS12_377))
}

func Test_ZkcUnit_Basic_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_15", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_16", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/basic_26", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_27(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_27", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_28(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_28", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_29(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_29", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_30(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_30", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_31(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_31", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_32(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_32", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_33(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_33", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_34(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_34", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/basic_42", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_43(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_43", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_44(t *testing.T) {
	// NOTE: needs u128 word (or fast mode splitting)
	checkZkcUnit(t, "zkc/unit/basic_44", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128))
}

func Test_ZkcUnit_Basic_45(t *testing.T) {
	// TODO: subtract with borrow; needs u128 word (or fast mode splitting).
	checkZkcUnit(t, "zkc/unit/basic_45", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_46(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_46", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128))
}

func Test_ZkcUnit_Basic_47(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_47", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_48(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_48", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_49(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_49", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_50(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_50", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_51(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_51", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_52(t *testing.T) {
	// TODO: missing check cast??
	checkZkcUnit(t, "zkc/unit/basic_52", DEFAULT_UNIT_CONFIG.Constraints(false).Splitting(false))
}

func Test_ZkcUnit_Basic_53(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_53", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_54(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_54", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_55(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_55", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_56(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_56", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_57(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_57", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_58(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_58", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_59(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_59", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_60(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_60", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_61(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_61", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Basic_62(t *testing.T) {
	// TODO: gogen
	checkZkcUnit(t, "zkc/unit/basic_62", DEFAULT_UNIT_CONFIG.GoGen(false).Constraints(false))
}

func Test_ZkcUnit_Basic_63(t *testing.T) {
	// TODO: constraint generation: carry exceeds max register width (u17 vs u16)
	checkZkcUnit(t, "zkc/unit/basic_63", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_64(t *testing.T) {
	// TODO: constraint tracing still runs the legacy word machine, which
	// rejects subtraction underflow ("arithmetic underflow") instead of
	// wrapping like the bytecode interpreter.
	checkZkcUnit(t, "zkc/unit/basic_64", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_Basic_65(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_65", DEFAULT_UNIT_CONFIG)
}

// Wide (u16 register operand) instruction forms: register-register +, - and *,
// plus a wide vectored add.
func Test_ZkcUnit_Basic_66(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_66", DEFAULT_UNIT_CONFIG)
}

// Wide (u16 register operand) instruction forms: load-constant, / and %, plus
// a wide conditional branch.
func Test_ZkcUnit_Basic_67(t *testing.T) {
	// TODO: constraint tracing of u16 division fails with an arithmetic
	// underflow (pre-existing; unrelated to wide instructions).
	checkZkcUnit(t, "zkc/unit/basic_67", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// Wide (u16 register operand) instruction forms: bitwise &, |, ^ and shifts.
func Test_ZkcUnit_Basic_68(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_68", DEFAULT_UNIT_CONFIG)
}

// Wide (u16 register operand) instruction forms: general (limb-based) load
// constant.  Runs with u128 words so the u32 constants are not split before
// encoding.
func Test_ZkcUnit_Basic_69(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/basic_69", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).GoGen(false))
}

func Test_ZkcUnit_AccessOnceMemory_01(t *testing.T) {
	// Multi-line address access-once memory: a read-only ROM and a write-once
	// WOM, exercising the access bit and at_flag carry columns end-to-end.
	checkZkcUnit(t, "zkc/unit/access_once_memory_01", DEFAULT_UNIT_CONFIG)
}

// DEFERRED: access_once_memory_02 is a multi-line double-write that must be
// REJECTED. The interpreter rejects it (WriteOnce.Write), but the harness can't
// assert that yet — it skips constraint tests for .rejects, and the backends it
// runs for .rejects (word-machine exec, gogen) don't enforce write-once.
// See wom-double-write-reject-gap.md.
// func Test_ZkcUnit_AccessOnceMemory_02(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/access_once_memory_02", DEFAULT_UNIT_CONFIG)
// }

func Test_ZkcUnit_AccessOnceMemory_03(t *testing.T) {
	// Multi-line address access-once memory: a read-only ROM and a write-once
	// WOM, exercising the access bit and at_flag carry columns end-to-end.
	checkZkcUnit(t, "zkc/unit/access_once_memory_03", DEFAULT_UNIT_CONFIG)
}

// DEFERRED: access_once_memory_04 (single-line double-write) — same reason as
// _02; see wom-double-write-reject-gap.md.
// func Test_ZkcUnit_AccessOnceMemory_04(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/access_once_memory_04", DEFAULT_UNIT_CONFIG)
// }

func Test_ZkcUnit_AccessOnceMemory_05(t *testing.T) {
	// Multi-line address access-once memory: a read-only ROM and a write-once
	// WOM, exercising the access bit and at_flag carry columns end-to-end.
	checkZkcUnit(t, "zkc/unit/access_once_memory_05", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/ifelse_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_IfElse_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/ifelse_07", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/const_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/const_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Const_07(t *testing.T) {
	// NOTE: u128 registers cannot be lowered to a 64-bit word machine, nor
	// are they supported by gogen.
	checkZkcUnit(t, "zkc/unit/const_07", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).GoGen(false))
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
	checkZkcUnit(t, "zkc/unit/while_02", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_While_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/while_03", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// x = f(x) in a while loop
func Test_ZkcUnit_While_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/while_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_For_01(t *testing.T) {
	// TODO: bitwise destruct
	checkZkcUnit(t, "zkc/unit/for_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_For_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/for_02", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_For_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/for_03", DEFAULT_UNIT_CONFIG.Constraints(false))
}

func Test_ZkcUnit_For_04(t *testing.T) {
	// TODO: duplicate variable declaration #1801
	checkZkcUnit(t, "zkc/unit/for_04", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// ===================================================================
// Break Tests
// ===================================================================

func Test_ZkcUnit_Break_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/break_01", DEFAULT_UNIT_CONFIG.Constraints(false))
}

// ===================================================================
// Continue Tests
// ===================================================================

func Test_ZkcUnit_Continue_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/continue_01", DEFAULT_UNIT_CONFIG.Constraints(false))
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
	checkZkcUnit(t, "zkc/unit/bitwise_05", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_06", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_09", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/bitwise_13", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_14(t *testing.T) {
	// TODO: register splitting: duplicate register "hi" declared
	checkZkcUnit(t, "zkc/unit/bitwise_14", DEFAULT_UNIT_CONFIG.Constraints(false).Splitting(false))
}

func Test_ZkcUnit_Bitwise_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_15", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_16", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bitwise_17", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Bitwise_18(t *testing.T) {
	// TODO: wide instructions not supported
	checkZkcUnit(t, "zkc/unit/bitwise_18", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).Constraints(false).Splitting(false))
}

func Test_ZkcUnit_Bitwise_19(t *testing.T) {
	// TODO: multiply granularity
	checkZkcUnit(t, "zkc/unit/bitwise_19", DEFAULT_UNIT_CONFIG.Words(vm.WORD_UINT128).Constraints(false).Splitting(false))
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
	checkZkcUnit(t, "zkc/unit/shift_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_04", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/shift_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_09", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_10", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_11", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Shift_12(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/shift_12", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Static Initialiser Tests
// ===================================================================

func Test_ZkcUnit_Static_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Static_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_02", DEFAULT_UNIT_CONFIG)
}

// A static ROM with u32 data lines: register splitting subdivides its constant
// contents into limbs (under the smaller test fields), and the value-checked
// accepts confirm each row reassembles to its original value.  The reject reads
// out of bounds.
func Test_ZkcUnit_Static_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/static_03", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/div_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Div_02(t *testing.T) {
	// TODO: register exceeds max register width
	checkZkcUnit(t, "zkc/unit/div_02", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377).Constraints(false))
}

func Test_ZkcUnit_Div_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Div_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_04", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Div_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/div_05", DEFAULT_UNIT_CONFIG)
}

// ===================================================================
// Remainder Tests
// ===================================================================

func Test_ZkcUnit_Rem_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_01", DEFAULT_UNIT_CONFIG)
}

// Constraints + register splitting now work under BLS12_377; KOALABEAR (u16)
// still fails (register exceeds field width), hence the field restriction.
func Test_ZkcUnit_Rem_02(t *testing.T) {
	// TODO: register exceeds maximum bitwidth
	checkZkcUnit(t, "zkc/unit/rem_02", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377).Constraints(false))
}

func Test_ZkcUnit_Rem_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_03", DEFAULT_UNIT_CONFIG)
}

// Remainder by a constant power of two (optimised into a bitwise AND in fast
// mode; see OptimizeDivisions).
func Test_ZkcUnit_Rem_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/rem_04", DEFAULT_UNIT_CONFIG)
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

// This test covers call register aliasing where the return is assigned to an argument register (x = f(x)).
func Test_ZkcUnit_Call_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_03", DEFAULT_UNIT_CONFIG)
}

// This test covers call register aliasing where the return is assigned to an argument register (x = f(x, y)).
func Test_ZkcUnit_Call_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_04", DEFAULT_UNIT_CONFIG)
}

// This test performs different module call depending on the branch taken
func Test_ZkcUnit_Call_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_05", DEFAULT_UNIT_CONFIG)
}

// b = f(a) + 1
// the return value is then modified in the same vector
func Test_ZkcUnit_Call_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_06", DEFAULT_UNIT_CONFIG)
}

// x = f(x) + 1
// mixed of register collision:
// arg and return value are the same register
// return value is rewrote in the same vector
func Test_ZkcUnit_Call_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_07", DEFAULT_UNIT_CONFIG)
}

// y = f(x); x = x+1
// rewrite the arg call
func Test_ZkcUnit_Call_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_08", DEFAULT_UNIT_CONFIG)
}

// y = f(x); if() {x = x+1}
// rewrite the arg call, hidden in a skip_if condition
func Test_ZkcUnit_Call_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/call_09", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/switch_07", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_08", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Switch_09(t *testing.T) {
	// TODO: unsupported instruction form
	checkZkcUnit(t, "zkc/unit/switch_09", DEFAULT_UNIT_CONFIG.Fields(field.BLS12_377).Constraints(false))
}

func Test_ZkcUnit_Switch_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_10", DEFAULT_UNIT_CONFIG)
}

// y = switch(x) f_x(x)
func Test_ZkcUnit_Switch_11(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/switch_11", DEFAULT_UNIT_CONFIG)
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
	checkZkcUnit(t, "zkc/unit/debug_01", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Debug_02(t *testing.T) {
	// Quiet mode elides the call to the #[debug] function, whose body would
	// otherwise fail.
	checkZkcUnit(t, "zkc/unit/debug_02", DEFAULT_UNIT_CONFIG.Quiet(true))
}

func Test_ZkcUnit_Debug_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/debug_03", DEFAULT_UNIT_CONFIG)
}

func Test_ZkcUnit_Debug_04(t *testing.T) {
	// As Debug_03, but in quiet mode (all debug calls elided).
	checkZkcUnit(t, "zkc/unit/debug_03", DEFAULT_UNIT_CONFIG.Quiet(true))
}

// ===================================================================
// Inline Function Tests
// ===================================================================
//
// These mirror the Basic_XX tests which contain a function call, but mark the
// called function with the #[inline] annotation.  Hence, the called function
// is inlined at every call site (and removed), but each test must still
// behave identically to its Basic_XX counterpart.

func Test_ZkcUnit_Inline_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_08", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Inline_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_09", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Inline_10(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_10", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Inline_25(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_25", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Inline_28(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_28", util.DEFAULT_CONFIG)
}

// func Test_ZkcUnit_Inline_33(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/inline_33", util.DEFAULT_CONFIG)
// }

func Test_ZkcUnit_Inline_34(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_34", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Inline_35(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/inline_35", util.DEFAULT_CONFIG)
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
// Padding Tests
// ===================================================================
// This test contains an OLI empty module for some execution
func Test_ZkcUnit_Padding_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/padding_01", util.DEFAULT_CONFIG)
}

// This test contains an OLI empty module for some execution.
// For the empty module, "0" is an invalid input (leads to a fail)
func Test_ZkcUnit_Padding_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/padding_02", util.DEFAULT_CONFIG)
}

// This test contains an OLI module doing a memory read
func Test_ZkcUnit_Padding_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/padding_03", util.DEFAULT_CONFIG)
}

// This test contains an OLI module doing a memory write
func Test_ZkcUnit_Padding_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/padding_04", util.DEFAULT_CONFIG)
}

// This test contains an OLI empty module doing a call in case of execution
func Test_ZkcUnit_Padding_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/padding_05", util.DEFAULT_CONFIG)
}

// ===================================================================
// Range check Tests
// ===================================================================
// Range check a u16
func Test_ZkcUnit_RangeCheck_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/range_check_01", DEFAULT_UNIT_CONFIG)
}

// Range check a u64
func Test_ZkcUnit_RangeCheck_02(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/range_check_02", DEFAULT_UNIT_CONFIG)
}

// Range check a u17
func Test_ZkcUnit_RangeCheck_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/range_check_03", DEFAULT_UNIT_CONFIG)
}

// Range check a u31
func Test_ZkcUnit_RangeCheck_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/range_check_04", DEFAULT_UNIT_CONFIG)
}

// Range check a u5
func Test_ZkcUnit_RangeCheck_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/range_check_05", DEFAULT_UNIT_CONFIG)
}

// Big Tests
// ===================================================================

func Test_ZkcUnit_BigNum_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_01", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_02(t *testing.T) {
	// TODO: constraint tracing still runs the legacy word machine, which
	// rejects subtraction underflow ("arithmetic underflow") instead of
	// wrapping like the bytecode interpreter.
	checkZkcUnit(t, "zkc/unit/bignum_02", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).Constraints(false))
}

func Test_ZkcUnit_BigNum_03(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_03", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_04(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_04", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).GoGen(false))
}

func Test_ZkcUnit_BigNum_05(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_05", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).GoGen(false))
}

func Test_ZkcUnit_BigNum_06(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_06", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_07(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_07", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_08(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_08", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_09(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_09", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

// func Test_ZkcUnit_BigNum_10(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/bignum_10", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).Constraints(false))
// }

// func Test_ZkcUnit_BigNum_11(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/bignum_11", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
// }

// func Test_ZkcUnit_BigNum_12(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/bignum_12", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
// }

// func Test_ZkcUnit_BigNum_13(t *testing.T) {
// 	checkZkcUnit(t, "zkc/unit/bignum_13", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
// }

func Test_ZkcUnit_BigNum_14(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_14", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_15(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_15", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).Constraints(false))
}

func Test_ZkcUnit_BigNum_16(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_16", DEFAULT_UNIT_CONFIG.FastModeSplitting(true).Constraints(false))
}

func Test_ZkcUnit_BigNum_17(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_17", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

func Test_ZkcUnit_BigNum_18(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/bignum_18", DEFAULT_UNIT_CONFIG.FastModeSplitting(true))
}

// ===================================================================
// Test Helpers
// ===================================================================

var STATIC_DEPTHS = []uint{256, 1 << 12}

// ZKC_PADDING_STRATEGIES enumerates the padding strategies that every ZkC unit
// test is exercised against (see checkZkcUnit).
var ZKC_PADDING_STRATEGIES = []struct {
	name     string
	strategy ir.PaddingStrategy
}{
	{"single-row-padding", ir.NaryRowPadding(1)},
	{"double-row-padding", ir.NaryRowPadding(2)},
	{"next-power-of-two-padding", ir.NextPowerOfTwoPadding},
}

// checkZkcUnit runs test for different combinations of:
// - STATIC_DEPTHS
// - padding strategy
func checkZkcUnit(t *testing.T, test string, config util.Config) {
	// The generated-Go ("native") executor is cross-checked on every unit test:
	// programs it cannot yet handle (wide registers/constants/moduli) are
	// logged-and-skipped by the harness, never failed.
	//
	// Run with defined config
	t.Run("Defined config", func(t *testing.T) {
		util.CheckValid(t, test, "zkc", config)
	})

	// Run with different static depths
	for _, depth := range STATIC_DEPTHS {
		t.Run(fmt.Sprintf("depth=%d", depth), func(t *testing.T) {
			util.CheckValid(t, test, "zkc", config.MaxStaticDepth(depth))
		})
	}

	// Run with different padding strategies
	for _, s := range ZKC_PADDING_STRATEGIES {
		t.Run(s.name, func(t *testing.T) {
			util.CheckValid(t, test, "zkc", config.Padding(s.strategy))
		})
	}
}
