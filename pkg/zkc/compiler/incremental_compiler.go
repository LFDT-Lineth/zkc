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
	"maps"
	"path/filepath"
	"slices"

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
// The supplied files may make up several independent programs.  Every file
// declaring a "main" function is the entry point of its own program, which
// consists of that file plus every file it (transitively) includes.  Programs
// are compiled separately, so declarations in one program never clash with
// those in another.
//
// The compiler is not safe for concurrent use; callers are responsible for
// serialising access (e.g. through a single document-update goroutine).
type IncrementalCompiler struct {
	field field.Config
	// maxStaticHeight bounds the number of rows any declared static table may
	// occupy (see validate.StaticTableHeight).
	maxStaticHeight uint
	// files holds every source file known to the compiler, keyed by filename.
	// This map is the sole source of truth: include directives are resolved
	// against these files only, never against the filesystem, so any file that
	// is not present here is treated as if it does not exist.
	files map[string]sourceFile
	// programs holds one compiled program per entry point, keyed by the
	// filename of the entry point.
	programs map[string]*program
	// entryOf maps each file to the entry point of the program used to answer
	// queries about it (see ProgramFor).  Files reachable from no entry point
	// are absent.
	entryOf map[string]string
}

// sourceFile is a source file together with the result of parsing it.  Files
// are parsed once, when they are supplied to Apply, and the result is reused by
// every compilation until the file changes again.
type sourceFile struct {
	contents string
	parsed   parser.UnlinkedSourceFile
	// errors arising from parsing this file.
	errors []source.SyntaxError
	// entryPoint indicates this file declares a "main" function.
	entryPoint bool
}

// program is the most recent compilation of a single entry point.
type program struct {
	// files reachable from the entry point (including itself), sorted.
	files   []string
	ast     ast.Program
	srcmaps source.Maps[any]
	// errors arising from resolving includes, linking and validating this
	// program (parse errors are held by each sourceFile instead).
	errors []source.SyntaxError
}

// Source returns the current contents of the file with the given filename
// from the in-memory store.  The second return value is false when no such
// file is known to the compiler.
func (p *IncrementalCompiler) Source(filename string) (string, bool) {
	f, ok := p.files[filename]
	return f.contents, ok
}

// ProgramFor returns the AST and source-span map of the program containing the
// given file, as produced by the most recent call to Apply.  A file included by
// several programs (e.g. a library) is answered from the first such program in
// filename order of entry points.  A file reachable from no entry point yields
// an empty program.
func (p *IncrementalCompiler) ProgramFor(filename string) (ast.Program, source.Maps[any]) {
	if entry, ok := p.entryOf[filename]; ok {
		return p.programs[entry].ast, p.programs[entry].srcmaps
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
		files:           make(map[string]sourceFile),
		programs:        make(map[string]*program),
		entryOf:         make(map[string]string),
	}
}

// Apply a given set of updates to the internal state of this compiler, and
// return all errors for the files it holds.
func (p *IncrementalCompiler) Apply(updates ...FileUpdate) []source.SyntaxError {
	changed := make(map[string]bool)
	// Parse only the files that changed; all other files keep their parse.
	for _, u := range updates {
		// Skip updates which change nothing (e.g. opening a file whose contents
		// were already discovered on disk).
		if f, ok := p.files[u.filename]; ok && !u.removed && f.contents == u.contents {
			continue
		}
		//
		changed[u.filename] = true
		//
		if u.removed {
			delete(p.files, u.filename)
		} else {
			p.files[u.filename] = parseSourceFile(u.filename, u.contents)
		}
	}
	//
	var (
		filenames = slices.Sorted(maps.Keys(p.files))
		canonical = make(map[string]string)
		programs  = make(map[string]*program)
	)
	//
	for _, f := range filenames {
		canonical[canonicalPath(f)] = f
	}
	// Recompute which files each entry point reaches.  This is cheap (it walks
	// cached parses only) and must be done on every update, since adding or
	// changing one file can alter the set of files another program includes.
	for _, entry := range filenames {
		if !p.files[entry].entryPoint {
			continue
		}
		//
		files, includeErrs := p.reachable(entry, canonical)
		// Reuse the previous compilation unless one of its files changed, or it
		// now consists of different files.
		if prev, ok := p.programs[entry]; ok && slices.Equal(prev.files, files) &&
			!slices.ContainsFunc(files, func(f string) bool { return changed[f] }) {
			programs[entry] = prev
		} else {
			programs[entry] = p.compile(files, includeErrs)
		}
	}
	// Programs whose entry point disappeared are dropped by replacing the map.
	p.programs = programs
	//
	return p.index(filenames)
}

// index rebuilds the file-to-program mapping used by ProgramFor, and collects
// the errors of every file and program.  A file shared by several programs is
// compiled once per program, so the same error can be reported by each of them;
// such duplicates are reported only once.
func (p *IncrementalCompiler) index(filenames []string) []source.SyntaxError {
	var (
		errors []source.SyntaxError
		seen   = make(map[syntaxErrorKey]bool)
	)
	//
	p.entryOf = make(map[string]string)
	// Parse errors are reported for every file, even those in no program.
	for _, f := range filenames {
		errors = append(errors, p.files[f].errors...)
	}
	// Visit entry points in filename order, so the first program containing a
	// given file is deterministic.
	for _, entry := range slices.Sorted(maps.Keys(p.programs)) {
		prog := p.programs[entry]
		//
		for _, f := range prog.files {
			if _, ok := p.entryOf[f]; !ok {
				p.entryOf[f] = entry
			}
		}
		//
		for _, e := range prog.errors {
			if key := keyOf(e); !seen[key] {
				seen[key] = true

				errors = append(errors, e)
			}
		}
	}
	//
	return errors
}

