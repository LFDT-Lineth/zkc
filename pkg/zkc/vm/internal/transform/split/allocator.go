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

package split

import (
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode is a useful alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// RegisterId provides a useful alias
type RegisterId = descriptor.RegisterId

// Allocator extends a register mapping with the ability to allocate new
// registers as necessary.  This is useful, for example,  in the context of
// register splitting for introducing new carry registers.
type Allocator[W any] interface {
	descriptor.RegisterMap[W]
	// Allocate a fresh register of the given width within the target module.
	// This is presumed to be a computed register, and automatically assigned a
	// unique name.  No assignment is included for the allocated register
	Allocate(prefix string, width uint) RegisterId
}

type registerAllocator[W any] struct {
	name      string
	registers []descriptor.Register[W]
}

// NewAllocator converts a mapping into a full allocator simply by wrapping the
// two fields.
func NewAllocator[W any](mapping descriptor.RegisterMap[W]) Allocator[W] {
	var (
		registers = slices.Clone(mapping.Registers())
	)
	//
	return &registerAllocator[W]{mapping.Name(), registers}
}

// Name implementation for the RegisterAllocator interface
func (p *registerAllocator[W]) Name() string {
	return p.name
}

// Allocate implementation for the RegisterAllocator interface
func (p *registerAllocator[W]) Allocate(prefix string, width uint) RegisterId {
	var (
		// Determine index for new register
		index = uint(len(p.registers))
		// Determine unique name for new register
		name = fmt.Sprintf("%s$%d", prefix, index)
		// Default padding (for now)
		zero W
	)
	// Allocate a new computed register.
	p.registers = append(p.registers,
		descriptor.NewRegister(register.COMPUTED_REGISTER, name, util.Some(width), zero))
	//
	return util.Cast[RegisterId](index)
}

func (p *registerAllocator[W]) HasRegister(name string) util.Option[RegisterId] {
	for i, r := range p.registers {
		if r.Name() == name {
			return util.Some(util.Cast[RegisterId](uint(i)))
		}
	}
	//
	return util.None[RegisterId]()
}

func (p *registerAllocator[W]) Register(id RegisterId) descriptor.Register[W] {
	return p.registers[id]
}

func (p *registerAllocator[W]) Registers() []descriptor.Register[W] {
	return p.registers
}

func (p *registerAllocator[W]) Width() uint {
	return uint(len(p.registers))
}
