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
package ast

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// RawProgram encapsulates one of more functions together, such that one may call
// another, etc.  Furthermore, it provides an interface between assembly
// components and the notion of a Schema.
type RawProgram[S any] struct {
	declarations []decl.Declaration[S]
}

// Component returns the ith entity in this program.
func (p *RawProgram[S]) Component(id uint) decl.Declaration[S] {
	return p.declarations[id]
}

// Components returns all functions making up this program.
func (p *RawProgram[S]) Components() []decl.Declaration[S] {
	return p.declarations
}

// Program represents a program whose declarations contain only resolved
// external identifiers. As such, it should not be possible that any
// declarations contained within refer to unknown (or otherwise incorrect)
// external components.
type Program struct {
	RawProgram[symbol.Resolved]
	srcmaps source.Maps[any]
}

// NewProgram constructs a new program using a given level of instruction.
func NewProgram(components []decl.Resolved, srcmaps source.Maps[any]) Program {
	//
	decls := make([]decl.Resolved, len(components))
	copy(decls, components)

	return Program{RawProgram[symbol.Resolved]{decls}, srcmaps}
}

// Environment creates a fresh environment for this program
func (p *Program) Environment() Environment {
	return NewEnvironment(p.declarations...)
}

// Compile attempts to compile a given high-level program into a low-level
// machine which can be used (for example) to execute this program with some
// given inputs.
func Compile(prog Program, config codegen.Config) (vm.Program[vm.Uint], []source.SyntaxError) {
	var (
		// Construct code generator
		compiler = codegen.NewCompiler(config, prog.Environment(), prog.srcmaps)
	)
	// Compiler into vm Program
	return compiler.Compile(prog.declarations)
}
