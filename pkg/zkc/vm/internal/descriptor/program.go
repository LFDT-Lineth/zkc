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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Program represents a bytecode program.  This representation is useful for
// transforming bytecode programs, generating code or constraints from bytecode
// programs and analysing bytecode programs.  However, it is not good for
// executing bytecode programs (and, for that, compiled programs are better).
type Program[W word.Word[W]] struct {
	// identifies the function representing the entrypoint of this program.
	// This function cannot accept have any parameters or returns.
	entrypoint uint
	// modules declared within this (uncompiled) program.
	modules []Module[W]
}

// NewProgram creates a new program descriptor.
func NewProgram[W word.Word[W]](entrypoint uint, modules ...Module[W]) Program[W] {
	return Program[W]{entrypoint, modules}
}

// EntryPoint identies the module id of the entry function.
func (p *Program[W]) EntryPoint() uint {
	return p.entrypoint
}

// AddCheckPoint returns a copy of this program in which all calls to the target
// function are "checkpointing calls" (i.e. have their CheckPoint field set).
// Switching a call to checkpointing only swaps its ENTER opcode (ENTER_n =>
// ENTERCP_n), which is width-preserving; hence every instruction offset --
// along with the symbol and chunk side-tables -- is unaffected, and the
// returned program shares those tables with the original.
func (p *Program[W]) AddCheckPoint(fid uint16) Program[W] {
	return mapProgram(*p, addCheckPoint[W](fid))
}

// EnvironmentOf returns an environment for the given module.  This is useful
// for working with bytecodes enclosed by that module, etc.
func (p Program[W]) EnvironmentOf(mid uint16) bytecode.Environment {
	return &moduleEnvironment[W]{
		mid,
		p.modules,
	}
}

// HasModule returns the identifier for the module with the given name, or returns
// false if no such module exists.
func (p Program[W]) HasModule(name string) (uint16, bool) {
	var mid = array.FindMatching(p.modules, func(m Module[W]) bool {
		return m.Name() == name
	})
	//
	return uint16(mid), mid <= math.MaxUint16
}

// Prune away all functions which cannot be reached from the entrypoint.
func (p *Program[W]) Prune() Program[W] {
	panic("todo")
}

// Modules returns an array of modules over which this
func (p *Program[W]) Modules() []Module[W] {
	return p.modules
}

// ============================================================================
// Module Environment
// ============================================================================

type moduleEnvironment[W word.Word[W]] struct {
	module  uint16
	modules []Module[W]
}

// Name returns the name of the enclosing function.
func (p moduleEnvironment[W]) Name() string {
	return p.modules[p.module].Name()
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p moduleEnvironment[W]) HasModule(name string) (ModuleId, bool) {
	for i, r := range p.modules {
		if r.Name() == name {
			return ModuleId(i), true
		}
	}
	// Failed
	return math.MaxUint16, false
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p moduleEnvironment[W]) HasRegister(name string) (RegisterId, bool) {
	return p.modules[p.module].HasRegister(name)
}

// Register returns the ith register used in this module.
func (p moduleEnvironment[W]) Module(id ModuleId) bytecode.ModuleInfo {
	return p.modules[id]
}

// Register returns the ith register used in this module.
func (p moduleEnvironment[W]) Register(id RegisterId) bytecode.RegisterInfo {
	return p.modules[p.module].Register(id)
}

// ============================================================================
// Map
// ============================================================================

func mapProgram[W word.Word[W]](p Program[W], fn func(uint, bytecode.Bytecode[W]) []Bytecode[W]) Program[W] {
	var modules = make([]Module[W], len(p.modules))
	//
	for i, m := range p.modules {
		modules[i] = mapModule(m, fn)
	}
	//
	return Program[W]{p.entrypoint, modules}
}

func mapModule[W word.Word[W]](m Module[W], fn func(uint, bytecode.Bytecode[W]) []Bytecode[W]) Module[W] {
	switch m := m.(type) {
	case *Memory[W]:
		return m
	case *Function[W]:
		return mapFunction(*m, fn)
	default:
		panic(fmt.Sprintf("unknown descriptor \"%s\"", m.Name()))
	}
}

func mapFunction[W word.Word[W]](m Function[W], fn func(uint, bytecode.Bytecode[W]) []Bytecode[W]) *Function[W] {
	var vectors = make([]bytecode.Vector[W], len(m.vectors))
	//
	for i, vec := range m.vectors {
		vectors[i] = vec.Map(fn)
	}
	//
	return &Function[W]{m.moduleBase, m.native, vectors}
}

// ============================================================================
// Mapping functions
// ============================================================================

func addCheckPoint[W word.Word[W]](fid ModuleId) func(uint, bytecode.Bytecode[W]) []Bytecode[W] {
	return func(_ uint, b bytecode.Bytecode[W]) []Bytecode[W] {
		if c, ok := b.(*bytecode.Call); ok && c.Target == fid {
			b = c.SetCheckPoint()
		}
		//
		return []Bytecode[W]{b}
	}
}
