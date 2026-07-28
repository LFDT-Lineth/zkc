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
package util

import (
	"fmt"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// GENERATION_FIELDS is the set of fields over which constraint-generation tests
// run.  It spans a wide range of register widths (GF_251: u4, GF_8209: u8,
// KOALABEAR_16: u16, BLS12_377: u160), so a fixed-width value such as a 32-bit
// timestamp splits into 8 / 4 / 2 / 1 limbs respectively — exercising the
// multi-limb machinery under every splitting regime.
var GENERATION_FIELDS = []field.Config{field.GF_251, field.GF_8209, field.KOALABEAR_16, field.BLS12_377}

// CheckConstraintGeneration verifies that a source fixture (a) compiles to a
// machine and (b) generates AIR constraints, for each field in GENERATION_FIELDS.
// Unlike CheckValid it uses no trace files: it exercises only compilation and
// constraint generation.  This is useful for a feature (such as RAM constraints)
// whose trace observer is not yet complete, so end-to-end tracing tests cannot
// yet run, but whose schema generation should nonetheless be exercised across
// fields.
func CheckConstraintGeneration(t *testing.T, test string) {
	for _, f := range GENERATION_FIELDS {
		f := f
		testfile := fmt.Sprintf("%s.zkc", test)
		//
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			//
			cfg := codegen.DEFAULT_CONFIG.
				SplitRegisters(true).
				Field(f).
				Word(DEFAULT_WORD)
			// (a) compilation into a machine (fast mode is off, so the constraint
			// path — including timestamp threading — runs).
			p := compileTestProgram(t, testfile, cfg)
			// (b) constraint generation.
			generateAirConstraints(t, p, f, cfg.GetMaxStaticDepth())
		})
	}
}

// generateAirConstraints generates the AIR schema for a program, failing the test
// (rather than crashing the test binary) if generation panics.
func generateAirConstraints(t *testing.T, p vm.Program[vm.Uint], f field.Config, maxStaticDepth uint) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("constraint generation panicked (%s): %v", f.Name, r)
		}
	}()
	//
	switch f {
	case field.GF_251:
		_ = constraints.GenerateAirConstraints[vm.Uint, gf251.Element](p, f, maxStaticDepth)
	case field.GF_8209:
		_ = constraints.GenerateAirConstraints[vm.Uint, gf8209.Element](p, f, maxStaticDepth)
	case field.KOALABEAR_16:
		_ = constraints.GenerateAirConstraints[vm.Uint, koalabear.Element](p, f, maxStaticDepth)
	case field.BLS12_377:
		_ = constraints.GenerateAirConstraints[vm.Uint, bls12_377.Element](p, f, maxStaticDepth)
	default:
		t.Errorf("unknown field configuration: %s", f.Name)
	}
}
