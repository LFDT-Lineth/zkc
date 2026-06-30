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
package descriptor

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Sanity check that *Function[W] implements Module[W]
var _ Module[word.Uint] = &Function[word.Uint]{}

// Function is a module describing an executable unit of a program.  In addition
// to the registers exposed by every module, it records execution-relevant
// properties such as whether the function is atomic (occupies a single trace
// line) or backed by a native circuit rather than bytecode instructions.
type Function[W word.Word[W]] struct {
	moduleBase[W]
	// Native indicates whether this function is backed by a native circuit
	// (i.e. declared with the @native annotation) rather than by the
	// instructions in code.
	native bool
	// Code defines the body of this function.
	vectors []bytecode.Vector[W]
}

// NewFunction constructs a new function with the given components.
func NewFunction[W word.Word[W]](name string, registers []Register[W], native bool,
	code []bytecode.Vector[W]) *Function[W] {
	return &Function[W]{newModuleBase(name, registers), native, code}
}

// IsOneLine determines whether or not this function contains a single "line"
// (i.e. exactly one bytecode vector).  If so, this implies that every instance
// of this function occupies exactly one line in the corresponding trace. This
// is important to distinguish, as certain optimisations can be applied to one
// line functions (e.g. no PC register is required).
func (p *Function[W]) IsOneLine() bool {
	return len(p.vectors) == 1
}

// IsNative reports whether this function is backed by a native circuit (i.e.
// declared with the @native annotation) rather than by the bytecode in its
// vectors.
func (p *Function[W]) IsNative() bool {
	return p.native
}

// PcWidth returns the bit width required for this function's program counter
// register, which must be able to index every code line plus the (one past the
// end) halt value.  Only meaningful for non-atomic functions, which are the
// only ones carrying a PC register.
func (p *Function[W]) PcWidth() uint {
	if p.IsOneLine() || p.IsNative() {
		panic("PC register unavailable on atomic or native function")
	}

	return bit.Width(uint(1 + len(p.vectors)))
}

// Vectors returns the bytecode vectors that define the body of this function.
// These vectors contain the executable bytecode instructions for the function.
func (p *Function[W]) Vectors() []bytecode.Vector[W] {
	return p.vectors
}
