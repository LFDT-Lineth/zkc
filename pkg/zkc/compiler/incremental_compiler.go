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
	"path/filepath"
	"sort"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/lower"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/validate"
)

// FileUpdate indicates that some change has been made to a given file
// (including, for example, that the file was removed altogether).
type FileUpdate struct {
	// indicates whether file is removed or not
	removed bool
	// path of file in question
	filename string
	// contents of update
	contents string
}

// ChangedFile constructs a FileUpdate describing a file that was added or
// whose contents have changed.
func ChangedFile(filename, contents string) FileUpdate {
	return FileUpdate{filename: filename, contents: contents}
}

// RemovedFile constructs a FileUpdate describing a file that has been
// deleted from the in-memory store.
func RemovedFile(filename string) FileUpdate {
	return FileUpdate{removed: true, filename: filename}
}

// IncrementalCompiler maintains an in-memory view of a set of source files and
// recompiles them on demand as updates arrive.  It is intended to back tools
// (such as the language server) which need to track the current state of an
// edited project without ever reading from disk: every file the compiler
// considers must first be supplied via Apply.
//
// The supplied files may make up several independent programs (e.g. a
// repository with many entry points).  Rather than compiling every file as a
// single program, the compiler follows include directives between the files
// it holds and compiles each entry point (a file which no other file includes)
// together with the files it transitively includes.
//
// The compiler is not safe for concurrent use; callers are responsible for
// serialising access (e.g. through a single document-update goroutine).
type IncrementalCompiler struct {
	field field.Config
	// maxStaticHeight bounds the number of rows any declared static table may
	// occupy (see validate.StaticTableHeight).
	maxStaticHeight uint
	// files holds the current contents of every source file known to the
	// compiler, keyed by filename.  This map is the sole source of truth:
	// include directives are resolved against these files only, never against
	// the filesystem, so any file that is not present here is treated as if it
	// does not exist.
	files map[string]string
	// units holds the independently compiled programs produced by the most
	// recent call to Apply.  It is replaced wholesale on each invocation
	// rather than patched in-place.
	units []compilationUnit
	// fileUnit maps each filename to the index (into units) of the program
	// used to answer queries about that file.
	fileUnit map[string]int
}

// compilationUnit is a single program: an entry point together with every file
// it transitively includes.
type compilationUnit struct {
	// program is the AST of this unit.
	program ast.Program
	// srcmaps maps AST nodes of this unit back to the source spans they
	// originated from.
	srcmaps source.Maps[any]
}

// Source returns the current contents of the file with the given filename
// from the in-memory store.  The second return value is false when no such
// file is known to the compiler.
func (p *IncrementalCompiler) Source(filename string) (string, bool) {
	contents, ok := p.files[filename]
	return contents, ok
}

// ProgramFor returns the AST and source-span map of the program which contains
// the given file, as produced by the most recent call to Apply.  When a file
// is shared between several programs (e.g. a library), the first program (in
// filename order of its entry point) is returned.  For a file which is not
// part of any program, an empty program is returned.
func (p *IncrementalCompiler) ProgramFor(filename string) (ast.Program, source.Maps[any]) {
	if i, ok := p.fileUnit[filename]; ok {
		return p.units[i].program, p.units[i].srcmaps
	}
	//
	return ast.Program{}, source.Maps[any]{}
}

// NewIncrementalCompiler constructs an IncrementalCompiler with an empty
// in-memory file store and no compiled program.  Source files must be
// introduced through Apply before any meaningful compilation can occur;
// calling Apply with no updates on a fresh compiler will simply produce an
// empty program.
func NewIncrementalCompiler() *IncrementalCompiler {
	return &IncrementalCompiler{
		field:           field.KOALABEAR_16,
		maxStaticHeight: codegen.DEFAULT_MAX_STATIC_HEIGHT,
		files:           make(map[string]string),
		fileUnit:        make(map[string]int),
	}
}

// Apply a given set of updates to the internal state of this compiler.
func (p *IncrementalCompiler) Apply(updates ...FileUpdate) []source.SyntaxError {
	// Apply updates to the in-memory store.
	for _, u := range updates {
		if u.removed {
			delete(p.files, u.filename)
		} else {
			p.files[u.filename] = u.contents
		}
	}
	//
	var (
		filenames        = p.sortedFilenames()
		includes, errors = p.includeGraph(filenames)
		units            []compilationUnit
		fileUnit         = make(map[string]int)
		seen             = make(map[syntaxErrorKey]bool)
	)
	// Compile each entry point together with the files it includes.  A file
	// shared between several entry points is compiled once per entry point,
	// but any error it contains is reported only once.
	for _, root := range entryPoints(filenames, includes) {
		var (
			closure     = includeClosure(root, includes)
			unit, uerrs = p.compileUnit(closure)
		)
		//
		for _, f := range closure {
			if _, ok := fileUnit[f]; !ok {
				fileUnit[f] = len(units)
			}
		}

		units = append(units, unit)

		for _, e := range uerrs {
			if key := keyOf(e); !seen[key] {
				seen[key] = true

				errors = append(errors, e)
			}
		}
	}
	// Update internal program state.
	p.units = units
	p.fileUnit = fileUnit
	//
	return errors
}