// reachable returns the given entry point together with every file it
// transitively includes, in sorted order.  Include patterns are globs relative
// to the including file (as for the batch compiler, see
// scanForFurtherSourceFiles) but are matched against the known files only,
// given here by their canonical path.  An include matching no known file is
// reported as an error.
func (p *IncrementalCompiler) reachable(entry string, canonical map[string]string,
) ([]string, []source.SyntaxError) {
	var (
		visited = map[string]bool{entry: true}
		stack   = []string{entry}
		errors  []source.SyntaxError
	)
	// Depth-first walk over include declarations; the visited set ensures each
	// file is considered once, even when includes form a cycle.
	for len(stack) > 0 {
		var (
			f      = stack[len(stack)-1]
			parsed = p.files[f].parsed
			dir    = filepath.Dir(f)
		)
		//
		stack = stack[:len(stack)-1]
		//
		for _, d := range parsed.Declarations {
			inc, ok := d.(*decl.Include[symbol.Unresolved])
			if !ok {
				continue
			}
			//
			var (
				pattern = canonicalPath(filepath.Join(dir, inc.Pattern()))
				msg     = "failed to match anything"
			)
			//
			for path, target := range canonical {
				ok, err := filepath.Match(pattern, path)
				if err != nil {
					// Malformed pattern: report it rather than "no match".
					msg = err.Error()
					break
				} else if ok {
					msg = ""

					if !visited[target] {
						visited[target] = true
						stack = append(stack, target)
					}
				}
			}
			//
			if msg != "" {
				errors = append(errors, *parsed.SourceMap.SyntaxError(inc, msg))
			}
		}
	}
	//
	return slices.Sorted(maps.Keys(visited)), errors
}

// compile links and validates a given set of (already parsed) files as a single
// program.  Errors arising from resolving includes are supplied by the caller.
func (p *IncrementalCompiler) compile(files []string, errors []source.SyntaxError) *program {
	var (
		items     []parser.UnlinkedSourceFile
		hasErrors = len(errors) != 0
	)
	//
	for _, f := range files {
		sf := p.files[f]
		hasErrors = hasErrors || len(sf.errors) != 0
		//
		if len(sf.parsed.Declarations) > 0 {
			items = append(items, withOwnSourceMap(sf.parsed))
		}
	}
	// Link assembly and resolve external accesses.
	linked, srcmaps, linkErrs := Link(items...)
	errors = append(errors, linkErrs...)
	hasErrors = hasErrors || len(linkErrs) != 0
	// Capture variable declarations before flattening discards them (they are
	// needed to anchor unused-variable errors on the original declaration).
	decls := validate.CollectVariableDeclarations(linked)
	// Flatten block-level constructs (if/else, while, for) into flat if-goto form.
	lower.Flatten(linked, srcmaps)
	// Well-formedness checks (assuming unlimited field width).  Any parse or
	// link errors accumulated above mean the program is not well-formed, which
	// some downstream checks rely upon.
	errors = append(errors, validateProgram(linked, p.field, srcmaps, hasErrors, decls, p.maxStaticHeight)...)
	//
	return &program{files, linked, srcmaps, errors}
}

// parseSourceFile parses a given file, and determines whether it is an entry
// point.
func parseSourceFile(filename, contents string) sourceFile {
	parsed, errors := parser.Parse(source.NewSourceFile(filename, []byte(contents)))
	entryPoint := false
	//
	for _, d := range parsed.Declarations {
		// TODO: https://github.com/LFDT-Lineth/zkc/issues/1869 parametrize "main" name
		if fn, ok := d.(*decl.UnresolvedFunction); ok && fn.Name() == "main" {
			entryPoint = true
		}
	}
	//
	return sourceFile{contents, parsed, errors, entryPoint}
}

// withOwnSourceMap returns a copy of a parsed file with its own source map.
// Linking and flattening add entries to the source maps they are given, so
// linking cached parses directly would grow their maps on every recompilation.
func withOwnSourceMap(f parser.UnlinkedSourceFile) parser.UnlinkedSourceFile {
	srcmap := source.NewSourceMap[any](f.SourceMap.Source())
	source.JoinMaps(srcmap, &f.SourceMap, func(node any) any { return node })
	f.SourceMap = *srcmap
	//
	return f
}

// syntaxErrorKey identifies a syntax error independently of which program
// reported it (errors cannot be compared directly, since each program holds its
// own copy of the source file they refer to).
type syntaxErrorKey struct {
	filename   string
	start, end int
	message    string
}

func keyOf(e source.SyntaxError) syntaxErrorKey {
	span := e.Span()
	return syntaxErrorKey{e.SourceFile().Filename(), span.Start(), span.End(), e.Message()}
}
