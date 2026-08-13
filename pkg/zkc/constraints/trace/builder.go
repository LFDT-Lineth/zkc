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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

type (
	// Word provides a useful alias
	Word[W any] = vm.Word[W]
	// Element provides a useful alias
	Element[F any] = field.Element[F]
	// Memory provides a useful alias
	Memory[W Word[W]] = vm.RuntimeMemory[W]
	// Module provides a useful alias
	Module[F any] = rtrace.Module[F]
	// ModuleBuilder provides a useful alias
	ModuleBuilder[F any, M rtrace.Module[F]] = rtrace.ModuleBuilder[F, M]
)

const (
	// PC_NAME gives the name used for the program counter in traces.
	PC_NAME = "$pc"
	// RET_NAME gives the name used for the return line in traces.
	RET_NAME = "$ret"
	// IS_PC_PREFIX gives the prefix used for the one-hot program counter
	// selector lines in traces.  A selector for PC value v is named
	// "$is_pc_<v>".
	IS_PC_PREFIX = "$is_pc_"
	// AT_FLAG_PREFIX name of binary flag names for multi-line address increments
	AT_FLAG_PREFIX = "$at_flag_"
	// ACCESS_BIT_NAME name of binary flag for non-padding rows
	ACCESS_BIT_NAME = "$access_bit"
	// RAM_EXEC_NAME is the binary flag marking a read-write memory (RAM) row as
	// belonging to the guest-program execution phase.
	RAM_EXEC_NAME = "$exec"
	// RAM_FINL_NAME is the binary flag marking a RAM row as belonging to the
	// initialization / finalization phase.
	RAM_FINL_NAME = "$finl"
	// RAM_IS_WRITE_NAME is the binary flag distinguishing a RAM write (1) from a
	// RAM read (0).
	RAM_IS_WRITE_NAME = "$is_write"
	// RAM_VALUE_READ_PREFIX prefixes the per-limb "value read" columns of a RAM
	// module (the value held at the accessed address immediately before the
	// access).
	RAM_VALUE_READ_PREFIX = "$value_read_"
	// RAM_TS_WRITTEN_PREFIX prefixes the per-limb timestamp-written columns.
	RAM_TS_WRITTEN_PREFIX = "$ts_written_"
	// RAM_TS_READ_PREFIX prefixes the per-limb timestamp-read columns.
	RAM_TS_READ_PREFIX = "$ts_read_"
	// RAM_TS_DELTA_PREFIX prefixes the per-limb timestamp-delta columns (the gap
	// enforcing TIMESTAMP_READ < TIMESTAMP_WRITTEN).
	RAM_TS_DELTA_PREFIX = "$ts_delta_"
	// RAM_ADDR_DELTA_PREFIX prefixes the per-limb address-delta columns (used to
	// prove address monotony in the finalization phase).
	RAM_ADDR_DELTA_PREFIX = "$addr_delta_"
	// RAM_TS_CARRY_PREFIX prefixes the per-boundary carry columns witnessing the
	// multi-limb timestamp addition TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA.
	RAM_TS_CARRY_PREFIX = "$ts_carry_"
	// RAM_ADDR_CARRY_PREFIX prefixes the per-boundary carry columns witnessing the
	// multi-limb address addition in the finalization phase.
	RAM_ADDR_CARRY_PREFIX = "$addr_carry_"
	// RAM_EXEC_WRITE_NAME is the binary column EXEC * IS_WRITE: the target-side
	// selector of the caller->RAM lookup for write accesses.
	RAM_EXEC_WRITE_NAME = "$exec_write"
	// RAM_EXEC_READ_NAME is the binary column EXEC * (1 - IS_WRITE): the
	// target-side selector of the caller->RAM lookup for read accesses.
	RAM_EXEC_READ_NAME = "$exec_read"
)

// RamLimbName returns the name of the limb-k column of a RAM register family
// with the given prefix.
func RamLimbName(prefix string, k uint) string {
	return fmt.Sprintf("%s%d", prefix, k)
}

// SelectorName returns the name of the one-hot program counter selector
// register for the instruction at the given (zero-based) code line.  The PC
// value for code line c is c+1 (PC==0 is reserved for padding).
func SelectorName(line uint) string {
	return fmt.Sprintf("%s%d", IS_PC_PREFIX, line+1)
}

