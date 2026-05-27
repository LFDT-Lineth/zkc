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
package zkc

import (
	"os"
	"path"

	"github.com/consensys/go-corset/pkg/ir/air"
	"github.com/consensys/go-corset/pkg/ir/mir"
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/compiler/codegen"
	"github.com/consensys/go-corset/pkg/zkc/constraints"
	"github.com/consensys/go-corset/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
)

// BuildArtifacts captures the set of outputs generated from compiling a given
// ZkC program (e.g. AIR constraints).  Whilst the AIR artifact might be
// considered the primary goal of compilation, the other artifacts are needed to
// support other features.  For example, the FIR artifact is needed for trace
// expansion, whilst the AST artifact allows the AST to be printed for debugging
// purposes.
type BuildArtifacts[F field.Element[F]] struct {
	// Abstract Syntax Tree
	ast util.Option[ast.Program]
	// Word Machine
	wir util.Option[vm.WordMachine[vm.Uint]]
	// Field Machine
	fir util.Option[vm.FieldMachine[F]]
	// MIR Constraints
	mir util.Option[mir.Schema[F]]
	// AIR Constraints
	air util.Option[air.Schema[F]]
}

// BuildConfig packages up all the requirements for building the set of target
// artifacts.
type BuildConfig[F field.Element[F]] struct {
	// code configuration includes various things which can be turned off / on.
	config codegen.Config
	// field configuration
	field field.Config
	// metadata to include in binary output file
	metadata []byte
	// flags signal which layers to generate artifacts for.
	ast, wir, fir, mir, air bool
}

// HasTarget checks whether or not at least one build target is specified.
func (p BuildConfig[F]) HasTarget() bool {
	return p.ast || p.wir || p.fir || p.mir || p.air
}

// Dependencies produces a build configuration with all transitive dependencies
// made explicit.
func (p BuildConfig[F]) Dependencies() BuildConfig[F] {
	p.mir = p.mir || p.air
	p.fir = p.fir || p.mir
	p.wir = p.wir || p.fir
	p.ast = p.ast || p.wir
	//
	return p
}

// Build applies a build configuration with a given set of source files.
func (p *BuildConfig[F]) Build(args ...string) BuildArtifacts[F] {
	var (
		errs []source.SyntaxError
		// determine transitive dependencies
		deps = p.Dependencies()
		//
		artifacts BuildArtifacts[F]
		// Abstract Syntax Tree
		ast ast.Program
		// Word Machine
		wir *vm.WordMachine[vm.Uint]
		// Field Machine
		fir *vm.FieldMachine[F]
		// MIR Constraints
		mir mir.Schema[F]
		// AIR Constraints
		air air.Schema[F]
	)
	// Check whether prebuilt binary supplied on command-line.
	if len(args) > 0 && path.Ext(args[0]) == ".bin" {
		// Sanity check exactly one prebuilt binary provide.
		if len(args) != 1 {
			log.Error("require exactly one prebuilt binary")
			os.Exit(6)
		} else if p.ast {
			log.Error("cannot extract AST from prebuilt binary")
			os.Exit(7)
		}
		// Single (binary) file supplied
		wm := ReadBinaryFile[F](args[0]).WordMachine()
		// Assign over
		wir = &wm
	} else {
		// Compile source files, or print errors
		ast = CompileSourceFiles(p.field, args...)
		// Word-level Intermediate Representation
		if deps.wir {
			// Compile the AST into the top-level word machine
			wir, errs = ast.Compile(p.config)
			//
			if len(errs) > 0 {
				for _, err := range errs {
					printSyntaxError(&err)
				}
				//
				os.Exit(4)
			}
		}
	}
	// Field-level Intermediate Representation
	if deps.fir {
		fir = vm.LowerWordMachine[vm.Uint, F](p.field, wir)
	}
	// Mid-level Intermediate Representation
	if deps.mir {
		mir = constraints.GenerateMirConstraints(fir)
	}
	// Arithmetic Intermediate Representation
	if deps.air {
		air = constraints.GenerateAirConstraints(fir, p.field)
	}
	// copy over what has been requested
	if p.ast {
		artifacts.ast = util.Some(ast)
	}
	//
	if p.wir {
		artifacts.wir = util.Some(*wir)
	}
	//
	if p.fir {
		artifacts.fir = util.Some(*fir)
	}
	//
	if p.mir {
		artifacts.mir = util.Some(mir)
	}
	//
	if p.air {
		artifacts.air = util.Some(air)
	}
	//
	return artifacts
}
