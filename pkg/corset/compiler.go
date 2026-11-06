// Copyright Consensys Incorporated
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
package corset

import (
	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/corset/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// SyntaxError defines the kind of errors that can be reported by this compiler.
// Syntax errors are always associated with some line in one of the original
// source files.  For simplicity, we reuse existing notion of syntax error from
// the S-Expression library.
type SyntaxError = source.SyntaxError

// CompilationConfig encapsulates various options which can affect compilation.
type CompilationConfig = compiler.Config

// CompileSourceFiles compiles one or more source files into a schema.  This
// process can fail if the source files are mal-formed, or contain syntax errors
// or other forms of error (e.g. type errors).
func CompileSourceFiles[F field.Element[F]](config CompilationConfig, srcfiles []source.File,
) (mir.Schema[F], []SyntaxError) {
	// Parse all source files.
	circuit, srcmap, errs := compiler.ParseSourceFiles(srcfiles, config)
	// Check for parsing errors
	if errs != nil {
		return mir.Schema[F]{}, errs
	}
	// Compile each module into the schema
	comp := NewCompiler(circuit, srcmap)
	//
	return comp.Compile[F](config)
}

// CompileSourceFile compiles exactly one source file into a schema.  This is
// really helper function for e.g. the testing environment.   This process can
// fail if the source file is mal-formed, or contains syntax errors or other
// forms of error (e.g. type errors).
func CompileSourceFile[F field.Element[F]](config CompilationConfig, srcfile source.File,
) (mir.Schema[F], []SyntaxError) {
	return CompileSourceFiles[F](config, []source.File{srcfile})
}

// Compiler packages up everything needed to compile a given set of module
// definitions down into an MIR schema.  Observe that the compiler may fail if
// the modules definitions are malformed in some way (e.g. fail type checking).
type Compiler struct {
	// A high-level definition of a Corset circuit.
	circuit ast.Circuit
	// Determines whether to apply sanity checks
	checks bool
	// Source maps nodes in the circuit back to the spans in their original
	// source files.  This is needed when reporting syntax errors to generate
	// highlights of the relevant source line(s) in question.
	srcmap *source.Maps[ast.Node]
}

// NewCompiler constructs a new compiler for a given set of modules.
func NewCompiler(circuit ast.Circuit, srcmaps *source.Maps[ast.Node]) *Compiler {
	return &Compiler{circuit, true, srcmaps}
}

// Compile is the top-level function for the corset compiler which actually
// compiles the given modules down into a schema.  This can fail in a variety of
// ways if the given modules are malformed in some way.  For example, if some
// expression refers to a non-existent module or column, or is not well-typed,
// etc.
func (p *Compiler) Compile[F field.Element[F]](config compiler.Config) (mir.Schema[F], []SyntaxError) {
	var (
		scope  *compiler.ModuleScope
		errors []SyntaxError
		// Used only to determine the field modulus
		element F
	)
	// Resolve variables (via nested scopes)
	scope, errors = compiler.ResolveCircuit(p.srcmap, &p.circuit)
	// Type check circuit.
	errors = append(errors, compiler.TypeCheckCircuit(p.srcmap, &p.circuit, element.Modulus())...)
	// Catch errors
	if len(errors) > 0 {
		return mir.Schema[F]{}, errors
	}
	// Convert global scope into an environment by allocating all columns.
	environment := compiler.NewGlobalEnvironment(scope)
	// Translate everything and add it to the schema.
	schema, errs := compiler.TranslateCircuit[F](environment, p.srcmap, &p.circuit, config)
	// Sanity check for errors
	if len(errs) > 0 {
		return mir.Schema[F]{}, errs
	}
	//
	return schema, errs
}
