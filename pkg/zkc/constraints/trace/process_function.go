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
package trace

import (
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

// InitOneLineFunction initialises a trace module for a one-line function.  The
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
func initOneLineFunction[W Word[W], F Element[F], M ModuleBuilder[F, M]](f vm.Function[W]) (module M) {
	var (
		// Native functions do not (currently) have return lines
		hasRet = !f.IsNative()
		// Copy over all parameter / return lines
		regs = array.Map(f.Registers(), toRtraceRegister)
	)
	// Check whether return line required
	if hasRet {
		// Yes, so add one
		regs = append(regs, rtrace.NewColumnDescriptor(RET_NAME, util.Some[uint](1)))
	}
	// Initialise the module
	return module.Initialise(rtrace.NewModuleDescriptor(f.Name(), regs))
}

// traceOneLineFunction materialises a trace row for a one-line function.
func traceOneLineFunction[W Word[W], F Element[F]](f vm.Function[W], m Module[F], st vm.State[W], scratch []F,
) {
	//
	var (
		one    = field.Uint64[F](1)
		hasRet = !f.IsNative()
		width  = st.Width()
		// allocate row from scratch memory
		row = scratch[:m.Width()]
	)
	// Copy over function state
	copyState(st, row)
	// Account for return line
	if hasRet {
		// Every real row of a one-line function is active.
		row[width] = one
	}
	//
	m.Append(row...)
}

// ============================================================================
// Multi-Line Functions
// ============================================================================

// InitMultiLineFunction initialises a trace module for a multi-line function
// (which requires adding control lines as necessary).  The format of a trace
// row for a multi-line function is:
//
// +----------+-----+----+--------+--------+-----+
// |   REGS   | RET | PC | IS_PC0 | IS_PC1 | ... |
// +----------+-----+----+--------+--------+-----+
//
// Here, REGS is the set of registers declared by the given function.
func initMultiLineFunction[W Word[W], F Element[F], M ModuleBuilder[F, M]](f vm.Function[W]) (module M) {
	var (
		nVectors = uint(len(f.Vectors()))
		// Copy over all address / data lines
		regs = array.Map(f.Registers(), toRtraceRegister)
		// Calculate bitwidth for PC register (recall that PC==0 is reserved for
		// padding).
		uPC = util.Some(bit.Width(nVectors + 1))
		// Bitwidth for binary selector line(s)
		u1 = util.Some[uint](1)
	)
	// Return Line
	regs = append(regs, rtrace.NewColumnDescriptor(RET_NAME, u1))
	// Program Counter
	regs = append(regs, rtrace.NewColumnDescriptor(PC_NAME, uPC))
	// PC selector lines
	for k := range nVectors {
		regs = append(regs, rtrace.NewColumnDescriptor(SelectorName(k), u1))
	}
	// Initialise the module
	return module.Initialise(rtrace.NewModuleDescriptor(f.Name(), regs))
}

// traceMultiLineFunction materialises a trace row a multi-line function
func traceMultiLineFunction[W Word[W], F Element[F]](m Module[F], st vm.State[W], scratch []F) {
	//
	var (
		one = field.Uint64[F](1)
		ret = st.Width()
		pc  = ret + 1
		row = scratch[:m.Width()]
	)
	// Copy over function state
	copyState(st, row)
	// Assign return register
	row[ret] = field.Uint1[F](st.IsTerminal())
	// Assign PC register
	row[pc] = field.Uint64[F](uint64(st.PC() + 1))
	// Zero out selector lines
	zeroOut(row[pc+1:])
	// Assign active PC selector
	row[pc+1+st.PC()] = one
	// Append trace row
	m.Append(row...)
}
