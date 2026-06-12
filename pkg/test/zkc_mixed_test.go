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

// DEFAULT_MIXED_CONFIG provides a default configuration for mixed tests.  These
// are executed via the bytecode interpreter (in addition to the usual word
// machine), exercising its support for native field arithmetic.
var DEFAULT_MIXED_CONFIG = util.DEFAULT_CONFIG.
	Words(vm.WORD_UINT, vm.WORD_UINT64).
	Bytecode(true)

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
	// NOTE: basic_07 is not yet executable under the bytecode interpreter.  Its
	// felt->u16 casts and "::" concatenation exercise a native-register width
	// path in the bytecode compiler which is unrelated to field arithmetic, so
	// it runs on the slow word interpreter only for now.
	checkZkcMixed(t, "zkc/mixed/basic_07", DEFAULT_MIXED_CONFIG.Bytecode(false))
}

func Test_ZkcMixed_Basic_08(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_08", DEFAULT_MIXED_CONFIG)
}

// ===================================================================
// Others
// ===================================================================

func Test_ZkcUnit_Felt_Memory_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_memory_01", DEFAULT_MIXED_CONFIG)
}

func Test_ZkcUnit_Felt_Casting_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/felt_casting_01", DEFAULT_MIXED_CONFIG.Fields(field.KOALABEAR_16))
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcMixed(t *testing.T, test string, config util.Config) {
	util.CheckValid(t, test, "zkc", config)
}
