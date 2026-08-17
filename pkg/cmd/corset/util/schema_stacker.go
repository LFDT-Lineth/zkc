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
package util

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"

	log "github.com/sirupsen/logrus"
)

const (
	// MIR_LAYER represents Mid-level Intermediate Representation (MIR) which is
	// a true collection of constraints and assignments.  However, it retains a
	// more high-level perspective.
	MIR_LAYER = 3
	// AIR_LAYER represents the Arithmetic Intermediate Representation (AIR)
	// which is the bottom layer in the system, and is the representation passed
	// to the prover.
	AIR_LAYER = 4
)

// SchemaStacker is an abstraction for building a schema stacks.  It allows us
// to configure which schemas should be in the resulting stack, to configure the
// trace builder, etc.
type SchemaStacker[F field.Element[F]] struct {
	// Corset compilation config options
	corsetConfig corset.CompilationConfig
	// Mir optimisation config options
	mirConfig mir.OptimisationConfig
	// Configuration for trace expansion
	traceBuilder ir.TraceBuilder[F]
	// Layers identifies which layers are included in the stack.
	layers bit.Set
	// Schema represents the top of this stack.  This is "abstract" in the sense
	// that registers have not yet been split to fit the target field.
	schema util.Option[mir.Schema[word.BigEndian]]
	// Source map for the schema above, which maps registers and constraints
	// back to their original source-level declarations.
	sourceMap util.Option[corset.SourceMap]
}

// NewSchemaStack constructs a new, but empty stack of schemas.
func NewSchemaStack[F field.Element[F]]() *SchemaStacker[F] {
	return &SchemaStacker[F]{}
}

// WithCorsetConfig determines the compilation configuration to use for Corset.
func (p SchemaStacker[F]) WithCorsetConfig(config corset.CompilationConfig) SchemaStacker[F] {
	p.corsetConfig = config
	//
	return p
}

// WithOptimisationConfig determines the optimisation level to apply at the MIR
// layer.
func (p SchemaStacker[F]) WithOptimisationConfig(config mir.OptimisationConfig) SchemaStacker[F] {
	p.mirConfig = config
	//
	return p
}

// WithLayer identifies that the given layer should be included in the schema
// stack.
func (p SchemaStacker[F]) WithLayer(layer uint) SchemaStacker[F] {
	// clone layers first
	p.layers = p.layers.Clone()
	// add new layer
	p.layers.Insert(layer)
	//
	return p
}

// WithTraceBuilder determines the settings to use for trace expansion, such as
// whether to use parallelisation, etc.
func (p SchemaStacker[F]) WithTraceBuilder(builder ir.TraceBuilder[F]) SchemaStacker[F] {
	p.traceBuilder = builder
	//
	return p
}

// HasSchema determines whether or not a schema is available
func (p SchemaStacker[F]) HasSchema() bool {
	return p.schema.HasValue()
}

// SourceMap returns the source map for the schema representing the top of this
// stack, along with an indication of whether one is available.
func (p SchemaStacker[F]) SourceMap() (*corset.SourceMap, bool) {
	if !p.sourceMap.HasValue() {
		return nil, false
	}
	//
	srcmap := p.sourceMap.Unwrap()
	//
	return &srcmap, true
}

// Field returns the field configuration used within this schema stack.
func (p SchemaStacker[F]) Field() field.Config {
	return p.corsetConfig.Field
}

// Read reads one or more constraints files into this stack.
func (p SchemaStacker[F]) Read(filenames ...string) SchemaStacker[F] {
	schema, srcmap := CompileSourceFiles(p.corsetConfig, filenames)
	p.schema = util.Some(schema)
	p.sourceMap = util.Some(srcmap)
	//
	return p
}

// TraceBuilder returns a configured trace builder.
func (p SchemaStacker[F]) TraceBuilder() ir.TraceBuilder[F] {
	return p.traceBuilder
}

// Build a fresh SchemaStack from this stacker.
func (p SchemaStacker[F]) Build() SchemaStack[F] {
	var (
		absSchema mir.Schema[word.BigEndian]
		airSchema air.Schema[F]
		stack     SchemaStack[F]
	)
	//
	if p.schema.HasValue() {
		stats := util.NewPerfStats()
		// Read out the (abstract) schema
		absSchema = p.schema.Unwrap()
		//
		stats.Log("concretization")
		// Apply register splitting for field agnosticity
		mirSchema, mapping := mir.Concretize[word.BigEndian, F](p.corsetConfig.Field, absSchema.RawModules())
		//
		stats.Log("translation")
		// Record mapping
		stack.mapping = mapping
		// Include Mid-level IR layer (if requested)
		if p.layers.Contains(MIR_LAYER) {
			stack.concreteSchemas = append(stack.concreteSchemas, mirSchema)
			stack.names = append(stack.names, "MIR")
		}
		// Include Arithmetic-level IR layer (if requested)
		if p.layers.Contains(AIR_LAYER) {
			// Lower to AIR
			airSchema = mir.LowerToAir(mirSchema, p.Field().BandWidth, p.mirConfig)
			//
			stats.Log("arithmetizion")
			//
			stack.concreteSchemas = append(stack.concreteSchemas, schema.Any(airSchema))
			stack.names = append(stack.names, "AIR")
		}
		// Assign source map used to build the stack
		stack.sourceMap = p.sourceMap
	}
	//
	return stack
}

