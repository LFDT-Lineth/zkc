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
	"bytes"
	"encoding/gob"

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
	// Kind records the execution-relevant properties of this function.
	kind FunctionKind
	// Effects records the module ids of the memories this function is permitted
	// to access (its declared "<...>" effects).
	effects []ModuleId
	// Code defines the body of this function.
	vectors []bytecode.Vector[W]
}

// NewFunction constructs a new function with the given components.
func NewFunction[W word.Word[W]](name string, registers []Register[W], kind FunctionKind,
	effects []ModuleId, code []bytecode.Vector[W]) *Function[W] {
	return &Function[W]{newModuleBase(name, registers), kind, effects, code}
}

// Kind returns the execution kind of this function.
func (p *Function[W]) Kind() FunctionKind {
	return p.kind
}

// Effects returns the module ids of the memories this function is permitted
// to access.
func (p *Function[W]) Effects() []ModuleId {
	return p.effects
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
	return p.kind.IsNative()
}

// HasUnsafeArgs reports whether calls may supply arguments which are undefined
// on some paths reaching the call.
func (p *Function[W]) HasUnsafeArgs() bool {
	return p.kind.AllowsUnsafeArgs()
}

// IsFunction identifies this module as a callable function.
func (p *Function[W]) IsFunction() bool {
	return true
}

// IsMemory identifies this module as a function rather than a memory.
func (p *Function[W]) IsMemory() bool {
	return false
}

// IsReadOnly is false because memory access modes do not apply to functions.
func (p *Function[W]) IsReadOnly() bool {
	return false
}

// IsWriteOnly is false because memory access modes do not apply to functions.
func (p *Function[W]) IsWriteOnly() bool {
	return false
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

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode marshals this function.  The embedded moduleBase and the vectors
// hold unexported fields, so an explicit encoding is required.  The bytecode
// implementations reachable through the vectors are registered with gob first,
// so that the Bytecode interface values they contain can be encoded.
//
// nolint
func (p *Function[W]) GobEncode() ([]byte, error) {
	bytecode.RegisterGobTypes[W]()
	//
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.name); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.registers); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(&p.kind); err != nil {
		return nil, err
	}
	// A local, so gob accepts a nil slice.
	effects := p.effects
	if err := gobEncoder.Encode(&effects); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.vectors); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Function[W]) GobDecode(data []byte) error {
	bytecode.RegisterGobTypes[W]()
	//
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		name       string
		registers  []Register[W]
	)
	//
	if err := gobDecoder.Decode(&name); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&registers); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.kind); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.effects); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.vectors); err != nil {
		return err
	}
	// Reconstruct the module base (which recomputes the input / output counts).
	p.moduleBase = newModuleBase(name, registers)
	//
	return nil
}
