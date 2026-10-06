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
package compiler

import (
	"testing"
)

const (
	testMain = "include \"lib.zkc\"\n\nfn main() {\n    if f(0) != 0 {\n        fail\n    }\n}\n"
	testLib  = "fn f(x:u8) -> (r:u8) {\n    r = 0\n}\n"
)

// apply loads a set of in-memory files (filename -> contents) into a fresh
// incremental compiler and returns it along with the reported errors.
func applyFiles(files map[string]string) (*IncrementalCompiler, []string) {
	var (
		c       = NewIncrementalCompiler()
		updates []FileUpdate
	)

	for name, contents := range files {
		updates = append(updates, ChangedFile(name, contents))
	}

	var msgs []string
	for _, e := range c.Apply(updates...) {
		msgs = append(msgs, e.SourceFile().Filename()+": "+e.Message())
	}

	return c, msgs
}

// Independent programs commonly live side by side in one workspace (e.g. one
// directory per entry point), and each declares its own `main` and helpers.
// Compiling every file as a single program would report these as duplicates,
// even though each program is valid on its own.
func Test_Incremental_IndependentProgramsDoNotClash(t *testing.T) {
	_, errs := applyFiles(map[string]string{
		"/w/a/main.zkc": testMain, "/w/a/lib.zkc": testLib,
		"/w/b/main.zkc": testMain, "/w/b/lib.zkc": testLib,
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

// A genuine duplicate inside a single program must still be reported, and must
// not be hidden or attributed to unrelated programs in the same workspace.
func Test_Incremental_DuplicateWithinProgramIsReported(t *testing.T) {
	_, errs := applyFiles(map[string]string{
		"/w/a/main.zkc": "include \"lib.zkc\"\ninclude \"lib2.zkc\"\n\nfn main() {\n}\n",
		"/w/a/lib.zkc":  testLib,
		"/w/a/lib2.zkc": testLib,
		"/w/b/main.zkc": testMain, "/w/b/lib.zkc": testLib,
	})
	if len(errs) == 0 {
		t.Fatal("expected a duplicate declaration error")
	}

	for _, e := range errs {
		if len(e) < 5 || e[:5] != "/w/a/" {
			t.Fatalf("error should only concern program a, got %q", e)
		}
	}
}

// A library shared by several programs is compiled once per program, but an
// error inside it must be shown to the user once, not once per program.
func Test_Incremental_SharedLibraryErrorReportedOnce(t *testing.T) {
	shared := "fn f(x:u8) -> (r:u8) {\n    r = y\n}\n"
	main := "include \"../lib/shared.zkc\"\n\nfn main() {\n    if f(0) != 0 {\n        fail\n    }\n}\n"
	//
	_, errs := applyFiles(map[string]string{
		"/w/a/main.zkc": main, "/w/b/main.zkc": main, "/w/lib/shared.zkc": shared,
	})
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %v", errs)
	}
}

// An include which matches no known file is a user error, as for the batch
// compiler, and is reported against the including file.
func Test_Incremental_UnmatchedIncludeIsReported(t *testing.T) {
	_, errs := applyFiles(map[string]string{
		"/w/main.zkc": "include \"missing.zkc\"\n\nfn main() {\n}\n",
	})
	if len(errs) != 1 || errs[0] != "/w/main.zkc: failed to match anything" {
		t.Fatalf("expected an unmatched include error, got %v", errs)
	}
}

// Queries about a file must be answered using the program it belongs to: a
// library file sees the declarations of its program, whilst a file which is
// not part of any program sees none.
func Test_Incremental_ProgramFor(t *testing.T) {
	c, errs := applyFiles(map[string]string{
		"/w/a/main.zkc": testMain, "/w/a/lib.zkc": testLib,
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if p, _ := c.ProgramFor("/w/a/lib.zkc"); len(p.Components()) != 2 {
		t.Fatalf("expected library to resolve to program with 2 declarations, got %d", len(p.Components()))
	}

	if p, _ := c.ProgramFor("/w/unknown.zkc"); len(p.Components()) != 0 {
		t.Fatalf("expected empty program for unknown file, got %d declarations", len(p.Components()))
	}
}

// Files which include each other have no obvious entry point, but must still
// be compiled (once) rather than silently skipped.
func Test_Incremental_IncludeCycleStillCompiled(t *testing.T) {
	c, errs := applyFiles(map[string]string{
		"/w/a.zkc": "include \"b.zkc\"\n\nfn main() {\n    if f(0) != 0 {\n        fail\n    }\n}\n",
		"/w/b.zkc": "include \"a.zkc\"\n\n" + testLib,
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if p, _ := c.ProgramFor("/w/b.zkc"); len(p.Components()) != 2 {
		t.Fatalf("expected both declarations in one program, got %d", len(p.Components()))
	}
}
