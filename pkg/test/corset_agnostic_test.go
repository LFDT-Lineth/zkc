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

func Test_Agnostic_Padding_01(t *testing.T) {
	util.CheckCorset(t, false, "corset/agnostic/padding_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Vanish_01(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/vanish_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Vanish_02(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/vanish_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Lookup_01(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_01", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Lookup_02(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_02", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

//#1258
// func Test_Agnostic_Lookup_03(t *testing.T) {
// 	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_03", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
// }

func Test_Agnostic_Lookup_04(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_04", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Lookup_05(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_05", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Lookup_06(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_06", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

// #1258
func Test_Agnostic_Lookup_07(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_07", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}

func Test_Agnostic_Lookup_08(t *testing.T) {
	util.CheckCorsetNoPadding(t, false, "corset/agnostic/lookup_08", field.BLS12_377, field.KOALABEAR_16, field.GF_8209)
}
