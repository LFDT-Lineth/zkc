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
package ir

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ModuleBuilder provides a mechanism to ease the construction of modules for
// use in schemas.  For example, it maintains a mapping from register names to
// their relevant indices.  It also provides a mechanism for constructing a
// register access based on the register name, etc.
//
// NOTE: overall, this interface has got somewhat out-of-hand and it would be
// useful to try and simplify it where possible.
type ModuleBuilder[F field.Element[F], T term.Expr[F, T]] interface {
	fmt.Stringer
	schema.ModuleView
	// AddAssignment adds a new assignment to this module.  Assignments are
	// responsible for computing the values of computed columns.
	AddAssignment(assignment schema.Assignment[F])
	// AddConstraint adds a new constraint to this module.
	AddConstraint(constraint schema.Constraint[F])
	// Assignments returns those assignments added to this module.
	Assignments() []schema.Assignment[F]
	// Constraints returns those constraints added to this module.
	Constraints() []schema.Constraint[F]
	// Id returns the module index of this module.
	Id() uint
	// IsExtern determines whether or not this is an external module or not.
	IsExtern() bool
	// NewRegister declares a new register within the module being built.  This will
	// panic if a register of the same name already exists.
	NewRegister(reg register.Register) register.Id
	// NewRegisters declares zero or more new registers within the module being
	// built.  This will panic if a register of the same name already exists.
	NewRegisters(registers ...register.Register) []register.Id
	// ZeroRegister returns an ID for the "zero register".  That is, a register
	// which is always zero.  If no such register exists already, one is
	// created.
	ConstRegister(constant uint8) register.Id
	// SetStaticContents sets the contents of this static reference table.  It
	// panics if invoked on a non-static module.
	SetStaticContents(contents [][]F)
	// StaticContents returns the static contents for a static reference table.  It
	// panics if invoked on a non-static module.
	StaticContents() (contents [][]F)
}

// ============================================================================
// Internal Module Builder
// ============================================================================

// NewModuleBuilder constructs a new builder for a module with the given name.
func NewModuleBuilder[F field.Element[F], T term.Expr[F, T]](name module.Name,
	mid schema.ModuleId, public, private, synthetic, static, native bool) ModuleBuilder[F, T] {
	//
	regmap := make(map[string]uint, 0)

	return &internalModuleBuilder[F, T]{name, mid, public, private, synthetic, static, native,
		regmap, nil, nil, nil, nil}
}

type internalModuleBuilder[F field.Element[F], T term.Expr[F, T]] struct {
	// Name of the module being constructed
	name module.Name
	// Id of this module
	moduleId schema.ModuleId
	// Indicates whether externally visible
	public bool
	// Indicates whether this is a private output or not
	private bool
	// Indicates whether this is a synthetic module or not
	synthetic bool
	// Indicates whether this is a static module or not
	static bool
	// Indicates whether this is a native module or not
	native bool
	// Maps register names (including aliases) to the register number.
	regmap map[string]uint
	// Registers declared for this module
	registers []register.Register
	// Constraints for this module
	constraints []schema.Constraint[F]
	// Assignments for computed registers
	assignments []schema.Assignment[F]
	// Static contents for ref tables
	staticContents [][]F
}

// AddAssignment implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) AddAssignment(assignment schema.Assignment[F]) {
	p.assignments = append(p.assignments, assignment)
}

// AddConstraint implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) AddConstraint(constraint schema.Constraint[F]) {
	p.constraints = append(p.constraints, constraint)
}

// Assignments implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) Assignments() []schema.Assignment[F] {
	return p.assignments
}

// Constraints implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) Constraints() []schema.Constraint[F] {
	return p.constraints
}

// Id implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) Id() uint {
	return p.moduleId
}

// IsExtern implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) IsExtern() bool {
	return false
}

// IsPublicOutput implementation for schema.ModuleView interface.
func (p *internalModuleBuilder[F, T]) IsPublicOutput() bool {
	return p.public
}

// IsPrivateOutput implementation for schema.ModuleView interface.
func (p *internalModuleBuilder[F, T]) IsPrivateOutput() bool {
	return p.private
}

// IsSynthetic implementation for schema.ModuleView interface.
func (p *internalModuleBuilder[F, T]) IsSynthetic() bool {
	return p.synthetic
}

// IsNative implementation for schema.ModuleView interface.  Modules built via
// this builder are never native; only the ZkC pipeline produces native
// modules and it does not go through this builder.
func (p *internalModuleBuilder[F, T]) IsNative() bool {
	return p.native
}

// IsStatic implementation for schema.ModuleView interface.  Modules built via
// this builder are never static; static modules are produced directly by the
// ZkC pipeline.
func (p *internalModuleBuilder[F, T]) IsStatic() bool {
	return p.static
}

// Width implementation for schema.ModuleView interface.
func (p *internalModuleBuilder[F, T]) Width() uint {
	return uint(len(p.registers))
}

// HasRegister implementation for register.Map interface.
func (p *internalModuleBuilder[F, T]) HasRegister(name string) (register.Id, bool) {
	// Lookup register associated with this name
	rid, ok := p.regmap[name]
	//
	return register.NewId(rid), ok
}

// Name implementation for register.Map interface.
func (p *internalModuleBuilder[F, T]) Name() module.Name {
	return p.name
}

// NewRegister implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) NewRegister(reg register.Register) register.Id {
	// Determine identifier
	id := uint(len(p.registers))
	// Sanity check
	if _, ok := p.regmap[reg.Name()]; ok {
		panic(fmt.Sprintf("register \"%s\" already declared", reg.Name()))
	}
	//
	p.registers = append(p.registers, reg)
	p.regmap[reg.Name()] = id
	//
	return register.NewId(id)
}

// NewRegisters implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) NewRegisters(registers ...register.Register) []register.Id {
	var ids = make([]register.Id, len(registers))
	//
	for i, r := range registers {
		ids[i] = p.NewRegister(r)
	}
	//
	return ids
}

// Register implementation for register.Map interface.
func (p *internalModuleBuilder[F, T]) Register(rid register.Id) register.Register {
	return p.registers[rid.Unwrap()]
}

// Registers implementation for register.Map interface.
func (p *internalModuleBuilder[F, T]) Registers() []register.Register {
	return p.registers
}

// RegisterAccessOf implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) RegisterAccessOf(name string, shift int) *term.RegisterAccess[F, T] {
	// Lookup register associated with this name
	var (
		rid = register.NewId(p.regmap[name])
		reg = p.Register(rid)
	)
	//
	return term.RawRegisterAccess[F, T](rid, reg.Width(), shift)
}

func (p *internalModuleBuilder[F, T]) String() string {
	return register.MapToString(p)
}

// ZeroRegister implementation for ModuleBuilder interface.
func (p *internalModuleBuilder[F, T]) ConstRegister(constant uint8) register.Id {
	var name = fmt.Sprintf("%d", constant)
	// Check whether register already exists
	if rid, ok := p.HasRegister(name); ok {
		return rid
	}
	// If not, create a new one.
	return p.NewRegister(register.NewConst(constant))
}

func (p *internalModuleBuilder[F, T]) SetStaticContents(contents [][]F) {
	if !p.IsStatic() {
		panic("cannot set static contents for non-static module")
	} else if p.staticContents != nil {
		panic("cannot reassign static-contents")
	}
	//
	p.staticContents = contents
}

func (p *internalModuleBuilder[F, T]) StaticContents() (contents [][]F) {
	if !p.IsStatic() {
		panic("cannot set static contents for non-static module")
	}
	//
	return p.staticContents
}
