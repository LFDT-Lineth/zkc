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
package register

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// LimbsMap provides a high-level mapping of all registers before and
// after subdivision occurs within a given module.  That is, it maps a given
// register to those limbs into which it was subdivided.
type LimbsMap interface {
	Map
	// Field returns the underlying field configuration used for this mapping.
	// This includes the field bandwidth (i.e. number of bits available in
	// underlying field) and the maximum register width (i.e. width at which
	// registers are capped).
	Field() field.Config
	// Limbs identifies the limbs into which a given register is divided.
	// Observe that limbs are ordered by their position in the original
	// register.  In particular, the first limb (i.e. at index 0) is always
	// least significant limb, and the last always most significant.
	LimbIds(Id) []LimbId
	// Limbs returns information about a given limb (i.e. a register which
	// exists after the split).
	Limb(LimbId) Limb
	// Limbs returns all limbs in the mapping.
	Limbs() []Limb
	// LimbsMap returns a register map for the limbs themselves.  This is useful
	// where we need a register map over the limbs, rather than the original
	// registers.
	LimbsMap() Map
}

// NewLimbsMap constructs an appropriate register map for a given module
// and parameter combination.
func NewLimbsMap[F any](field field.Config, module Map) limbsMap {
	var (
		regs    = module.Registers()
		limbs   []Limb
		mapping = make([][]LimbId, len(regs))
		limbId  = uint(0)
	)
	// Split up limbs
	for i, r := range regs {
		ls := SplitIntoLimbs(field.RegisterWidth, r)
		limbs = append(limbs, ls...)
		// build mapping
		m := make([]Id, len(ls))
		//
		for i := 0; i != len(m); i++ {
			m[i] = NewId(limbId)
			limbId++
		}
		// Assign mapping
		mapping[i] = m
	}
	// Done
	return limbsMap{
		module.Name(),
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
type limbsMap struct {
	// Name of the module to which this mapping corresponds
	name trace.ModuleName
	// Field configuration in play
	field field.Config
	// Set of registers in the original schema (i.e. as they were before the
	// split)
	registers []Register
	// Set of "limbs" (i.e registers) in the split schema.
	limbs []Limb
	// Mapping for each register above to its corresponding set of limbs.
	mapping [][]LimbId
}

// Field implementation for register.Map interface
func (p limbsMap) Field() field.Config {
	return p.field
}

// Limbs implementation for the register.Map interface
func (p limbsMap) LimbIds(reg Id) []LimbId {
	return p.mapping[reg.Unwrap()]
}

// Limb implementation for the register.Map interface
func (p limbsMap) Limb(reg LimbId) Limb {
	return p.limbs[reg.Unwrap()]
}

// Limbs implementation for the register.Map interface
func (p limbsMap) Limbs() []Limb {
	return p.limbs
}

// LimbsMap implementation for the register.Map interface
func (p limbsMap) LimbsMap() Map {
	return limbsMap{
		p.name, p.field, p.limbs, nil, nil,
	}
}

// Name implementation for register.Map interface
func (p limbsMap) Name() trace.ModuleName {
	return p.name
}

// RegisterOf determines a register's ID based on its name.
func (p limbsMap) RegisterOf(name string) Id {
	for i, reg := range p.registers {
		if reg.Name() == name {
			return NewId(uint(i))
		}
	}
	//
	panic(fmt.Sprintf("unknown register \"%s\"", name))
}

// HasRegister implementation for RegisterMap interface.
func (p limbsMap) HasRegister(name string) (Id, bool) {
	for i, reg := range p.registers {
		if reg.Name() == name {
			return NewId(uint(i)), true
		}
	}
	//
	return UnusedId(), false
}

// Register implementation for RegisterMap interface.
func (p limbsMap) Register(rid Id) Register {
	return p.registers[rid.Unwrap()]
}

// Registers implementation for RegisterMap interface.
func (p limbsMap) Registers() []Register {
	return p.registers
}

func (p limbsMap) String() string {
	return LimbsMapToString(p)
}

// ============================================================================
// Helpers
// ============================================================================

// WidthsOfLimbs returns the limb bitwidths corresponding to a given set of
// identifiers.
func WidthsOfLimbs(mapping LimbsMap, lids []LimbId) []uint {
	var (
		widths []uint = make([]uint, len(lids))
	)
	//
	for i, lid := range lids {
		widths[i] = mapping.Limb(lid).Width()
	}
	//
	return widths
}

// LimbsOf returns those limbs corresponding to a given set of identifiers.
func LimbsOf(mapping LimbsMap, lids []LimbId) []Limb {
	var (
		limbs []Limb = make([]Limb, len(lids))
	)
	//
	for i, lid := range lids {
		limbs[i] = mapping.Limb(lid)
	}
	//
	return limbs
}

// ApplyLimbsMap applies a given mapping to a set of registers producing a
// corresponding set of limbs.  In essence, each register is convert to its
// limbs in turn, and these are all appended together in order of ococurence.
func ApplyLimbsMap(mapping LimbsMap, rids ...Id) []LimbId {
	var limbs []LimbId
	//
	for _, rid := range rids {
		limbs = append(limbs, mapping.LimbIds(rid)...)
	}
	//
	return limbs
}

// LimbsMapToString provides a default method for converting a register
// limbs map into a simple string representation.
func LimbsMapToString(p LimbsMap) string {
	var builder strings.Builder
	//
	builder.WriteString("{")
	builder.WriteString(p.Name().String())
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
