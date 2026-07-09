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
package post

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// Transcriber describes a process
type Transcriber[W Word[W], F Element[F]] func(state vm.State[W]) []F

// ============================================================================
// One-Line Functions
// ============================================================================

// ProcessOneLineFunction performs post-processing on a one-line function.  The
// format of a trace row for a (non-native) one-line function is:
//
// +----------+-----+
// |   REGS   | RET |
// +----------+-----+
//
// Here, REGS is the set of registers declared by the given function.  The RET
// activity line is 1 on every non-padding row, since each instance of a one-line
// function occupies exactly one active row; padding rows are left at 0.
// TODO: see https://github.com/LFDT-Lineth/zkc/issues/1975
// OLI won't have a $ret column
func ProcessOneLineFunction[W Word[W], F Element[F]](f vm.Function[W], states []vm.State[W]) rtrace.ArrayModule[F] {
	var (
		hasRet = !f.IsNative()
		regs   = array.Map(f.Registers(), toRtraceRegister)
		rows   = transcribe(states, oneLineTranscriber[W, F](hasRet))
	)
	// Return Line
	if hasRet {
		regs = append(regs, rtrace.NewRegister(io.RET_NAME, util.Some([]uint{1})))
	}
	//
	return rtrace.NewArrayModule(f.Name(), regs, rows...)
}

func oneLineTranscriber[W Word[W], F Element[F]](hasRet bool) Transcriber[W, F] {
	var one = field.Uint64[F](1)
	//
	return func(st vm.State[W]) []F {
		var width = st.Width()
		//
		if !hasRet {
			var row = make([]F, width)
			copyState(st, row)

			return row
		}
		// Reserve an extra column for the $ret activity line.
		var row = make([]F, width+1)
		// Copy over function state
		copyState(st, row)
		// Every real row of a one-line function is active.
		row[width] = one
		//
		return row
	}
}

// ============================================================================
// Multi-Line Functions
// ============================================================================

// ProcessMultiLineFunction performs post-processing on a multi-line function
// (which requires adding control lines as necessary).  The format of a trace
// row for a multi-line function is:
//
// +----------+-----+----+--------+--------+-----+
// |   REGS   | RET | PC | IS_PC0 | IS_PC1 | ... |
// +----------+-----+----+--------+--------+-----+
//
// Here, REGS is the set of registers declared by the given function.
func ProcessMultiLineFunction[W Word[W], F Element[F]](f vm.Function[W], states []vm.State[W]) rtrace.ArrayModule[F] {
	var (
		nVectors = uint(len(f.Vectors()))
		regs     = determineMultiLineFnRegisters(f.Registers(), nVectors)
		rows     = transcribe(states, multiLineTranscriber[W, F](nVectors))
	)
	//
	return rtrace.NewArrayModule(f.Name(), regs, rows...)
}

// Determine the full set of registers required for the trace of this funcion.
func determineMultiLineFnRegisters[W vm.Word[W]](registers []vm.Register[W], nVectors uint) []rtrace.Register {
	var (
		// Copy over all address / data lines
		regs = array.Map(registers, toRtraceRegister)
		// Calculate bitwidth for PC register (recall that PC==0 is reserved for
		// padding).
		uPC = util.Some([]uint{bit.Width(nVectors + 1)})
		// Bitwidth for binary selector line(s)
		u1 = util.Some([]uint{1})
	)
	// Return Line
	regs = append(regs, rtrace.NewRegister(io.RET_NAME, u1))
	// Program Counter
	regs = append(regs, rtrace.NewRegister(io.PC_NAME, uPC))
	// PC selector lines
	for k := range nVectors {
		regs = append(regs, rtrace.NewRegister(io.SelectorName(k), u1))
	}
	//
	return regs
}

func multiLineTranscriber[W Word[W], F Element[F]](nVectors uint) Transcriber[W, F] {
	var one = field.Uint64[F](1)
	//
	return func(st vm.State[W]) []F {
		var (
			ret = st.Width()
			pc  = ret + 1
			row = make([]F, st.Width()+2+nVectors)
		)
		// Copy over function state
		copyState(st, row)
		// Assign return register
		row[ret] = field.Uint1[F](st.IsTerminal())
		// Assign PC register
		row[pc] = field.Uint64[F](uint64(st.PC() + 1))
		// Assign active PC selector
		row[pc+1+st.PC()] = one
		//
		return row
	}
}

// ============================================================================
// Helpers
// ============================================================================

func transcribe[W Word[W], F Element[F]](states []vm.State[W], scribe Transcriber[W, F]) [][]F {
	var rows [][]F = make([][]F, len(states))
	// Transcribe states (for now).
	for i, st := range states {
		rows[i] = scribe(st)
	}
	//
	return rows
}

// Copy all words out of the given state, and assign them into the give array
// after conversion.
func copyState[W Word[W], F Element[F]](st vm.State[W], fields []F) {
	// Copy over state registers
	for i, v := range st.Frame() {
		var val F
		// Copy over data
		fields[i] = val.SetBytes(v.BigInt().Bytes())
	}
}
