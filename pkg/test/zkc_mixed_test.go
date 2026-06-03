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

func Test_ZkcMixed_Basic_01(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_01", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_02(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_02", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_03(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_03", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_04(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_04", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_05(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_05", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_06(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_06", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_07(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_07", field.BLS12_377, field.KOALABEAR_16)
}

func Test_ZkcMixed_Basic_08(t *testing.T) {
	checkZkcMixed(t, "zkc/mixed/basic_08", field.BLS12_377, field.KOALABEAR_16)
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkZkcMixed(t *testing.T, test string, fields ...field.Config) {
	var config = util.DEFAULT_CONFIG.Fields(fields...)
	//
	util.CheckValid(t, test, "zkc", config)
}
