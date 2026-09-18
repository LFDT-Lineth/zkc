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
)

// DEFAULT_UTIL_CONFIG provides a default configuration for util tests.
var DEFAULT_UTIL_CONFIG = util.DEFAULT_CONFIG

func Test_ZkcUtil_Byte(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BitRor64(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_ror64", DEFAULT_UTIL_CONFIG.Sampling(0.1))
}

func Test_ZkcUtil_BitSar(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_sar", DEFAULT_UTIL_CONFIG.Sampling(0.1))
}

func Test_ZkcUtil_BitShr(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_shr", DEFAULT_UTIL_CONFIG.Sampling(0.1))
}

func Test_ZkcUtil_BitShl(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_shl", DEFAULT_UTIL_CONFIG.Sampling(0.1))
}

func Test_ZkcUtil_ByteCounting(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte_counting", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_ByteSize(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte_size", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_FillBytes(t *testing.T) {
	// #2007: support implicit sign bit
	checkZkcUtil(t, "zkc/util/fill_bytes", DEFAULT_UTIL_CONFIG.Constraints(false))
}

func Test_ZkcUtil_FirstByte(t *testing.T) {
	checkZkcUtil(t, "zkc/util/first_byte", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_G1G2(t *testing.T) {
	checkZkcUtil(t, "zkc/util/g1g2", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Log2(t *testing.T) {
	checkZkcUtil(t, "zkc/util/log2", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Log256(t *testing.T) {
	checkZkcUtil(t, "zkc/util/log256", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Max2(t *testing.T) {
	checkZkcUtil(t, "zkc/util/max2", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Max3(t *testing.T) {
	checkZkcUtil(t, "zkc/util/max3", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Min(t *testing.T) {
	checkZkcUtil(t, "zkc/util/min", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_Padding(t *testing.T) {
	checkZkcUtil(t, "zkc/util/padding", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_SetByte(t *testing.T) {
	checkZkcUtil(t, "zkc/util/set_byte", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_SignExtend(t *testing.T) {
	checkZkcUtil(t, "zkc/util/signextend", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_SwitchEndian(t *testing.T) {
	checkZkcUtil(t, "zkc/util/switch_endian", DEFAULT_UTIL_CONFIG)
}

// ===================================================================
// BigNum Tests
// ===================================================================

func Test_ZkcUtil_BigNum_01(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_01", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_02(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_02", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_03(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_03", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_04(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_04", DEFAULT_UTIL_CONFIG.Sampling(0.01))
}

func Test_ZkcUtil_BigNum_05(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_05", DEFAULT_UTIL_CONFIG.Sampling(0.01))
}

func Test_ZkcUtil_BigNum_06(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_06", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_07(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_07", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_08(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_08", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_09(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_09", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_10(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_10", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_11(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_11", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_12(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_12", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_13(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_13", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_14(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_14", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_15(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_15", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BigNum_16(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_16", DEFAULT_UTIL_CONFIG.Sampling(0.01))
}

func Test_ZkcUtil_BigNum_17(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_17", DEFAULT_UTIL_CONFIG.Sampling(0.01))
}

func Test_ZkcUtil_BigNum_18(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bignum_18", DEFAULT_UTIL_CONFIG.Sampling(0.01))
}

// ===================================================================
// Sharding Tests
// ===================================================================

func Test_ZkcUtil_Sharding_01(t *testing.T) {
	checkZkcUtil(t, "zkc/util/sharding_01", DEFAULT_UTIL_CONFIG.Sharding("copy", 1))
}

func Test_ZkcUtil_Sharding_02(t *testing.T) {
	checkZkcUtil(t, "zkc/util/sharding_02", DEFAULT_UTIL_CONFIG.Sharding("checkNonZero", 256))
}

func Test_ZkcUtil_Sharding_03(t *testing.T) {
	checkZkcUtil(t, "zkc/util/sharding_03", DEFAULT_UTIL_CONFIG.Sharding("checkNonZero", 256))
}

func Test_ZkcUtil_Sharding_04(t *testing.T) {
	checkZkcUtil(t, "zkc/util/sharding_04", DEFAULT_UTIL_CONFIG.Sharding("checkNonZero", 256))
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcUtil(t *testing.T, test string, config util.TestConfig) {
	util.CheckValid(t, test, config)
}
