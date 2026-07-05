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

// DEFAULT_UTIL_CONFIG provides a default configuration for util tests.
var DEFAULT_UTIL_CONFIG = util.DEFAULT_CONFIG.
	Fields(field.KOALABEAR_16).
	Constraints(true).
	Splitting(true).
	Bytecode(true).
	GoGen(true)

func Test_ZkcUtil_Byte(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BitRor64(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_ror64", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BitSar(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_sar", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BitShr(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_shr", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_BitShl(t *testing.T) {
	checkZkcUtil(t, "zkc/util/bit_shl", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_ByteCounting(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte_counting", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_ByteSize(t *testing.T) {
	checkZkcUtil(t, "zkc/util/byte_size", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_FillBytes(t *testing.T) {
	// TODO: no multiply granularity for field bandwidth
	checkZkcUtil(t, "zkc/util/fill_bytes", DEFAULT_UTIL_CONFIG.Fields(field.BLS12_377).Constraints(false))
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

func Test_ZkcUtil_Signextend(t *testing.T) {
	checkZkcUtil(t, "zkc/util/signextend", DEFAULT_UTIL_CONFIG)
}

func Test_ZkcUtil_SwitchEndian(t *testing.T) {
	checkZkcUtil(t, "zkc/util/switch_endian", DEFAULT_UTIL_CONFIG)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcUtil(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config)
}
