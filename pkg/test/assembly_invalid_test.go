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

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

func Test_AsmInvalid_Basic_01(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_01")
}

func Test_AsmInvalid_Basic_02(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_02")
}

func Test_AsmInvalid_Basic_03(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_03")
}

func Test_AsmInvalid_Basic_04(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_04")
}
func Test_AsmInvalid_Basic_05(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_05")
}
func Test_AsmInvalid_Basic_06(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_06")
}
func Test_AsmInvalid_Basic_07(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/basic_07")
}

// ===================================================================
// Flow Tests
// ===================================================================

func Test_AsmInvalid_Flow_01(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/flow_01")
}

func Test_AsmInvalid_Flow_02(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/flow_02")
}

func Test_AsmInvalid_Flow_03(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/flow_03")
}
func Test_AsmInvalid_Flow_04(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/flow_04")
}

func Test_AsmInvalid_Flow_05(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/flow_05")
}

// ===================================================================
// Bitwidth Tests
// ===================================================================

func Test_AsmInvalid_Bitwidth_01(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/bitwidth_01")
}
func Test_AsmInvalid_Bitwidth_02(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/bitwidth_02")
}

// ===================================================================
// Division Tests
// ===================================================================

func Test_Invalid_Div_01(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_01")
}

func Test_Invalid_Div_02(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_02")
}

func Test_Invalid_Div_03(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_03")
}

func Test_Invalid_Div_04(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_04")
}
func Test_Invalid_Div_05(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_05")
}
func Test_Invalid_Div_06(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_06")
}
func Test_Invalid_Div_07(t *testing.T) {
	checkAsmInvalid(t, "asm/invalid/div_07")
}

// ===================================================================
// Test Helpers
// ===================================================================

func checkAsmInvalid(t *testing.T, test string) {
	util.CheckInvalid(t, test, "zkasm", ";;error", compileAssembly)
}

func compileAssembly(srcfile source.File) []source.SyntaxError {
	_, _, errors := asm.Assemble(srcfile)
	//
	return errors
}