// compileUnit compiles a given set of files as a single program.
func (p *IncrementalCompiler) compileUnit(filenames []string) (compilationUnit, []source.SyntaxError) {
	var (
		items  []parser.UnlinkedSourceFile
		errors []source.SyntaxError
	)
	// Parse every file of this program.  Includes are not resolved against
	// disk; the in-memory store is the sole source of truth.
	for _, filename := range filenames {
		var (
			srcfile  = source.NewSourceFile(filename, []byte(p.files[filename]))
			cs, errs = parser.Parse(srcfile)
		)
		if len(cs.Declarations) > 0 {
			items = append(items, cs)
		}

		errors = append(errors, errs...)
	}
	// Link assembly and resolve external accesses.
	program, srcmaps, linkErrs := Link(items...)
	errors = append(errors, linkErrs...)
	// Capture variable declarations before flattening discards them (they are
	// needed to anchor unused-variable errors on the original declaration).
	decls := validate.CollectVariableDeclarations(program)
	// Flatten block-level constructs (if/else, while, for) into flat if-goto form.
	lower.Flatten(program, srcmaps)
	// Well-formedness checks (assuming unlimited field width).  Any parse or
	// link errors accumulated above mean the program is not well-formed, which
	// some downstream checks rely upon.
	errors = append(errors, validateProgram(program, p.field, srcmaps, len(errors) != 0, decls, p.maxStaticHeight)...)
	//
	return compilationUnit{program, srcmaps}, errors
}

// sortedFilenames returns the names of all known files in a deterministic order.
func (p *IncrementalCompiler) sortedFilenames() []string {
	filenames := make([]string, 0, len(p.files))
	for f := range p.files {
		filenames = append(filenames, f)
	}

	sort.Strings(filenames)

	return filenames
}

// includeGraph determines, for each file, which of the known files it includes.
// Include patterns are globs relative to the including file (as for the batch
// compiler) and are matched against the in-memory store only.  An include which
// matches nothing is reported as an error.
func (p *IncrementalCompiler) includeGraph(filenames []string) (map[string][]string, []source.SyntaxError) {
	var (
		includes = make(map[string][]string)
		errors   []source.SyntaxError
		canon    = make(map[string]string)
	)
	//
	for _, f := range filenames {
		canon[canonicalPath(f)] = f
	}
	//
	for _, f := range filenames {
		cs, _ := parser.Parse(source.NewSourceFile(f, []byte(p.files[f])))
		dir := filepath.Dir(f)
		//
		for _, d := range cs.Declarations {
			inc, ok := d.(*decl.Include[symbol.Unresolved])
			if !ok {
				continue
			}
			//
			pattern := canonicalPath(filepath.Join(dir, inc.Pattern()))
			matched := false
			//
			for key, target := range canon {
				if ok, err := filepath.Match(pattern, key); err != nil {
					errors = append(errors, *cs.SourceMap.SyntaxError(inc, err.Error()))
					matched = true // already reported
					//
					break
				} else if ok {
					matched = true

					if target != f {
						includes[f] = append(includes[f], target)
					}
				}
			}
			//
			if !matched {
				errors = append(errors, *cs.SourceMap.SyntaxError(inc, "failed to match anything"))
			}
		}
	}
	//
	return includes, errors
}

// entryPoints determines the files from which programs are compiled.  These are
// the files which no other file includes.  Files which are only reachable
// through an include cycle have no such entry point, so (in filename order) the
// first unreachable file of each cycle is used instead.
func entryPoints(filenames []string, includes map[string][]string) []string {
	var (
		included = make(map[string]bool)
		reached  = make(map[string]bool)
		roots    []string
	)
	//
	for _, targets := range includes {
		for _, t := range targets {
			included[t] = true
		}
	}
	//
	for _, f := range filenames {
		if !included[f] {
			roots = append(roots, f)

			for _, r := range includeClosure(f, includes) {
				reached[r] = true
			}
		}
	}
	//
	for _, f := range filenames {
		if !reached[f] {
			roots = append(roots, f)

			for _, r := range includeClosure(f, includes) {
				reached[r] = true
			}
		}
	}
	//
	return roots
}

// includeClosure returns the given file together with every file it
// transitively includes, in filename order.
func includeClosure(root string, includes map[string][]string) []string {
	var (
		visited = map[string]bool{root: true}
		stack   = []string{root}
		closure []string
	)
	//
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		closure = append(closure, f)

		for _, t := range includes[f] {
			if !visited[t] {
				visited[t] = true
				stack = append(stack, t)
			}
		}
	}

	sort.Strings(closure)

	return closure
}

// syntaxErrorKey identifies a syntax error independently of which program
// reported it.
type syntaxErrorKey struct {
	filename   string
	start, end int
	message    string
}

func keyOf(e source.SyntaxError) syntaxErrorKey {
	span := e.Span()
	return syntaxErrorKey{e.SourceFile().Filename(), span.Start(), span.End(), e.Message()}
}
