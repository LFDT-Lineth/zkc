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

// Vectors returns the bytecode vectors that define the body of this function.
// These vectors contain the executable bytecode instructions for the function.
func (p *Function[W]) Vectors() []bytecode.Vector[W] {
	return p.vectors
}
