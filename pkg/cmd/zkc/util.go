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
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/consensys/go-corset/pkg/trace/json"
	"github.com/consensys/go-corset/pkg/trace/lt"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/file"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast"
	"github.com/consensys/go-corset/pkg/zkc/constraints"
	"github.com/consensys/go-corset/pkg/zkc/util"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func startCpuProfiling(cmd *cobra.Command) *os.File {
	if filename := GetString(cmd, "cpuprof"); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		//
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		//
		return f
	}
	//
	return nil
}

func stopCpuProfiling(cmd *cobra.Command, f *os.File) {
	if filename := GetString(cmd, "cpuprof"); filename != "" {
		pprof.StopCPUProfile()
	}
	// Close file hand (if opened)
	if f != nil {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}
}

func writeMemProfile(cmd *cobra.Command) {
	if filename := GetString(cmd, "memprof"); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		//nolint
		defer f.Close()
		//
		runtime.GC()
		//
		if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}

// ParseInputFile parses a given input file (which is currently assumed to be
// JSON).  An input file contains the input bytes for each ROM in the given
// program.
func ParseInputFile(filename string) map[string][]byte {
	var (
		inputFile map[string][]byte
		// Read input file
		fn, fileBytes, err = file.ReadAndUncompress(filename)
	)
	//
	if err == nil {
		ext := path.Ext(fn)
		//
		switch ext {
		case ".json":
			inputFile, err = util.ParseJsonInputFile(fileBytes)
		default:
			err = fmt.Errorf("unknown trace file format: %s", ext)
		}
	}
	// Handle error (if applicable)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	// Done
	return inputFile
}

// CompileSourceFiles accepts a set of source files and compiles them into a
// program.  This can result, for example, in one or more syntax errors, etc.
func CompileSourceFiles(field field.Config, filenames ...string) ast.Program {
	//
	var (
		errors   []source.SyntaxError
		srcfiles = make([]source.File, len(filenames))
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
	// Compile source files
	macroProgram, _, errors := compiler.Compile(field, srcfiles...)
	// Check for errors
	if len(errors) != 0 {
		// Report errors
		for _, err := range errors {
			printSyntaxError(&err)
		}
		// Fail
		os.Exit(4)
	}
	// Done
	return macroProgram
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

// WriteTraceFile writes a given lt trace file to disk, either in JSON or LT
// formats.
func WriteTraceFile(filename string, tracefile lt.TraceFile) {
	var (
		err   error
		bytes []byte
	)
	// Check file extension
	ext := path.Ext(filename)
	//
	switch ext {
	case ".json":
		js := json.ToJsonString(tracefile.RawModules())
		//
		if err = os.WriteFile(filename, []byte(js), 0644); err == nil {
			return
		}
	case ".lt":
		bytes, err = tracefile.MarshalBinary()
		//
		if err == nil {
			if err = os.WriteFile(filename, bytes, 0644); err == nil {
				return
			}
		}
	default:
		err = fmt.Errorf("unknown trace file format: %s", ext)
	}
	// Handle error
	fmt.Println(err)
	os.Exit(4)
}

// WriteBinaryFile writes a binary file (e.g. zkvm.bin) to disk using the given
// binfile versioning defined in the binfile package.
//
//nolint:errcheck
func WriteBinaryFile[F field.Element[F]](binfile *constraints.BinaryFile[F], filename string) {
	var (
		bytes []byte
		err   error
	)
	// Encode binary file as bytes
	if bytes, err = binfile.MarshalBinary(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	} else if path.Ext(filename) == ".hex" {
		h := fmt.Sprintf("0x%s", hex.EncodeToString(bytes))
		bytes = []byte(h)
	}
	// Write file
	if err := os.WriteFile(filename, bytes, 0644); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// ReadBinaryFile reads a binary constraints file from disk
func ReadBinaryFile[F field.Element[F]](filename string) *constraints.BinaryFile[F] {
	var binf constraints.BinaryFile[F]
	// Read schema file
	data, err := os.ReadFile(filename)
	// Handle errors
	if err == nil {
		err = binf.UnmarshalBinary(data)
	}
	// Return if no errors
	if err == nil {
		return &binf
	}
	// Handle error & exit
	fmt.Println(err)
	os.Exit(2)
	// unreachable
	return &binf
}
