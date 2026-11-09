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
package vm

import (
	"fmt"
	"os"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/gogen"
	log "github.com/sirupsen/logrus"
)

// GoGenConfig controls the shape of the Go source GenerateGo produces.
type GoGenConfig = gogen.Config

// GoGenBinary gives a handle to a compiler gogen binary executable.
type GoGenBinary = gogen.Binary

// GoGenMachineFailure is used to indicate when a gogen binary leads to a
// machine failure, as opposed to any other kind of internal error.
var GoGenMachineFailure = gogen.ErrMachineFailure

// GoGenGenerate compiles a bytecode program into native Go source: the "fast
// execution mode" alternative to interpreting the program.  Where the bytecode
// interpreter builds a decode-dispatch loop, this emits plain Go the compiler
// can optimise — and it consumes the program over Uint (the same program the
// reference executor interprets), so the generated semantics mirror the
// reference executor, with no register splitting or word narrowing in the way.
//
// The result exposes Run(inputs) (outputs, error); with cfg.Package == "main"
// (the default) a JSON stdin/stdout harness is appended, so the artefact can be
// compiled and run as a subprocess for differential testing and benchmarking.
func GoGenGenerate(program Program[Uint], cfg GoGenConfig) (string, error) {
	return gogen.Generate(program, cfg)
}

// GoGenCompile compiles a ZkC program into a GoGen program which can be
// executed, or an error indicating some kind of compilation failure.
func GoGenCompile(program Program[Uint]) (GoGenBinary, error) {
	return gogen.Compile(program, gogen.DEFAULT_CONFIG)
}

// BootAndExecuteWithGogen executes the given program by generating native Go,
// compiling it, and running the resulting binary as a subprocess.
func BootAndExecuteWithGogen(program Program[Uint], input map[string]bit.Array) (map[string]bit.Array, []error) {
	var (
		stats  = util.NewPerfStats()
		output map[string]bit.Array
		binary gogen.Binary
		err    error
	)
	// Compile program into gogen executable.
	binary, err = gogen.Compile(program, gogen.DEFAULT_CONFIG)
	// Check for errors
	if err != nil {
		return nil, []error{err}
	}
	// Log result
	stats.Log(fmt.Sprintf("compiling binary %s", binary.Path()))
	// Make sure temporary executable is deleted, regardless of what happens.
	defer func() {
		if err := binary.Free(); err != nil {
			log.Error(err)
		}
	}()
	// Run the compiled program, passing only the machine's declared inputs (the
	// generated harness rejects unknown keys).  Forward its debug/printf and
	// fail output to our stderr so those statements are surfaced to the user.
	if output, err = binary.Run(input, os.Stderr); err != nil {
		return output, []error{err}
	}
	// success
	return output, nil
}

// BootAndCheckpointWithGogen executes the given program by generating native
// Go, compiling it, and running the resulting binary as a subprocess.  In this
// case, the generated Go code is instrumented to generate checkpoint dumps
// according to the given strategy, and these are then passed on to the
// checkpointing function.
func BootAndCheckpointWithGogen(program Program[Uint], input map[string]bit.Array, cp CheckPointer,
) (map[string]bit.Array, []error) {
	var (
		stats  = util.NewPerfStats()
		output map[string]bit.Array
		binary gogen.Binary
		err    error
	)
	// Compile program into gogen executable.
	binary, err = gogen.Compile(program, gogen.DEFAULT_CONFIG.WithCheckPointing(cp.ShardingStrategy))
	// Check for errors
	if err != nil {
		return nil, []error{err}
	}
	// Log result
	stats.Log(fmt.Sprintf("compiling binary %s", binary.Path()))
	// Make sure temporary executable is deleted, regardless of what happens.
	defer func() {
		if err := binary.Free(); err != nil {
			log.Error(err)
		}
	}()
	// Run the compiled program, passing only the machine's declared inputs (the
	// generated harness rejects unknown keys).  Forward its debug/printf and
	// fail output to our stderr so those statements are surfaced to the user.
	if output, err = binary.CheckPoint(input, cp.checkpoint, os.Stderr); err != nil {
		return output, []error{err}
	}
	// success
	return output, nil
}
