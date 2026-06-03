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
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/binfile"
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
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"

	log "github.com/sirupsen/logrus"
)

const (
	// MACRO_ASM_LAYER represents the macro assembly layer which is most
	// high-level layer in the stack.
	MACRO_ASM_LAYER = 0
	// MICRO_ASM_LAYER represents the micro assembly layer which is typically
	// vectorised and field specific.
	MICRO_ASM_LAYER = 1
	// NANO_ASM_LAYER represents the micro assembly layer after register
	// splitting.
	NANO_ASM_LAYER = 2
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
	// Asm lowering config options
	asmConfig asm.LoweringConfig
	// Mir optimisation config options
	mirConfig mir.OptimisationConfig
	// Configuration for trace expansion
	traceBuilder ir.TraceBuilder[F]
	// Externalised constant definitions
	externs []string
	// Layers identifies which layers are included in the stack.
	layers bit.Set
	// Binfile represents the top of this stack.
	binfile util.Option[binfile.BinaryFile]
}

// NewSchemaStack constructs a new, but empty stack of schemas.
func NewSchemaStack[F field.Element[F]]() *SchemaStacker[F] {
	return &SchemaStacker[F]{}
}

// WithAssemblyConfig determines the ASM lowering configuration to use for this
// schema stack.  This determines, amongst other things, the maximum register
// size.
func (p SchemaStacker[F]) WithAssemblyConfig(config asm.LoweringConfig) SchemaStacker[F] {
	p.asmConfig = config
	//
	return p
}

