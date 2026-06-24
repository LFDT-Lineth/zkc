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
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FromRegisters converts an array of schema registers into an array of register
// descriptors.
func FromRegisters[W word.Word[W]](registers ...register.Register) []Register[W] {
	var regs = make([]Register[W], len(registers))
	//
	for i, r := range registers {
		var (
			padding  W
			bitwidth util.Option[uint]
		)
		// Determine bitwidth (if applicable)
		if !r.IsNative() {
			bitwidth = util.Some(r.Width())
		}
		//
		regs[i] = Register[W]{
			r.Kind(),
			r.Name(),
			bitwidth,
			padding,
		}
	}
	//
	return regs
}

// Register represents an individual register in a module that, eventually, will
// be mapped to one (or more) columns in the trace.  Likewise, a single register
// can end up being mapped across multiple columns as a result of register
// splitting  to ensure field agnosticity. Hence, why they are referred to as
// registers rather than columns --- they are similar, but not identical,
// concepts.
type Register[W any] struct {
	// Kind of register (input / output)
	kind register.Type
	// Given name of this register.
	name string
	// Bitwidth holds the bitwidth of word registers, otherwise is empty (for
	// field registers).
	bitwidth util.Option[uint]
	// Determines what value will be used to pad this register.
	padding W
}

// Bitwidth determines the bitwidth of this register (if applicable).  Observe
// that native registers have no explicit bitwidth and, hence, this simply
// returns none in such cases.
func (p Register[W]) Bitwidth() util.Option[uint] {
	return p.bitwidth
}

// IsInput determines whether or not this is an input register
func (p Register[W]) IsInput() bool {
	return p.kind == register.INPUT_REGISTER
}

// IsInputOutput determines whether or not this is an input or output register
func (p Register[W]) IsInputOutput() bool {
	return p.IsInput() || p.IsOutput()
}

// IsNative determines whether or not this is a native register
func (p Register[W]) IsNative() bool {
	return !p.bitwidth.HasValue()
}

// IsOutput determines whether or not this is an output register
func (p Register[W]) IsOutput() bool {
	return p.kind == register.OUTPUT_REGISTER
}

// IsComputed determines whether or not this is a computed register.  Observer
// that "zero" registers are included in this, since they are neither input nor
// output registers.
func (p Register[W]) IsComputed() bool {
	return p.kind == register.COMPUTED_REGISTER
}

// Name returns the  name of this register
func (p Register[W]) Name() string {
	return p.name
}

// Padding returns the padding for this register
func (p Register[W]) Padding() W {
	return p.padding
}
