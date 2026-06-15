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
package gogen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// opcodeCoverage records, for every opcode in the instruction set, how the
// generator handles it.  TestOpcodeCoverage walks the upstream opcode
// declarations and fails when one is missing here — so adding an opcode
// upstream breaks this test, not production.
var opcodeCoverage = map[string]string{
	// Base instructions (gen.go emitInstruction).
	"CALL":         "emitCall",
	"FAIL":         "panic(failure)",
	"JUMP":         "goto",
	"MEMORY_READ":  "emitMemRead",
	"MEMORY_WRITE": "emitMemWrite",
	"SKIP_IF":      "condExpr + goto",
	"SKIP":         "goto",
	"RETURN":       "returnOk",
	"DEBUG":        "no-op (diagnostics only)",
	// Word instructions.
	"INT_ADD":      "emitArith (WordTypeA)",
	"INT_SUB":      "emitArith (WordTypeA)",
	"INT_MUL":      "emitArith (WordTypeA)",
	"BIT_CONCAT":   "emitArith (WordTypeA)",
	"INT_DIV":      "emitTypeB (WordTypeB)",
	"INT_REM":      "emitTypeB (WordTypeB)",
	"BIT_AND":      "emitTypeB (WordTypeB)",
	"BIT_OR":       "emitTypeB (WordTypeB)",
	"BIT_XOR":      "emitTypeB (WordTypeB)",
	"BIT_NOT":      "emitTypeB (WordTypeB)",
	"BIT_SHL":      "emitTypeB (WordTypeB)",
	"BIT_SHR":      "emitTypeB (WordTypeB)",
	"INT_ADDMOD_P": "emitFieldOp (WordTypeF)",
	"INT_SUBMOD_P": "emitFieldOp (WordTypeF)",
	"INT_MULMOD_P": "emitFieldOp (WordTypeF)",
	// Hint instructions.
	"HINT_DIVISION": "emitHint (FieldHint)",
	// Field instructions cannot appear in a word machine; the emitter's
	// default arm rejects them with a clean error.
	"FIELD_ASSIGN": "rejected (field-machine only)",
}

// conditionCoverage mirrors opcodeCoverage for SKIP_IF conditions
// (condExpr in emit_control.go).
var conditionCoverage = map[string]string{
	"EQ":   "==",
	"NEQ":  "!=",
	"LT":   "<",
	"GT":   ">",
	"LTEQ": "<=",
	"GTEQ": ">=",
}

// TestOpcodeCoverage asserts every opcode and condition declared upstream is
// classified above.  It parses the declarations rather than relying on a
// COUNT sentinel (the enum has none), so it tracks upstream additions.
func TestOpcodeCoverage(t *testing.T) {
	checks := []struct {
		file, typeName string
		coverage       map[string]string
	}{
		{"../../instruction/opcode/opcode.go", "OpCode", opcodeCoverage},
		{"../../instruction/opcode/condition.go", "Condition", conditionCoverage},
	}

	for _, c := range checks {
		declared := declaredConsts(t, c.file, c.typeName)
		if len(declared) == 0 {
			t.Fatalf("no %s constants found in %s — moved upstream?", c.typeName, c.file)
		}

		for _, name := range declared {
			if _, ok := c.coverage[name]; !ok {
				t.Errorf("%s %s is not classified by the generator's coverage table — "+
					"decide how gogen handles it and record it here", c.typeName, name)
			}
		}

		for name := range c.coverage {
			if !slices.Contains(declared, name) {
				t.Errorf("%s %s no longer exists upstream — remove it from the coverage table", c.typeName, name)
			}
		}
	}
}

// declaredConsts parses a Go file and returns the names of constants declared
// with (or inheriting, within a const block) the given type.
func declaredConsts(t *testing.T, path, typeName string) []string {
	t.Helper()

	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	var names []string

	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}

		inBlock := false

		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			// A const either names its type, or (iota continuation) inherits
			// the block's.
			if id, ok := vs.Type.(*ast.Ident); ok {
				inBlock = id.Name == typeName
			}

			if inBlock {
				for _, n := range vs.Names {
					names = append(names, n.Name)
				}
			}
		}
	}

	return names
}

// generateSkippable enumerates the only error shapes GenerateGo may legitimately
// return on a compilable program: everything else is a coverage bug.
var generateSkippable = []*regexp.Regexp{
	regexp.MustCompile(`wider than 128 bits`),
	regexp.MustCompile(`wider than 64 bits`),
	regexp.MustCompile(`native (function|register)`),
	regexp.MustCompile(`no 'main' function`),
}

// TestGenerateCorpus sweeps every checked-in fixture: whenever a .zkc file
// compiles standalone to a word machine, GenerateGo must either succeed or
// fail with one of the enumerated unsupported-feature errors above.
func TestGenerateCorpus(t *testing.T) {
	dirs := []string{
		"../../../../../testdata/zkc/unit",
		"../../../../../testdata/zkc/bench",
		"../../../../../testdata/zkc/mixed",
		"../../../../../testdata/zkc/lib",
	}

	var generated, skipped int

	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.zkc"))
		if err != nil {
			t.Fatal(err)
		}

		for _, file := range files {
			for _, shape := range shapes {
				t.Run(filepath.Base(file)+"/"+shape.name, func(t *testing.T) {
					data, err := os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
					// Fixtures that do not compile standalone (libraries,
					// include fixtures) are out of generation's scope.
					sf := source.NewSourceFile(file, data)

					program, _, errs := compiler.Compile(field.KOALABEAR_16, *sf)
					if len(errs) > 0 {
						t.Skipf("does not compile standalone: %v", errs[0])
					}

					cfg := codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).
						LowerNatives(shape.lowered).Vectorize(true).Quiet(true)

					wm, errs := program.Compile(cfg)
					if len(errs) > 0 {
						t.Skipf("codegen fails: %v", errs[0])
					}

					if _, err := vm.GenerateGo(wm, vm.GoGenConfig{}); err != nil {
						for _, re := range generateSkippable {
							if re.MatchString(err.Error()) {
								skipped++

								t.Skipf("enumerated unsupported feature: %v", err)
							}
						}

						t.Errorf("unenumerated generation failure: %v", err)
					} else {
						generated++
					}
				})
			}
		}
	}

	t.Logf("corpus: %d generated, %d enumerated skips", generated, skipped)
}

// TestGeneratedHeader pins the provenance line: a Source config renders as a
// header comment go:generate workflows can compare against.
func TestGeneratedHeader(t *testing.T) {
	src, err := vm.GenerateGo(compileUint(t, tutorialSrc, false),
		vm.GoGenConfig{Package: "p", Source: "tutorial.zkc sha256:ab12"})
	if err != nil {
		t.Fatal(err)
	}

	want := "// Code generated by zkc gogen. DO NOT EDIT.\n// Source: tutorial.zkc sha256:ab12\npackage p\n"
	if !strings.HasPrefix(src, want) {
		t.Errorf("header mismatch:\n%s", src[:min(len(src), 200)])
	}
}
