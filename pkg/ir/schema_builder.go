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

	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
)

// BuildableModule embodies the notion of a module which can be initialised from
// the various required components.  This provides a useful way for constructing
// modules once all the various pieces of information have been finalised.
type BuildableModule[F any, C schema.Constraint[F], M any] interface {
	Init(name module.Name, padding, public, synthetic, native, static bool, keys uint) M
	// Add one or more assignments to this buildable module
	AddAssignments(assignments ...schema.Assignment[F])
	// Add one or more constraints to this buildable module
	AddConstraints(constraints ...C)
	// Add one or more registers to this buildable module.
	AddRegisters(registers ...register.Register)
	// Set contents for static module
	SetStaticContents(contents [][]F)
}

// BuildSchema builds all modules defined within a give SchemaBuilder instance.
func BuildSchema[M BuildableModule[F, C, M], F field.Element[F], C schema.Constraint[F], T term.Expr[F, T]](
	p SchemaBuilder[F, C, T]) []M {
	//
	var modules = make([]M, len(p.modules))
	//
	for i, m := range p.modules {
		modules[i] = BuildModule[F, C, T, M](m)
	}
	//
	return modules
}

// BuildModule builds a module from a given ModuleBuilder instance.
func BuildModule[F field.Element[F], C schema.Constraint[F], T term.Expr[F, T], M BuildableModule[F, C, M]](
	m ModuleBuilder[F, C, T]) M {
	//
	var module M
	// Build it
	module = module.Init(m.Name(), m.AllowPadding(), m.IsPublic(), m.IsSynthetic(), m.IsNative(),
		m.IsStatic(), m.Keys())
	module.AddRegisters(m.Registers()...)
	module.AddAssignments(m.Assignments()...)
	module.AddConstraints(m.Constraints()...)
	// set static contents (if applicable)
	if m.IsStatic() {
		module.SetStaticContents(m.StaticContents())
	}
	// Done
	return module
}

// SchemaBuilder is a mechanism for constructing mixed schemas which attempts to
// simplify the problem of mapping source-level names to e.g. module-specific
// register indexes.
type SchemaBuilder[F field.Element[F], C schema.Constraint[F], T term.Expr[F, T]] struct {
	// Modmap maps modules identifers to modules
	modmap map[module.Name]uint
	// Externs represent modules which have already been constructed.  These
	// will be given the lower module identifiers, since they are already
	// packaged and, hence, we must avoid breaking thein linkage.
	externs []register.ConstMap
	// Modules being constructed
	modules []ModuleBuilder[F, C, T]
}

// NewSchemaBuilder constructs a new schema builder with a given number of
// externally defined modules.  Such modules are allocated module indices first.
func NewSchemaBuilder[F field.Element[F], C schema.Constraint[F], T term.Expr[F, T], E register.ConstMap](externs ...E,
) SchemaBuilder[F, C, T] {
	var (
		modmap   = make(map[module.Name]uint, 0)
		nexterns = make([]register.ConstMap, len(externs))
	)
	// Initialise module map
	for i, m := range externs {
		// Quick sanity check
		if _, ok := modmap[m.Name()]; ok {
			panic(fmt.Sprintf("duplicate module \"%s\" detected", m.Name()))
		}
		//
		modmap[m.Name()] = uint(i)
	}
	// Convert externs
	for i, m := range externs {
		nexterns[i] = m
	}
	//
	return SchemaBuilder[F, C, T]{modmap, nexterns, nil}
}

// NewModule constructs a new, empty module and returns its unique module
// identifier.
func (p *SchemaBuilder[F, C, T]) NewModule(name module.Name, padding, public, synthetic, static, native bool,
	keys uint) uint {
	var mid = uint(len(p.externs) + len(p.modules))
	// Sanity check this module is not already declared
	if _, ok := p.modmap[name]; ok {
		panic(fmt.Sprintf("module \"%s\" already declared", name))
	}
	//
	p.modules = append(p.modules, NewModuleBuilder[F, C, T](name, mid, padding, public, synthetic, static, native, keys))
	p.modmap[name] = mid
	//
	return mid
}

// Externs provides direct access to the external modules.
func (p *SchemaBuilder[F, C, T]) Externs() []register.ConstMap {
	return p.externs
}

// HasModule checks whether a moduleregister of the given name exists already
// and,if so, returns its index.
func (p *SchemaBuilder[F, C, T]) HasModule(name module.Name) (uint, bool) {
	// Lookup module associated with this name
	mid, ok := p.modmap[name]
	// That's it.
	return mid, ok
}

// Module returns the builder for the given module based on its index.
func (p *SchemaBuilder[F, C, T]) Module(mid uint) ModuleBuilder[F, C, T] {
	var n uint = uint(len(p.externs))
	// Sanity check
	if mid < n {
		return NewExternModuleBuilder[F, C, T](mid, p.externs[mid])
	}
	//
	return p.modules[mid-n]
}

// ModuleOf returns the builder for the given module based on its name.
func (p *SchemaBuilder[F, C, T]) ModuleOf(name module.Name) ModuleBuilder[F, C, T] {
	id, ok := p.modmap[name]
	//
	if ok {
		return p.Module(id)
	}
	//
	return nil
}
