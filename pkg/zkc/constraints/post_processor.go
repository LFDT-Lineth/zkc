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
package constraints

import (
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/post"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

type (
	// Word provides a useful alias
	Word[W any] = vm.Word[W]
	// Element provides a useful alias
	Element[F any] = field.Element[F]
	// Memory provides a useful alias
	Memory[W Word[W]] = vm.RuntimeMemory[W]
)

// Post processor for post-processing recorded state for a given module.  For
// memories, this means transcribing the state into a suitable trace format with
// auxiliary registers as required (e.g. for selector bits, etc).  For
// functions, this means transcribing each state generated for the function
// during execution.
type postProcess[W Word[W], F Element[F]] struct {
	// field configuration of the trace being produced; needed to size the
	// synthetic RAM timestamp columns (which split according to the field's
	// register width) so they match the constraint schema.
	field field.Config
}

// TraceFunction implementation for the vm.TraceProcessor interface.
func (p *postProcess[W, F]) TraceFunction(f vm.Function[W], states []vm.State[W]) rtrace.ArrayModule[F] {
	if f.IsOneLine() {
		return post.ProcessOneLineFunction[W, F](f, states)
	}
	//
	return post.ProcessMultiLineFunction[W, F](f, states)
}

// TraceMemory implementation for the vm.TraceProcessor interface.
func (p *postProcess[W, F]) TraceMemory(m vm.RuntimeMemory[W]) rtrace.ArrayModule[F] {
	switch m.Descriptor().Kind() {
	case vm.PRIVATE_STATIC_MEMORY, vm.PUBLIC_STATIC_MEMORY:
		// ProcessStaticMemory does what is required to represent a static memory within
		// a trace.  Specifically, static memories do exist in the trace, but only to
		// ensure alignment of module identifiers.  Hence, they always have an empty trace.
		return rtrace.NewArrayModule[F](m.Descriptor().Name(), nil)
	case vm.PRIVATE_READ_ONLY_MEMORY, vm.PUBLIC_READ_ONLY_MEMORY:
		return post.ProcessAccessOnceMemory[W, F](m)
	case vm.PRIVATE_WRITE_ONCE_MEMORY, vm.PUBLIC_WRITE_ONCE_MEMORY:
		return post.ProcessAccessOnceMemory[W, F](m)
	default:
		return post.ProcessReadWriteMemory[W, F](m, p.field)
	}
}
