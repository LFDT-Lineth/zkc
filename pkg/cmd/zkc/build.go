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

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
)

// BuildConfig packages up all the requirements for building the set of target
// artifacts.
type BuildConfig struct {
	// code configuration includes various things which can be turned off / on.
	config codegen.Config
	// fast mode determination
	fastMode bool
	// metadata to include in binary output file
	metadata util.Option[[]byte]
	// enable go code generator
	gogen bool
	// padding strategy
	padding ir.PaddingStrategy
	// ignored pipeline stages
	ignores []string
}

// Build applies a build configuration with a given set of source files.
func Build[F field.Element[F]](build BuildConfig, args ...string) (*ast.Program, *constraints.BinaryFile[F]) {
	var (
		errs []source.SyntaxError
		raw  vm.Program[vm.Uint]
	)
	// Check whether prebuilt binary supplied on command-line.
	if len(args) > 0 && path.Ext(args[0]) == ".bin" {
		// Sanity check exactly one prebuilt binary provide.
		if len(args) != 1 {
			log.Error("require exactly one prebuilt binary")
			os.Exit(6)
		}
		//
		var (
			// Read existing binary file
			binf = ReadBinaryFile[F](args[0])
			// Determine metadata
			metadata = build.metadata.UnwrapOr(binf.Header().MetaData)
		)
		// Single (binary) file supplied
		return nil, constraints.NewBinaryFile[F](metadata, binf.Attributes(), binf.RawProgram()).
			WithIgnores(build.ignores...)
	}
	// Compile source files, or print errors
	prog := CompileSourceFiles(build.config.GetField(), args...)
	// Word-level Intermediate Representation
	// Compile the AST into the top-level word machine
	raw, errs = ast.Compile(prog, build.config)
	//
	if len(errs) > 0 {
		for _, err := range errs {
			printSyntaxError(&err)
		}
		//
		os.Exit(4)
	}
	//
	return &prog, constraints.NewBinaryFile[F](build.metadata.UnwrapOr(nil), nil, raw).
		WithIgnores(build.ignores...)
}
