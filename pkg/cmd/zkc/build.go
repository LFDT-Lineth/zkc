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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
)

// BuildArtifacts captures the set of outputs generated from compiling a given
// ZkC program (e.g. AIR constraints).  Whilst the AIR artifact might be
// considered the primary goal of compilation, the other artifacts are needed to
// support other features.  For example, the FIR artifact is needed for trace
// expansion, whilst the AST artifact allows the AST to be printed for debugging
// purposes.
type BuildArtifacts struct {
	// Abstract Syntax Tree
	ast util.Option[ast.Program]
	// Word Machine
	wir vm.WordMachine[vm.Uint]
	// Annotations on source-level declarations (functions and memories),
	// keyed by declaration name.  These are not carried through to the lower
	// levels, hence they are retained here for printing purposes.  Observe
	// this is empty when building from a prebuilt binary.
	annotations map[string][]string
}

// BuildConfig packages up all the requirements for building the set of target
// artifacts.
type BuildConfig struct {
	// code configuration includes various things which can be turned off / on.
	config codegen.Config
	// field configuration
	field field.Config
	// metadata to include in binary output file
	metadata []byte
	// flags signal which layers to generate artifacts for.
	fastMode bool
	// enable go code generator
	gogen bool
}

// Build applies a build configuration with a given set of source files.
func Build[F field.Element[F]](build BuildConfig, args ...string) BuildArtifacts {
	var (
		errs []source.SyntaxError
		//
		artifacts BuildArtifacts
	)
	// Check whether prebuilt binary supplied on command-line.
	if len(args) > 0 && path.Ext(args[0]) == ".bin" {
		// Sanity check exactly one prebuilt binary provide.
		if len(args) != 1 {
			log.Error("require exactly one prebuilt binary")
			os.Exit(6)
		}
		// Single (binary) file supplied
		wm := ReadBinaryFile[F](args[0]).WordMachine()
		// Assign over
		artifacts.wir = wm
	} else {
		var wir *vm.WordMachine[vm.Uint]
		// Compile source files, or print errors
		ast := CompileSourceFiles(build.field, args...)
		// Record AST (e.g. for debugging)
		artifacts.ast = util.Some(ast)
		// Record annotations for printing purposes
		artifacts.annotations = annotationsOf(ast)
		// Word-level Intermediate Representation
		// Compile the AST into the top-level word machine
		wir, errs = ast.Compile(build.config)
		//
		artifacts.wir = *wir
		//
		if len(errs) > 0 {
			for _, err := range errs {
				printSyntaxError(&err)
			}
			//
			os.Exit(4)
		}
	}
	//
	return artifacts
}

// annotationsOf extracts the annotations associated with each annotated
// declaration (i.e. function or memory) in a given program, keyed by the
// declaration's name.
func annotationsOf(program ast.Program) map[string][]string {
	var annotations = make(map[string][]string)
	//
	for _, d := range program.Components() {
		if annots := d.Annotations(); len(annots) > 0 {
			annotations[d.Name()] = annots
		}
	}
	//
	return annotations
}
