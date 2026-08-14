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

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/lower"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/validate"
)

// Compile takes a given set of source files and parses them and their
// dependencies into a given set of linked declarations. This includes
// performing various checks on the files, such as type checking, etc.
// Switch statements are lowered to a multiway-skip dispatch.
func Compile(field field.Config, maxStaticHeight uint, sourceFiles ...source.File,
) (ast.Program, source.Maps[any], []source.SyntaxError) {
	//
	var (
		unlinkedSourceFiles []parser.UnlinkedSourceFile
		errors              []source.SyntaxError
		program             ast.Program
		srcmaps             source.Maps[any]
		knownSourceFiles    map[string]bool = make(map[string]bool)
	)
	// Initialise accounted for source files map with all top-level files
	for _, sf := range sourceFiles {
		knownSourceFiles[canonicalPath(sf.Filename())] = true
	}
	// recursively parse all files required to compile sourceFiles:
	// the files themselves as well as the recursive closure of all
	// included files
	for len(sourceFiles) > 0 {
		var (
			sourceFile         = sourceFiles[0]
			errs               []source.SyntaxError
			furtherSourceFiles []source.File
			unlinkedSourceFile parser.UnlinkedSourceFile
		)
		//
		sourceFiles = sourceFiles[1:]
		// Parse source file; keep partial results even on error.
		unlinkedSourceFile, errs = parser.Parse(&sourceFile)
		if len(unlinkedSourceFile.Declarations) > 0 {
			unlinkedSourceFiles = append(unlinkedSourceFiles, unlinkedSourceFile)

			var inclErrs []source.SyntaxError

			furtherSourceFiles, inclErrs = scanForFurtherSourceFiles(sourceFile, unlinkedSourceFile, knownSourceFiles)
			errs = append(errs, inclErrs...)
			sourceFiles = append(sourceFiles, furtherSourceFiles...)
		}

		errors = append(errors, errs...)
	}
	// Link assembly and resolve external accesses
	program, srcmaps, linkErrs := Link(unlinkedSourceFiles...)
	//
	errors = append(errors, linkErrs...)
	// Capture variable declarations before flattening discards them (they are
	// needed to anchor unused-variable errors on the original declaration).
	decls := validate.CollectVariableDeclarations(program)
	// Flatten block-level constructs (if/else, switch, while, for) into flat if-goto form
	lower.Flatten(program, srcmaps)
	// Well-formedness checks (assuming unlimited field width).  Any parse or
	// link errors accumulated above mean the program is not well-formed, which
	// some downstream checks rely upon.
	errors = append(errors, validateProgram(program, field, srcmaps, len(errors) != 0, decls, maxStaticHeight)...)
	// Lower fixed-size arrays into flat local access registers
	if len(errors) == 0 {
		lower.FlattenFixedArrays(field, program)
	}
	// Done
	return program, srcmaps, errors
}

// canonicalPath returns an absolute, cleaned form of filename for use as a
// dedup key, so the same file reached through different relative spellings maps
// to one key.  On error (e.g. the working directory is unavailable) it falls
// back to the cleaned relative path rather than failing the compile.
func canonicalPath(filename string) string {
	if abs, err := filepath.Abs(filename); err == nil {
		return abs
	}

	return filepath.Clean(filename)
}

// scanForFurtherSourceFiles goes over the include declarations of a parsed source file and
// determines which include declarations are genuinely new vs already known / accounted for.
// It then returns the list of new source files that must be added to the compilation process.
//
// Since include declarations provide relative paths, the original sourceFile is provided
// in order to determine canonical (absolute) paths.
//
// Note: scanForFurtherSourceFiles implicitly updates 'knownSourceFiles' with every new
// source file that it adds to its furtherSourceFiles output
func scanForFurtherSourceFiles(sourceFile source.File, parsedSourceFile parser.UnlinkedSourceFile,
	knownSourceFiles map[string]bool) ([]source.File, []source.SyntaxError) {
	//
	var (
		dir                = filepath.Dir(sourceFile.Filename())
		furtherSourceFiles []source.File
		errors             []source.SyntaxError
	)
	//
	for _, d := range parsedSourceFile.Declarations {
		if inc, ok := d.(*decl.Include[symbol.Unresolved]); ok {
			var (
				pattern      = filepath.Join(dir, inc.Pattern())
				matches, err = filepath.Glob(pattern)
			)
			//
			if err != nil {
				errors = append(errors, *parsedSourceFile.SourceMap.SyntaxError(inc, err.Error()))
				continue
			} else if len(matches) == 0 {
				// failed to match anything
				errors = append(errors, *parsedSourceFile.SourceMap.SyntaxError(inc, "failed to match anything"))
				continue
			}
			//
			for _, filename := range matches {
				// Dedup on the canonical (absolute, cleaned) path: the same
				// physical file is reached through different relative spellings
				// (e.g. main includes "memory.zkc" while a library includes
				// "../../riscv/memory.zkc"), and keying on the raw path would
				// parse it twice, yielding spurious duplicate-declaration errors.
				key := canonicalPath(filename)
				// Check filename not already parsed
				if seen, ok := knownSourceFiles[key]; seen && ok {
					// file already loaded, therefore ignore.
				} else if fs, err := source.ReadFiles(filename); err == nil {
					furtherSourceFiles = append(furtherSourceFiles, fs...)
				} else {
					errors = append(errors, *parsedSourceFile.SourceMap.SyntaxError(inc, err.Error()))
				}
				// Record that we've seen this file now.
				knownSourceFiles[key] = true
			}
		}
	}
	//
	return furtherSourceFiles, errors
}

// Validate checks that a given program is well-formed.  For example, an
// assignment "x,y = z" must be balanced (i.e. number of bits on lhs must match
// number on rhs).  Likewise, variables cannot be used before they are defined,
// and all control-flow paths must reach a "return" instruction, etc. Finally,
// we cannot assign to an input register under the current calling convention.
func validateProgram(program ast.Program, field field.Config, srcmaps source.Maps[any],
	hasPriorErrors bool, decls validate.VariableDeclarations, maxStaticHeight uint) []source.SyntaxError {
	var errors []source.SyntaxError
	// Check for cyclic definitions (constants and type aliases); if cycle is
	// detected, skip remaining phases (for now).
	if errors = validate.CycleDetection(program, srcmaps); len(errors) > 0 {
		return errors
	}
	// Attempt to type the program.
	errors = append(errors, validate.Typing(program, field, srcmaps)...)
	// Attempt to check that every variable in the program is used.  This is only
	// meaningful for a well-formed program, as a variable may otherwise appear
	// unused simply because an upstream error (e.g. an unresolved symbol in its
	// type) prevented it from being wired up.
	if !hasPriorErrors && len(errors) == 0 {
		errors = append(errors, validate.VariableUses(program, field, srcmaps, decls)...)
	}
	// Check the entry point (if any) is well-formed.
	errors = append(errors, validate.EntryPoint(program, srcmaps)...)
	// Perform final validation
	errors = append(errors, validate.ControlFlow(program, srcmaps)...)
	// Check #[debug] functions are safe to elide
	errors = append(errors, validate.DebugFunctions(program, srcmaps)...)
	// Check #[inline] functions can actually be inlined
	errors = append(errors, validate.InlineFunctions(program, srcmaps)...)
	// Check no static tables have more rows than max-static-height
	errors = append(errors, validate.StaticTableHeight(program, srcmaps, maxStaticHeight)...)
	//
	return errors
}
