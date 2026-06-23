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
package schema

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ModuleId abstracts the notion of a "module identifier"
type ModuleId = uint

// ModuleView provides access to certain structural information about a module.
type ModuleView interface {
	register.Map
	// Module name
	Name() module.Name
	// IsPublic indicates whether or not this module is externally visible.
	IsPublic() bool
	// IsSynthetic modules are generated during compilation, rather than being
	// provided by the user.
	IsSynthetic() bool
	// IsNative indicates whether this module corresponds to a function backed
	// by a native circuit (i.e. declared with the @native annotation in ZkC).
	// Only modules produced by the ZkC pipeline can ever be native; modules
	// from any other source always return false.
	IsNative() bool
	// IsStatic indicates whether this module represents a static reference
	// table whose contents are fixed at compile time.  Static modules carry
	// their data in StaticContents() (queried via the field-parameterised
	// schema.Module interface).
	IsStatic() bool
	// Returns the number of registers in this module.
	Width() uint
}

// Module represents a "table" within a schema which contains zero or more rows
// for a given set of registers.
type Module[F any] interface {
	ModuleView
	// AllowPadding determines whether the given module allows an initial
	// padding row, or not.
	AllowPadding() bool
	// Assignments returns an iterator over the assignments of this module.
	// These are the computations used to assign values to all computed columns
	// in this module.
	Assignments() iter.Iterator[Assignment[F]]
	// Constraints provides access to those constraints associated with this
	// module.
	Constraints() iter.Iterator[Constraint[F]]
	// Consistent applies a number of internal consistency checks.  Whilst not
	// strictly necessary, these can highlight otherwise hidden problems as an aid
	// to debugging.
	Consistent(fieldWidth uint, schema AnySchema[F]) []error
	// Keys returns the number n of key columns in this module.  Key columns are
	// always the first n columns in a module.  Such columns have the property
	// that they can be used in conjunction with Find.
	Keys() uint
	// StaticContents returns the contents of this module, assuming it
	// corresponds with a static reference table.  Each entry in the entries
	// array returned should have Width() elements and correspond to a row in
	// the static module.  NOTE: this will panic when IsStatic() is false (i.e.
	// since only static modules can have contents).
	StaticContents() (entries [][]F)
	// Substitute any matchined labelled constants within this module
	Substitute(map[string]F)
}

// ============================================================================
//
// ============================================================================

// Table provides a straightforward, reusable module implementation.  There is
// nothing fancy here: we simply have a set of registers, constraints and
// assignments.  A table is a field agnostic module with a simple strategy of
// subdividing registers "in place".  For example, suppose we have registers X
// and Y (in that order) where both are to be halfed.  Then, the result is X'0,
// X'1, Y'0. Y'1 (in that order).  Hence, predicting the new register indices is
// relatively straightforward.
type Table[F field.Element[F], C Constraint[F]] struct {
	name           module.Name
	padding        bool
	public         bool
	synthetic      bool
	native         bool
	static         bool
	keys           uint
	registers      []register.Register
	constraints    []C
	assignments    []Assignment[F]
	staticContents [][]F
}

// Init implementation for ir.InitModule interface.  The native flag indicates
// that this module corresponds to a function backed by a native circuit; only
// the ZkC pipeline should ever pass true.  The static flag indicates that this
// module is a static reference table whose contents are fixed at compile time
// and are populated separately via SetStaticContents.
func (p *Table[F, C]) Init(name module.Name, padding, public, synthetic, native, static bool,
	keys uint) *Table[F, C] {
	return &Table[F, C]{name, padding, public, synthetic, native, static, keys, nil, nil, nil, nil}
}

// Assignments provides access to those assignments defined as part of this
// table.
func (p *Table[F, C]) Assignments() iter.Iterator[Assignment[F]] {
	return iter.NewArrayIterator(p.assignments)
}

// Constraints provides access to those constraints associated with this
// module.
func (p *Table[F, C]) Constraints() iter.Iterator[Constraint[F]] {
	arrIter := iter.NewArrayIterator(p.constraints)
	return iter.NewCastIterator[C, Constraint[F]](arrIter)
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p *Table[F, C]) Consistent(fieldWidth uint, schema AnySchema[F]) []error {
	var errors []error
	// Check constraints
	for _, c := range p.constraints {
		errors = append(errors, c.Consistent(schema)...)
	}
	// Check assignments
	for _, a := range p.assignments {
		errors = append(errors, a.Consistent(schema)...)
	}
	// Done
	return errors
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p *Table[F, C]) HasRegister(name string) (register.Id, bool) {
	for i := range p.Width() {
		if p.registers[i].Name() == name {
			return register.NewId(i), true
		}
	}
	// Fail
	return register.UnusedId(), false
}

// Name returns the module name.
func (p *Table[F, C]) Name() module.Name {
	return p.name
}

// Keys implementation of Module interface.
func (p *Table[F, C]) Keys() uint {
	return p.keys
}

// AllowPadding determines whether the given module supports padding at the
// beginning of the module.  This is necessary because legacy modules expect an
// initial padding row, and allow defensive padding as well.
func (p *Table[F, C]) AllowPadding() bool {
	return p.padding
}

// IsPublic identifies whether or not this module is externally visible.
func (p *Table[F, C]) IsPublic() bool {
	return p.public
}

// IsSynthetic modules are generated during compilation, rather than being
// provided by the user.
func (p *Table[F, C]) IsSynthetic() bool {
	return p.synthetic
}

