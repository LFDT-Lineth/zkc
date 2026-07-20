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

// DEFAULT_MIXED_CONFIG provides a default configuration for mixed tests.  These
// exercise native field arithmetic: they run on the bytecode interpreter and,
// since the tested field (KoalaBear) has a modulus ≤ 64 bits, on the
// generated-Go executor too (gogen represents a field element as a single
// uint64 — see pkg/zkc/vm/internal/gogen).
var DEFAULT_MIXED_CONFIG = util.DEFAULT_CONFIG.
	GoGen(true)

// ===================================================================
// Basic Tests
// ===================================================================

func Test_ZkcMixed_Basic_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_01", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_02(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_02", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_03(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_03", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_04(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_04", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_05(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_05", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_06(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_06", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_07(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_07", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_08(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_08", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_09(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_09", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_10(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_10", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Basic_11(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_11", DEFAULT_MIXED_CONFIG)
}

// ===================================================================
// Others
// ===================================================================

func Test_ZkcMixed_Felt_Memory_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_memory_01", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_01", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_02(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_02", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_03(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_03", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_04(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_04", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_05(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_05", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcMixed_Felt_Casting_06(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_06", DEFAULT_MIXED_CONFIG)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcMixed(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config)
}
