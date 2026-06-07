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
	"os"
	"path/filepath"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/test/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
)

func Test_ZkcUnit_Cost_01(t *testing.T) {
	checkZkcUnit(t, "zkc/unit/cost_01", util.DEFAULT_CONFIG)
}

func Test_ZkcUnit_Cost_01_ReportIncludesLoweredLabels(t *testing.T) {
	path := filepath.Join(util.TestDir, "zkc/unit/cost_01.zkc")

	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	src := source.NewSourceFile(path, bytes)

	program, _, errors := compiler.Compile(field.KOALABEAR_16, *src)
	if len(errors) != 0 {
		t.Fatalf("unexpected compile errors: %v", errors)
	}

	report := codegen.NewCostReport()

	_, errors = program.Compile(codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).CostReport(report))
	if len(errors) != 0 {
		t.Fatalf("unexpected codegen errors: %v", errors)
	}

	totals := report.StaticTotals()
	for _, label := range []string{
		"load_first",
		"load_second",
		"array_copy",
		"branch",
		"leaf",
		"loop",
	} {
		if totals[label] == 0 {
			t.Fatalf("expected cost label %q to survive lowering and fixed-array expansion, got totals %v", label, totals)
		}
	}
}