// CompileSourceFiles accepts a set of source (i.e. lisp) files and compiles
// them into a single schema, along with its corresponding source map.  Any
// directories given in the list of filenames are recursively expanded first.
// This can result, for example, in a syntax error, etc.  NOTES: source files
// can be compiled with (or without) the standard library.  Generally speaking,
// you want to compile with the standard library.  However, some internal tests
// are run without including the standard library to minimise the surface area.
func CompileSourceFiles(config corset.CompilationConfig, filenames []string,
) (mir.Schema[word.BigEndian], corset.SourceMap) {
	//
	var (
		err      error
		errors   []source.SyntaxError
		srcmap   corset.SourceMap
		srcfiles []source.File
		schema   mir.Schema[word.BigEndian]
	)
	//
	if len(filenames) == 0 {
		fmt.Println("source constraint(s) file required.")
		os.Exit(5)
	}
	// Recursively expand any directories given in the list of filenames.
	if filenames, err = expandSourceFiles(filenames); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	//
	srcfiles = make([]source.File, len(filenames))
	// Read each file
	for i, n := range filenames {
		log.Debug(fmt.Sprintf("including source file %s", n))
		// Read source file
		bytes, err := os.ReadFile(n)
		// Sanity check for errors
		if err != nil {
			fmt.Println(err)
			os.Exit(3)
		}
		//
		srcfiles[i] = *source.NewSourceFile(n, bytes)
	}
	// Parse and compile source files
	schema, srcmap, errors = corset.CompileSourceFiles(config, srcfiles)
	// Check for any errors
	if len(errors) == 0 {
		return schema, srcmap
	}
	// Report errors
	for _, err := range errors {
		printSyntaxError(&err)
	}
	// Fail
	os.Exit(4)
	// unreachable
	return schema, srcmap
}

// Look through the list of filenames and identify any which are directories.
// Those are then recursively expanded.
func expandSourceFiles(filenames []string) ([]string, error) {
	var expandedFilenames []string
	//
	for _, f := range filenames {
		// Lookup information on the given file.
		if info, err := os.Stat(f); err != nil {
			// Something is wrong with one of the files provided, therefore
			// terminate with an error.
			return nil, err
		} else if info.IsDir() {
			// This a directory, so read its contents
			if contents, err := expandDirectory(f); err != nil {
				return nil, err
			} else {
				expandedFilenames = append(expandedFilenames, contents...)
			}
		} else {
			// This is a single file
			expandedFilenames = append(expandedFilenames, f)
		}
	}
	//
	return expandedFilenames, nil
}

// Recursively search through a given directory looking for any lisp files.
func expandDirectory(dirname string) ([]string, error) {
	var filenames []string
	// Recursively walk the given directory.
	err := filepath.Walk(dirname, func(filename string, info os.FileInfo, err error) error {
		if !info.IsDir() && path.Ext(filename) == ".lisp" {
			filenames = append(filenames, filename)
		} else if !info.IsDir() && path.Ext(filename) == ".lispX" {
			log.Info(fmt.Sprintf("ignoring file %s", filename))
		}
		// Continue.
		return nil
	})
	// Done
	return filenames, err
}

// Print a syntax error with appropriate highlighting.
func printSyntaxError(err *source.SyntaxError) {
	span := err.Span()
	line := err.FirstEnclosingLine()
	lineOffset := span.Start() - line.Start()
	// Calculate length (ensures don't overflow line)
	length := min(line.Length()-lineOffset, span.Length())
	// Print error + line number
	fmt.Printf("%s:%d:%d-%d %s\n", err.SourceFile().Filename(),
		line.Number(), 1+lineOffset, 1+lineOffset+length, err.Message())
	// Print separator line
	fmt.Println()
	// Print line
	fmt.Println(line.String())
	// Print indent (todo: account for tabs)
	fmt.Print(strings.Repeat(" ", lineOffset))
	// Print highlight
	fmt.Println(strings.Repeat("^", length))
}