// AtFlagName returns the name of the binary "at flag" register for address
// limb k, used to locate the carry-stop limb on a multi-limb address increment.
func AtFlagName(k uint) string {
	return fmt.Sprintf("%s%d", AT_FLAG_PREFIX, k)
}

// convert register descriptor into rtrace register
func toRtraceRegister[W Word[W]](_ uint, reg vm.Register[W]) rtrace.ColumnDescriptor {
	return rtrace.NewColumnDescriptor(reg.Name(), reg.Bitwidth())
}

// Builder for post-processing recorded state for a given module.  For
// memories, this means transcribing the state into a suitable trace format with
// auxiliary registers as required (e.g. for selector bits, etc).  For
// functions, this means transcribing each state generated for the function
// during execution.
type Builder[W Word[W], F Element[F], M rtrace.ModuleBuilder[F, M]] struct {
	descriptors []vm.Module[W]
	// set of modules actively being traced
	modules []M
	// scratch memory area, used to avoid memory allocation.
	scratch []F
}

// NewBuilder initialises a new trace builder from a given program.
func NewBuilder[W Word[W], F Element[F], M rtrace.ModuleBuilder[F, M]](program vm.Program[W]) Builder[W, F, M] {
	var (
		maxWidth uint
		//
		modules = make([]M, len(program.Modules()))
	)
	//
	for i, m := range program.Modules() {
		switch m := m.(type) {
		case *vm.Function[W]:
			if m.IsOneLine() {
				modules[i] = initOneLineFunction[W, F, M](*m)
			} else {
				modules[i] = initMultiLineFunction[W, F, M](*m)
			}
		case *vm.Memory[W]:
			modules[i] = initialiseMemory[W, F, M](program.Field(), *m)
		}
		// Update maximum width
		maxWidth = max(maxWidth, modules[i].Width())
	}
	// allocate scratch memory
	return Builder[W, F, M]{program.Modules(), modules, make([]F, maxWidth)}
}

// Build implementation for TraceBuilder interface.
func (p Builder[W, F, M]) Build() rtrace.Trace[F] {
	return rtrace.NewArray(p.modules)
}

// TraceFunctionLine implementation for the vm.TraceBuilder interface.
func (p Builder[W, F, M]) TraceFunctionLine(state vm.State[W]) {
	var (
		mod = p.modules[state.Fid()]
		f   = p.descriptors[state.Fid()].(*vm.Function[W])
	)
	//
	if f.IsOneLine() {
		traceOneLineFunction(*f, mod, state, p.scratch)
	} else {
		traceMultiLineFunction(mod, state, p.scratch)
	}
}

// TraceMemory implementation for the vm.TraceBuilder interface.
func (p Builder[W, F, M]) TraceMemory(mid uint16, m vm.RuntimeMemory[W], field field.Config) {
	var module = p.modules[mid]
	//
	switch m.Descriptor().Kind() {
	case vm.PRIVATE_STATIC_MEMORY, vm.PUBLIC_STATIC_MEMORY:
		// skip
	case vm.PRIVATE_READ_ONLY_MEMORY, vm.PUBLIC_READ_ONLY_MEMORY:
		traceAccessOnceMemory(m, module, p.scratch)
	case vm.PRIVATE_WRITE_ONCE_MEMORY, vm.PUBLIC_WRITE_ONCE_MEMORY:
		traceAccessOnceMemory(m, module, p.scratch)
	default:
		traceReadWriteMemory(m, module, field, p.scratch)
	}
}

func initialiseMemory[W Word[W], F Element[F], M rtrace.ModuleBuilder[F, M]](cfg field.Config, memory vm.Memory[W]) M {
	switch memory.Kind() {
	case vm.PRIVATE_STATIC_MEMORY, vm.PUBLIC_STATIC_MEMORY:
		var empty M
		// ProcessStaticMemory does what is required to represent a static memory within
		// a trace.  Specifically, static memories do exist in the trace, but only to
		// ensure alignment of module identifiers.  Hence, they always have an empty trace.
		return empty.Initialise(memory.Name(), nil)
	case vm.PRIVATE_READ_ONLY_MEMORY, vm.PUBLIC_READ_ONLY_MEMORY:
		return initAccessOnceMemory[W, F, M](memory)
	case vm.PRIVATE_WRITE_ONCE_MEMORY, vm.PUBLIC_WRITE_ONCE_MEMORY:
		return initAccessOnceMemory[W, F, M](memory)
	default:
		return initReadWriteMemory[W, F, M](cfg, memory)
	}
}
