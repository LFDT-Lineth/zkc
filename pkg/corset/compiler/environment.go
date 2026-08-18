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
package compiler

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// Environment provides an interface into the global scope which can be used for
// simply resolving column identifiers.
type Environment interface {
	// Register returns the name of the given module.
	Module(index uint) string
	// Module returns information about a given module, such as its module
	// identifier.
	ModuleOf(module string) uint
	// Register returns information about a given register, based on its index
	// (i.e. underlying MIR column identifier).
	Register(index uint) *Register
	// RegisterOf identifies the register (i.e. underlying (MIR) column) to
	// which a given source-level (i.e. corset) column is allocated.  This
	// expects an absolute path.
	RegisterOf(path *file.Path) uint
	// RegistersOf identifies the set of registers (i.e. underlying (MIR)
	// columns) associated with a given module.
	RegistersOf(module string) []uint
}

// GlobalEnvironment is a wrapper around a global scope.  The point, really, is
// to signal the change between a global scope whose columns have yet to be
// allocated, from an environment whose columns are allocated.
type GlobalEnvironment struct {
	modules []string
	// Info about moduleMap
	moduleMap map[string]uint
	// Registers (i.e. MIR-level columns)
	registers []Register
	// Map source-level columns to registers
	columnMap map[string]uint
}

// NewGlobalEnvironment constructs a new global environment from a global scope
// by allocating appropriate identifiers to all columns.
func NewGlobalEnvironment(root *ModuleScope) GlobalEnvironment {
	// Sanity Check
	if !root.IsRoot() {
		// Definitely should be unreachable.
		panic("root scope required")
	}
	// Construct top-level module list.
	modules := root.Flatten()
	// Initialise the environment
	env := GlobalEnvironment{nil, nil, nil, nil}
	env.initModules(modules)
	env.initColumnsAndRegisters(modules)
	// Done
	return env
}

// Module returns information about a given module, such as its module
// identifier.
func (p GlobalEnvironment) Module(mid uint) string {
	return p.modules[mid]
}

// ModuleOf returns the internal index of the given module.
func (p GlobalEnvironment) ModuleOf(module string) uint {
	return p.moduleMap[module]
}

// Register returns information about a given register, based on its index
// (i.e. underlying MIR column identifier).
func (p GlobalEnvironment) Register(index uint) *Register {
	return &p.registers[index]
}

// RegisterOf identifies the register (i.e. underlying (MIR) column) to
// which a given source-level (i.e. corset) column is allocated.
func (p GlobalEnvironment) RegisterOf(column *file.Path) uint {
	regId := p.columnMap[column.String()]
	// Lookup register info
	return regId
}

// RegistersOf identifies the set of registers (i.e. underlying (MIR)
// columns) associated with a given module.
func (p GlobalEnvironment) RegistersOf(module string) []uint {
	regs := make([]uint, 0)
	// Iterate all registers looking for those in the given module.
	for i, reg := range p.registers {
		if reg.Context.Module() == module {
			// match
			regs = append(regs, uint(i))
		}
	}
	// Done
	return regs
}

// ColumnsOf returns the set of registers allocated to a given column.
func (p GlobalEnvironment) ColumnsOf(register uint) []string {
	var columns []string
	//
	for col, reg := range p.columnMap {
		if reg == register {
			columns = append(columns, col)
		}
	}
	//
	return columns
}

// ===========================================================================
// Helpers
// ===========================================================================

// Module allocation is a simple process of allocating modules their specific
// identifiers.  This has to match exactly how the translator does it, otherwise
// there will be problems.
func (p *GlobalEnvironment) initModules(modules []*ModuleScope) {
	p.moduleMap = make(map[string]uint)
	// Allocate submodules one-by-one
	for _, m := range modules {
		if !m.Virtual() {
			name := m.path.String()
			mid := uint(len(p.modules))
			p.modules = append(p.modules, name)
			p.moduleMap[name] = mid
		}
	}
}

// Performs an initial register allocation which simply maps every column to a
// unique register.  The intention is that, subsequently, registers can be
// merged as necessary.
func (p *GlobalEnvironment) initColumnsAndRegisters(modules []*ModuleScope) {
	p.columnMap = make(map[string]uint)
	p.registers = make([]Register, 0)
	// Allocate input columns first.
	for _, m := range modules {
		for _, col := range m.DestructuredColumns() {
			if !col.Computed {
				p.allocateRegister(col)
			}
		}
	}
	// Allocate assignments second.
	for _, m := range modules {
		for _, col := range m.DestructuredColumns() {
			if col.Computed {
				p.allocateRegister(col)
			}
		}
	}
}

// Allocate a source-level column into this environment.  Since a source-level
// column can correspond to multiple underling registers, this can result in the
// allocation of a number of registers (based on the columns type).  For
// example, an array of length n will allocate n registers, etc.
func (p *GlobalEnvironment) allocateRegister(source Register) {
	regId := uint(len(p.registers))
	// Allocate register
	p.registers = append(p.registers, source)
	// Map column to register
	p.columnMap[source.Path.String()] = regId
}
