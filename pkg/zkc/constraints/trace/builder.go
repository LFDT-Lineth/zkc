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
			modules[i] = initialiseMemory[W, F, M](*m)
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
func (p Builder[W, F, M]) TraceMemory(mid uint16, m vm.RuntimeMemory[W]) {
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
		traceReadWriteMemory(m, module)
	}
}

func initialiseMemory[W Word[W], F Element[F], M rtrace.ModuleBuilder[F, M]](memory vm.Memory[W]) M {
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
		return initReadWriteMemory[W, F, M](memory)
	}
}
