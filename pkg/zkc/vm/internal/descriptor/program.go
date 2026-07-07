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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Tags identifying the concrete kind of a module when encoding the (interface
// typed) modules of a program.
const (
	moduleFunctionTag uint8 = iota
	moduleMemoryTag
)

// MapProgram applies a mapping function to all modules within this program,
// producing an array of mapped items.  Returned items are guaranteed to be in
// the same order as in the original program.
func MapProgram[W word.Word[W], T any](p Program[W], mapper func(uint, Module[W]) T) []T {
	var mapping = make([]T, len(p.modules))
	//
	for i, m := range p.modules {
		mapping[i] = mapper(uint(i), m)
	}
	//
	return mapping
}

// Program represents a bytecode program.  This representation is useful for
// transforming bytecode programs, generating code or constraints from bytecode
// programs and analysing bytecode programs.  However, it is not good for
// executing bytecode programs (and, for that, compiled programs are better).
type Program[W word.Word[W]] struct {
	// modules declared within this (uncompiled) program.
	modules []Module[W]
	// field configuration for the target prime field
	field field.Config
}

// NewProgram creates a new program descriptor.
func NewProgram[W word.Word[W]](field field.Config, modules ...Module[W]) Program[W] {
	return Program[W]{modules, field}
}

// AddCheckPoint returns a copy of this program in which all calls to the target
// function are "checkpointing calls" (i.e. have their CheckPoint field set).
// Switching a call to checkpointing only swaps its ENTER opcode (ENTER_n =>
// ENTERCP_n), which is width-preserving; hence every instruction offset --
// along with the symbol and chunk side-tables -- is unaffected, and the
// returned program shares those tables with the original.
func (p Program[W]) AddCheckPoint(fid uint16) Program[W] {
	return mapProgram(p, addCheckPoint[W](fid))
}

// EnvironmentOf returns an environment for the given module.  This is useful
// for working with bytecodes enclosed by that module, etc.
func (p Program[W]) EnvironmentOf(mid uint16) bytecode.Environment {
	return &moduleEnvironment[W]{
		mid,
		p.modules,
	}
}

// Field returns the field configuration for prime field associated with this
// program.
func (p Program[W]) Field() field.Config {
	return p.field
}

// MaxStaticDepth reports the maximum depth (i.e. number of rows) of any static
// table used within this program.
func (p Program[W]) MaxStaticDepth() uint {
	var depth = uint(0)
	//
	for _, m := range p.modules {
		if ith, ok := m.(*Memory[W]); ok && ith.IsStatic() {
			depth = max(depth, ith.StaticDepth())
		}
	}
	//
	return depth
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

// Inputs returns the set of declared input memories
func (p *Program[W]) Inputs() iter.Iterator[Memory[W]] {
	var inputs []Memory[W]
	//
	for _, m := range p.modules {
		if m, ok := m.(*Memory[W]); ok && m.IsReadOnly() && !m.IsStatic() {
			inputs = append(inputs, *m)
		}
	}
	//
	return iter.NewArrayIterator(inputs)
}

// Outputs returns the set of declared output memories
func (p *Program[W]) Outputs() iter.Iterator[Memory[W]] {
	var outputs []Memory[W]
	//
	for _, m := range p.modules {
		if m, ok := m.(*Memory[W]); ok && m.IsWriteOnly() {
			outputs = append(outputs, *m)
		}
	}
	//
	return iter.NewArrayIterator(outputs)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode marshals this program.  Modules are held via the Module interface,
// so each is written with a leading tag identifying its concrete kind; the
// concrete types themselves define the encoding of their (unexported) fields.
//
// nolint
func (p *Program[W]) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	// Number of modules.
	if err := gobEncoder.Encode(uint16(len(p.modules))); err != nil {
		return nil, err
	}
	// Encode field configuration
	if err := gobEncoder.Encode(p.field); err != nil {
		return nil, err
	}
	// Each module, prefixed with a tag identifying its concrete type.
	for _, m := range p.modules {
		switch m := m.(type) {
		case *Function[W]:
			if err := gobEncoder.Encode(moduleFunctionTag); err != nil {
				return nil, err
			}
			//
			if err := gobEncoder.Encode(m); err != nil {
				return nil, err
			}
		case *Memory[W]:
			if err := gobEncoder.Encode(moduleMemoryTag); err != nil {
				return nil, err
			}
			//
			if err := gobEncoder.Encode(m); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("cannot encode unknown module \"%s\"", m.Name())
		}
	}
	//
	return buffer.Bytes(), nil
}

// nolint
func (p *Program[W]) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		n          uint16
	)
	// Number of modules.
	if err := gobDecoder.Decode(&n); err != nil {
		return err
	}
	// Decode field config
	if err := gobDecoder.Decode(&p.field); err != nil {
		return err
	}
	//
	modules := make([]Module[W], n)
	// Each module, dispatched on its leading tag.
	for i := uint16(0); i < n; i++ {
		var tag uint8
		//
		if err := gobDecoder.Decode(&tag); err != nil {
			return err
		}
		//
		switch tag {
		case moduleFunctionTag:
			var fn Function[W]
			//
			if err := gobDecoder.Decode(&fn); err != nil {
				return err
			}
			//
			modules[i] = &fn
		case moduleMemoryTag:
			var mem Memory[W]
			//
			if err := gobDecoder.Decode(&mem); err != nil {
				return err
			}
			//
			modules[i] = &mem
		default:
			return fmt.Errorf("cannot decode unknown module tag %d", tag)
		}
	}
	//
	p.modules = modules
	//
	return nil
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
func (p moduleEnvironment[W]) HasModule(name string) util.Option[ModuleId] {
	for i, r := range p.modules {
		if r.Name() == name {
			return util.Some(ModuleId(i))
		}
	}
	// Failed
	return util.None[ModuleId]()
}

// HasRegister checks whether a register with the given name exists and, if
// so, returns its register identifier.  Otherwise, it returns false.
func (p moduleEnvironment[W]) HasRegister(name string) util.Option[RegisterId] {
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
	return Program[W]{modules, p.field}
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
