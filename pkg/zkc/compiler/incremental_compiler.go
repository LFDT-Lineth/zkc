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
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/lower"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
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
// The compiler is not safe for concurrent use; callers are responsible for
// serialising access (e.g. through a single document-update goroutine).
type IncrementalCompiler struct {
	field field.Config
	// files holds the current contents of every source file known to the
	// compiler, keyed by filename.  This map is the sole source of truth:
	// include directives are not resolved against the filesystem, so any
	// file that is not present here is treated as if it does not exist.
	files map[string]string
	// program is the AST produced by the most recent call to Apply.  It is
	// replaced wholesale on each invocation rather than patched in-place,
	// so callers should re-read it after every update.
	program ast.Program
	// srcmaps maps AST nodes from the most recent compilation back to the
	// source spans they originated from.  Like program, it is replaced
	// wholesale on each Apply.
	srcmaps source.Maps[any]
}

// Source returns the current contents of the file with the given filename
// from the in-memory store.  The second return value is false when no such
// file is known to the compiler.
func (p *IncrementalCompiler) Source(filename string) (string, bool) {
	contents, ok := p.files[filename]
	return contents, ok
}

// Program returns the AST produced by the most recent call to Apply.
func (p *IncrementalCompiler) Program() ast.Program {
	return p.program
}

// SourceMaps returns the source-span map produced by the most recent call to
// Apply, allowing callers to translate AST nodes back to their originating
// file and span.
func (p *IncrementalCompiler) SourceMaps() source.Maps[any] {
	return p.srcmaps
}

// NewIncrementalCompiler constructs an IncrementalCompiler with an empty
// in-memory file store and no compiled program.  Source files must be
// introduced through Apply before any meaningful compilation can occur;
// calling Apply with no updates on a fresh compiler will simply produce an
// empty program.
func NewIncrementalCompiler() *IncrementalCompiler {
	return &IncrementalCompiler{
		field: field.KOALABEAR_16,
		files: make(map[string]string),
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
		items   []parser.UnlinkedSourceFile
		errors  []source.SyntaxError
		program ast.Program
		srcmaps source.Maps[any]
	)
	// Parse every file currently in the in-memory store. Includes are not
	// resolved against disk; the store is the sole source of truth.
	for filename, contents := range p.files {
		var (
			srcfile  = source.NewSourceFile(filename, []byte(contents))
			cs, errs = parser.Parse(srcfile)
		)
		if len(cs.Components) > 0 {
			items = append(items, cs)
		}

		errors = append(errors, errs...)
	}
	// Link assembly and resolve external accesses.
	var linkErrs []source.SyntaxError

	program, srcmaps, linkErrs = Link(items...)
	errors = append(errors, linkErrs...)
	// Flatten block-level constructs (if/else, while, for) into flat if-goto form.
	lower.Flatten(program, srcmaps)
	// Well-formedness checks (assuming unlimited field width).
	errors = append(errors, validateProgram(program, p.field, srcmaps)...)
	// Update internal program state.
	p.program = program
	p.srcmaps = srcmaps
	//
	return errors
}
