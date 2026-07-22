// Copyright Consensys Software Inc.
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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Limb is simply a regsiter descriptor being used specifically to represent a
// limb of some register after splitting.
type Limb[W any] = Register[W]

// LimbId is a register ID used to represent a "limb".
type LimbId = RegisterId

// LimbsMap provides a high-level mapping of all registers before and
// after subdivision occurs within a given module.  That is, it maps a given
// register to those limbs into which it was subdivided.
type LimbsMap[W any] interface {
	RegisterMap[W]
	// BandWidth returns the maximum number of bits representable in the underlying
	// machine word.
	BandWidth() uint
	// RegisterWidth returns the maximum number of bits any register is permitted to
	// have.
	RegisterWidth() uint
	// Field returns the underlying configuration of the target field.
	Field() field.Config
	// Limbs identifies the limbs into which a given register is divided.
	// Observe that limbs are ordered by their position in the original
	// register.  In particular, the first limb (i.e. at index 0) is always
	// least significant limb, and the last always most significant.
	LimbIds(RegisterId) []LimbId
	// Limbs returns information about a given limb (i.e. a register which
	// exists after the split).
	Limb(LimbId) Limb[W]
	// Limbs returns all limbs in the mapping.
	Limbs() []Limb[W]
	// LimbsRegisterMap returns a register map for the limbs themselves.  This is useful
	// where we need a register map over the limbs, rather than the original
	// registers.
	LimbsRegisterMap() RegisterMap[W]
}

// NewLimbsMap constructs a new limbs map from a given register mapping
// according to a given field configuration.
func NewLimbsMap[W word.Word[W]](word word.Config, field field.Config, m RegisterMap[W]) LimbsMap[W] {
	var (
		regs    = m.Registers()
		limbs   []Limb[W]
		mapping = make([][]LimbId, len(regs))
		limbId  = uint(0)
	)
	// Split up limbs
	for i, r := range regs {
		var (
			// Initial split with least significant limb first
			ls = SplitIntoLimbs(word.RegisterWidth, r)
			// Reverse split so most significant comes first
			rls = array.Reverse(ls)
		)
		//
		limbs = append(limbs, rls...)
		// build mapping
		m := make([]RegisterId, len(ls))
		//
		for i := 0; i != len(m); i++ {
			m[i] = util.Cast[RegisterId](limbId)
			limbId++
		}
		// Assign mapping (reversed to match rls)
		mapping[i] = array.Reverse(m)
	}
	// Done
	return limbsMap[W]{
		m.Name(),
		word,
		field,
		regs,
		limbs,
		mapping,
	}
}

// ============================================================================
// LimbMap
// ============================================================================

// limbsMap provides a mapping from registers from the original schema to
// registers (referred to as limbs) in the split schema.   In some cases, there
// may be only one limb matching the original register above exactly (i.e. when
// the register width was already below the cutoff); in other cases, there can
// be many limbs for a single register above.  It should always be the case that
// the total width of limbs matches that of the original register.  Furthermore,
// if the original register was computed, then the limbs should be also, etc.
type limbsMap[W any] struct {
	// Name of the module to which this mapping corresponds
	name string
	// word configuration in play
	word word.Config
	// field configuration in play
	field field.Config
	// Set of registers in the original schema (i.e. as they were before the
	// split)
	registers []Register[W]
	// Set of "limbs" (i.e registers) in the split schema.
	limbs []Limb[W]
	// Mapping for each register above to its corresponding set of limbs.
	mapping [][]LimbId
}

// RegisterWidth returns the maximum number of bits any register is permitted to
// have.
func (p limbsMap[W]) RegisterWidth() uint {
	return p.word.RegisterWidth
}

// BandWidth returns the maximum number of bits representable in the underlying
// machine word.
func (p limbsMap[W]) BandWidth() uint {
	return p.word.BandWidth
}

// Field returns the configuration of the underlying field.
func (p limbsMap[W]) Field() field.Config {
	return p.field
}

// Limbs implementation for the LimbsMap interface
func (p limbsMap[W]) LimbIds(reg RegisterId) []LimbId {
	return p.mapping[reg]
}

// Limb implementation for the LimbsMap interface
func (p limbsMap[W]) Limb(reg LimbId) Limb[W] {
	return p.limbs[reg]
}

// Limbs implementation for the LimbsMap interface
func (p limbsMap[W]) Limbs() []Limb[W] {
	return p.limbs
}

// LimbsMap implementation for the LimbsMap interface
func (p limbsMap[W]) LimbsRegisterMap() RegisterMap[W] {
	return limbsMap[W]{
		p.name, p.word, p.field, p.limbs, nil, nil,
	}
}

// Name implementation for LimbsMap interface
func (p limbsMap[W]) Name() string {
	return p.name
}

// HasRegister implementation for RegisterMap interface.
func (p limbsMap[W]) HasRegister(name string) util.Option[RegisterId] {
	for i, reg := range p.registers {
		if reg.Name() == name {
			return util.Some(util.Cast[RegisterId](uint(i)))
		}
	}
	//
	return util.None[RegisterId]()
}

// Register implementation for RegisterMap interface.
func (p limbsMap[W]) Register(rid RegisterId) Register[W] {
	return p.registers[rid]
}

// Registers implementation for RegisterMap interface.
func (p limbsMap[W]) Registers() []Register[W] {
	return p.registers
}

func (p limbsMap[W]) String() string {
	var builder strings.Builder
	//
	builder.WriteString("{")
	builder.WriteString(p.Name())
	builder.WriteString(":")
	//
	for i, r := range p.Registers() {
		if i != 0 {
			builder.WriteString(",")
		}
		//
		builder.WriteString(r.Name())
		builder.WriteString("=>")
		//
		mapping := p.Limbs()
		//
		for j := len(mapping); j > 0; {
			if j != len(mapping) {
				builder.WriteString("::")
			}
			//
			j = j - 1
			//
			builder.WriteString(mapping[j].Name())
		}
	}
	//
	builder.WriteString("}")
	//
	return builder.String()
}

// Width implementation for RegisterMap interface.
func (p limbsMap[W]) Width() uint {
	return uint(len(p.registers))
}
