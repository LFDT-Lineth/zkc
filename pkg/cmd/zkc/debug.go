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
	"fmt"
	"os"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug [flags] input.json file1.zkc file2.zkc ...",
	Short: "Debug a zkc program.",
	Long:  `Debug a zkc program to produce a set of outputs a from given a set of inputs.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, debugCmds)
	},
}

// Available instances
var debugCmds = []FieldAgnosticCmd{
	{field.GF_251, runDebugCmd[gf251.Element]},
	{field.GF_8209, runDebugCmd[gf8209.Element]},
	{field.KOALABEAR_16, runDebugCmd[koalabear.Element]},
	{field.BLS12_377, runDebugCmd[bls12_377.Element]},
}

func runDebugCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		build = GetBuildConfig[F](cmd, field)
	)
	//
	input := ParseInputFile(args[0])
	// Build artifacts (compiles source files or loads a prebuilt binary).
	artifacts := Build[F](build, args[1:]...)
	// Lower the bytecode program to a fixed-width form the bytecode interpreter
	// can execute (mirroring the execute / trace commands).
	program := vm.ProgramToProgram[vm.Uint, vm.Uint128](artifacts.ir)
	// Filter out unnecessary inputs
	input = vm.FilterInputs(program, input)
	// Construct a trace observer which prints each executed trace line, with
	// register values rendered inline.
	observer := NewDebugger(program)
	// Boot & execute via the bytecode interpreter, printing a trace line for
	// each executed bytecode vector.
	errs := vm.BootAndDebug(program, input, observer.Observe)
	//
	if len(errs) > 0 {
		for _, e := range errs {
			log.Error(e)
		}
		//
		os.Exit(4)
	}
	//
	fmt.Println()
}

// ============================================================================
// Misc
// ============================================================================

// Debugger renders an execution trace to stdout using the bytecode
// interpreter's debug facility (see vm.BootAndDebug).  It prints one line per
// executed trace line (bytecode vector), rendered against the enclosing
// function's environment, with register values shown inline (see valueEnv).
type Debugger[W vm.Word[W]] struct {
	// program being debugged, used to resolve the executing function's bytecode
	// and environment at each step.
	program vm.Program[W]
	// started indicates whether at least one state has been observed (so the
	// first function header is always printed).
	started bool
	// prevFid records the previously executing function, used to detect when
	// control enters a different function (so a header can be printed).
	prevFid uint16
}

// NewDebugger constructs a trace observer for the given (lowered) bytecode
// program.
func NewDebugger[W vm.Word[W]](program vm.Program[W]) *Debugger[W] {
	return &Debugger[W]{program: program}
}

// Observe is invoked once per executed trace line (bytecode vector).  It is
// intended to be passed as the observer callback to vm.BootAndDebug.
func (p *Debugger[W]) Observe(st vm.State[W]) {
	fid := st.Fid()
	//
	fn, ok := p.program.Module(fid).(*vm.Function[W])
	if !ok {
		// Only functions carry executable bytecode; nothing to render otherwise.
		return
	}
	// Print a function header whenever execution (re-)enters a different function.
	if !p.started || fid != p.prevFid {
		fmt.Printf("\n> %s\n", fn.Name())
		//
		p.started = true
		p.prevFid = fid
	}
	//
	var (
		vectors = fn.Vectors()
		// Wrap the function's environment so register values are rendered inline.
		env = valueEnv[W]{p.program.EnvironmentOf(fid), st.Frame()}
	)
	//
	if st.PC() < uint(len(vectors)) {
		vec := vectors[st.PC()]
		fmt.Printf("[%02x] %s\n", st.PC(), vec.String(env))
	}
}

// valueEnv wraps a bytecode environment, supplementing it with the current
// register values recorded for the active frame.  This is what allows
// Bytecode.String to render each register's value inline (via
// Environment.ValueOf).
type valueEnv[W vm.Word[W]] struct {
	vm.BytecodeEnvironment[W]
	frame []W
}

// ValueOf returns the current value held in the given register, when available.
func (e valueEnv[W]) ValueOf(id vm.RegisterId) util.Option[W] {
	if int(id) < len(e.frame) {
		return util.Some(e.frame[id])
	}
	//
	return util.None[W]()
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(debugCmd)
}