// WithBinaryFile updates the binary file which forms the root of any constructed stacks.
func (p SchemaStacker[F]) WithBinaryFile(binf binfile.BinaryFile) SchemaStacker[F] {
	var stacker = p
	//
	stacker.binfile = util.Some(binf)
	//
	return stacker
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

// WithConstantDefinitions determines the externalised constant definitions to
// apply to the constructed binary file.
func (p SchemaStacker[F]) WithConstantDefinitions(externs []string) SchemaStacker[F] {
	p.externs = externs
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

// HasBinaryFile determines whether or not a binary file is available
func (p SchemaStacker[F]) HasBinaryFile() bool {
	return p.binfile.HasValue()
}

// BinaryFile returns the binary file representing the top of this stack.
func (p SchemaStacker[F]) BinaryFile() *binfile.BinaryFile {
	bf := p.binfile.Unwrap()
	return &bf
}

// Field returns the field configuration used within this schema stack.
func (p SchemaStacker[F]) Field() field.Config {
	return p.asmConfig.Field
}

// Read reads one or more constraints files into this stack.
func (p SchemaStacker[F]) Read(filenames ...string) SchemaStacker[F] {
	bf := readConstraintFiles(p.corsetConfig, p.asmConfig, filenames)
	p.binfile = util.Some(bf)
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
		asmProgram  asm.MacroHirProgram
		uasmProgram asm.MicroHirProgram
		airSchema   air.Schema[F]
		stack       SchemaStack[F]
	)
	//
	if p.binfile.HasValue() {
		stats := util.NewPerfStats()
		binfile := p.binfile.Unwrap()
		// Apply any user-specified values for externalised constants.
		applyExternOverrides(p.externs, &binfile)
		// Read out the mixed macro schema
		asmProgram = binfile.Schema
		// Lower to mixed micro schema
		uasmProgram = asm.LowerMixedMacroProgram(p.asmConfig.Vectorize, asmProgram)
		//
		stats.Log("lowering")
		// Apply register splitting for field agnosticity
		nasmProgram, mapping := asm.Concretize[F](p.asmConfig.Field, uasmProgram)
		//
		stats.Log("concretization")
		// Compile
		mirSchema := asm.Compile(nasmProgram)
		//
		stats.Log("translation")
		// Record mapping
		stack.mapping = mapping
		// Include (Macro) Assembly Layer (if requested)
		if p.layers.Contains(MACRO_ASM_LAYER) {
			stack.abstractSchemas = append(stack.abstractSchemas, &asmProgram)
			stack.names = append(stack.names, "ASM")
		}
		// Include (Micro) Assembly Layer (if requested)
		if p.layers.Contains(MICRO_ASM_LAYER) {
			stack.abstractSchemas = append(stack.abstractSchemas, &uasmProgram)
			stack.names = append(stack.names, "UASM")
		}
		// Include (Micro) Assembly Layer (if requested)
		if p.layers.Contains(NANO_ASM_LAYER) {
			stack.concreteSchemas = append(stack.concreteSchemas, &nasmProgram)
			stack.names = append(stack.names, "NASM")
		}
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
		// Assign trace builder with limb map
		stack.traceBuilder = p.traceBuilder.WithRegisterMapping(mapping)
		// Assign binfile used to build the stack
		stack.binfile = util.Some(binfile)
	}
	//
	return stack
}

// readConstraintFiles provides a generic interface for reading constraint files
// in one of two ways.  If a single file is provided with the "bin" extension
// then this is treated as a binfile (e.g. zkevm.bin).  Otherwise, the files are
// assumed to be source (i.e. lisp) files and are read in and then compiled into
// a binfile.  NOTES:  when source files are provided, they can be compiled with
// (or without) the standard library.  Generally speaking, you want to compile
// with the standard library.  However, some internal tests are run without
// including the standard library to minimise the surface area.
func readConstraintFiles(config corset.CompilationConfig, lowering asm.LoweringConfig,
	filenames []string) binfile.BinaryFile {
	//
	var err error
	//
	if len(filenames) == 0 {
		fmt.Println("source or binary constraint(s) file required.")
		os.Exit(5)
	} else if len(filenames) == 1 && path.Ext(filenames[0]) == ".bin" {
		// Single (binary) file supplied
		return ReadBinaryFile(filenames[0])
	}
	// Recursively expand any directories given in the list of filenames.
	if filenames, err = expandSourceFiles(filenames); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// Must be source files
	return CompileSourceFiles(config, lowering, filenames)
}

// ReadAssemblyProgram reads a given set of assembly files into a (macro) assembly program.
func ReadAssemblyProgram(filenames ...string) (asm.MacroProgram, source.Maps[any]) {
	srcfiles, err := source.ReadFiles(filenames...)
	//
	if err != nil {
		panic(err)
	}
	//
	program, srcmaps, errs := asm.Assemble(srcfiles...)
	//
	if len(errs) == 0 {
		return program, srcmaps
	}
	// Report errors
	for _, err := range errs {
		printSyntaxError(&err)
	}
	// Fail
	os.Exit(4)
	// Unreachable
	return asm.MacroProgram{}, srcmaps
}

// ReadBinaryFile reads a binfile which includes the metadata bytes, along with
// the schema, and any included attributes.
func ReadBinaryFile(filename string) binfile.BinaryFile {
	var binf binfile.BinaryFile
	// Read schema file
	data, err := os.ReadFile(filename)
	// Handle errors
	if err == nil {
		err = binf.UnmarshalBinary(data)
	}
	// Return if no errors
	if err == nil {
		return binf
	}
	// Handle error & exit
	fmt.Println(err)
	os.Exit(2)
	// unreachable
	return binf
}

// CompileSourceFiles accepts a set of source files and compiles them into a
// single schema.  This can result, for example, in a syntax error, etc.  This
// can be done with (or without) including the standard library, and also with
// (or without) debug constraints.
func CompileSourceFiles(config corset.CompilationConfig, asmConfig asm.LoweringConfig,
	filenames []string) binfile.BinaryFile {
	//
	var (
		errors            []source.SyntaxError
		srcmap            corset.SourceMap
		srcfiles          = make([]source.File, len(filenames))
		mixedMacroProgram asm.MacroHirProgram
	)
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
	// Separate Corset from ASM files.
	corsetFiles, asmFiles := splitSourceFiles(srcfiles)
	// Compile ASM files
	macroProgram, _, errors := asm.Assemble(asmFiles...)
	// Continue if no errors
	if len(errors) == 0 {
		// Parse and compile source files
		mixedMacroProgram, srcmap, errors = corset.CompileSourceFiles(config, corsetFiles, macroProgram)
		// Check for any errors
		if len(errors) == 0 {
			attributes := []binfile.Attribute{&srcmap}
			return *binfile.NewBinaryFile(nil, attributes, mixedMacroProgram)
		}
	}
	// Report errors
	for _, err := range errors {
		printSyntaxError(&err)
	}
	// Fail
	os.Exit(4)
	// unreachable
	return binfile.BinaryFile{}
}

func splitSourceFiles(srcfiles []source.File) ([]source.File, []source.File) {
	var (
		corsetFiles []source.File
		asmFiles    []source.File
	)
	// Expand assembly programs
	for _, n := range srcfiles {
		if path.Ext(n.Filename()) == ".zkasm" {
			asmFiles = append(asmFiles, n)
		} else {
			corsetFiles = append(corsetFiles, n)
		}
	}
	//
	return corsetFiles, asmFiles
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

// Apply any user-specified values for the given externalised constants.  Each
// constant should be checked that it exists, to ensure assignments are not
// silently dropped.
func applyExternOverrides(externs []string, binf *binfile.BinaryFile) {
	// NOTE: frMapping is to be deprecated and removed.
	var (
		frMapping = make(map[string]word.BigEndian)
		biMapping = make(map[string]big.Int)
	)
	// Sanity check debug information is available.
	srcmap, srcmap_ok := binfile.GetAttribute[*corset.SourceMap](binf)
	// Check if need to do anything.
	if len(externs) > 0 {
		//
		for _, item := range externs {
			var (
				frElement fr.Element
				biElement big.Int
			)
			//
			split := strings.Split(item, "=")
			if len(split) != 2 {
				fmt.Printf("malformed definition \"%s\"\n", item)
				os.Exit(2)
			}
			//
			path := strings.Split(split[0], ".")
			// More sanity checks
			if srcmap_ok && !checkExternExists(path, srcmap.Root) {
				fmt.Printf("unknown externalised constant \"%s\"\n", split[0])
				os.Exit(2)
			} else if _, err := frElement.SetString(split[1]); err != nil {
				fmt.Println(err.Error())
				os.Exit(2)
			} else if _, ok := biElement.SetString(split[1], 0); !ok {
				fmt.Printf("error parsing string \"%s\"\n", split[1])
				os.Exit(2)
			}
			//
			frMapping[split[0]] = word.NewBigEndian(biElement.Bytes())
			biMapping[split[0]] = biElement
		}
		// Substitute through constraints
		mir.SubstituteConstants(&binf.Schema, frMapping)
		// Update source mapping
		srcmap.SubstituteConstants(biMapping)
	}
}

func checkExternExists(name []string, mod corset.SourceModule) bool {
	switch len(name) {
	case 0:

	case 1:
		// look for it in this module
		for _, c := range mod.Constants {
			if name[0] == c.Name {
				return true
			}
		}
	default:
		// look for suitable submodule
		for _, submod := range mod.Submodules {
			if name[0] == submod.Name {
				return checkExternExists(name[1:], submod)
			}
		}
	}
	//
	return false
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