// IsNative reports whether this module corresponds to a function backed by
// a native circuit (i.e. declared with the @native annotation in ZkC).
func (p *Table[F, C]) IsNative() bool {
	return p.native
}

// IsStatic reports whether this module is a static reference table whose
// contents are fixed at compile time.
func (p *Table[F, C]) IsStatic() bool {
	return p.static
}

// StaticContents returns the contents of this static reference table.  It
// panics if invoked on a non-static module, since no contents are stored in
// that case.
func (p *Table[F, C]) StaticContents() [][]F {
	if !p.static {
		panic(fmt.Sprintf("module \"%s\" is not static", p.name))
	}
	//
	return p.staticContents
}

// RawAssignments provides raw access to those assignments defined as part of this
// table.
func (p *Table[F, C]) RawAssignments() []Assignment[F] {
	return p.assignments
}

// RawConstraints provides raw access to those constraints associated with this
// module.
func (p *Table[F, C]) RawConstraints() []C {
	return p.constraints
}

// Register returns the given register in this table.
func (p *Table[F, C]) Register(id register.Id) register.Register {
	return p.registers[id.Unwrap()]
}

// Registers returns an iterator over the underlying registers of this schema.
// Specifically, the index of a register in this array is its register index.
func (p *Table[F, C]) Registers() []register.Register {
	return p.registers
}

// Substitute any matchined labelled constants within this module
func (p *Table[F, C]) Substitute(mapping map[string]F) {
	for _, c := range p.assignments {
		c.Substitute(mapping)
	}
	//
	for _, c := range p.constraints {
		c.Substitute(mapping)
	}
}

// Width returns the number of registers in this Table.
func (p *Table[F, C]) Width() uint {
	return uint(len(p.registers))
}

func (p *Table[F, C]) String() string {
	return register.MapToString(p)
}

// ConstRegister implementation for register.ConstMap interface
func (p *Table[F, C]) ConstRegister(constant uint8) register.Id {
	var (
		name  = fmt.Sprintf("%d", constant)
		nregs = uint(len(p.registers))
	)
	// Check whether register already exists
	if rid, ok := p.HasRegister(name); ok {
		return rid
	}
	// Allocate constant register
	p.registers = append(p.registers, register.NewConst(constant))
	//
	return register.NewId(nregs)
}

// ============================================================================
// Mutators
// ============================================================================

// AddAssignments adds a new assignments to this table.
func (p *Table[F, C]) AddAssignments(assignments ...Assignment[F]) {
	p.assignments = append(p.assignments, assignments...)
}

// AddConstraints adds new constraints to this table.
func (p *Table[F, C]) AddConstraints(constraints ...C) {
	p.constraints = append(p.constraints, constraints...)
}

// AddRegisters adds new registers to this table.
func (p *Table[F, C]) AddRegisters(registers ...register.Register) {
	// Add registers
	p.registers = append(p.registers, registers...)
}

// SetStaticContents sets the contents of this static reference table.  It
// panics if invoked on a non-static module.
func (p *Table[F, C]) SetStaticContents(contents [][]F) {
	if !p.static {
		panic(fmt.Sprintf("module \"%s\" is not static", p.name))
	}
	//
	p.staticContents = contents
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (p *Table[F, M]) GobEncode() (data []byte, err error) {
	var buffer bytes.Buffer
	//
	gobEncoder := gob.NewEncoder(&buffer)
	// Name
	if err := gobEncoder.Encode(p.name.Name); err != nil {
		return nil, err
	}
	// Multiplier
	if err := gobEncoder.Encode(p.name.Multiplier); err != nil {
		return nil, err
	}
	// Padding
	if err := gobEncoder.Encode(p.padding); err != nil {
		return nil, err
	}
	// Native
	if err := gobEncoder.Encode(p.native); err != nil {
		return nil, err
	}
	// Static
	if err := gobEncoder.Encode(p.static); err != nil {
		return nil, err
	}
	// Static contents
	if err := gobEncoder.Encode(p.staticContents); err != nil {
		return nil, err
	}
	// registers
	if err := gobEncoder.Encode(p.registers); err != nil {
		return nil, err
	}
	// constraints
	if err := gobEncoder.Encode(p.constraints); err != nil {
		return nil, err
	}
	// assignments
	if err := gobEncoder.Encode(p.assignments); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *Table[F, M]) GobDecode(data []byte) error {
	buffer := bytes.NewBuffer(data)
	gobDecoder := gob.NewDecoder(buffer)
	// Name
	if err := gobDecoder.Decode(&p.name.Name); err != nil {
		return err
	}
	// Multiplier
	if err := gobDecoder.Decode(&p.name.Multiplier); err != nil {
		return err
	}
	// Padding
	if err := gobDecoder.Decode(&p.padding); err != nil {
		return err
	}
	// Native
	if err := gobDecoder.Decode(&p.native); err != nil {
		return err
	}
	// Static
	if err := gobDecoder.Decode(&p.static); err != nil {
		return err
	}
	// Static contents
	if err := gobDecoder.Decode(&p.staticContents); err != nil {
		return err
	}
	// Registers
	if err := gobDecoder.Decode(&p.registers); err != nil {
		return err
	}
	// Constraints
	if err := gobDecoder.Decode(&p.constraints); err != nil {
		return err
	}
	// Assignments
	if err := gobDecoder.Decode(&p.assignments); err != nil {
		return err
	}
	// Success!
	return nil
}
