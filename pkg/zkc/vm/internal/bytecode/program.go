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
package bytecode

import (
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
)

// Module provides a convenient alias
type Module = base.Module

// SystemMap provides a convenient alias
type SystemMap = base.SystemMap

// Program represents a self-contained bytecode program with a given entry
// point.
type Program struct {
	// Modules declaredThe by the program
	modules []Module
	// The bytecode sequence itself.
	bytecodes []uint32
	// Symbols associated with bytecode offsets
	symbols map[uint32]uint
}

// NewProgram constructs a new bytecode program with a given entry point.
func NewProgram(modules []Module, bytecodes []uint32, symbols map[uint32]uint) Program {
	return Program{
		modules,
		bytecodes,
		symbols,
	}
}

// HasModule returns the identifier for the module with the given name, or returns
// false if no such module exists.
func (p Program) HasModule(name string) (uint, bool) {
	var mid = array.FindMatching(p.modules, func(m Module) bool {
		return m.Name() == name
	})
	//
	return mid, mid != math.MaxUint
}

// Module returns the module with the given identifier.
func (p Program) Module(mid uint) Module {
	return p.modules[mid]
}

// AddressOf determines the address of a given (function) symbol, or returns an
// error if no such symbol exists.
func (p Program) AddressOf(mid uint) (uint32, bool) {
	// Find the symbols address
	for addr, id := range p.symbols {
		if id == mid {
			return addr, true
		}
	}
	// failed
	return 0, false
}

// SymbolAt determines whether or not there is a symbol associated with a given
// instruction address.
func (p Program) SymbolAt(address Address) util.Option[Module] {
	if idx, ok := p.symbols[address]; ok {
		return util.Some(p.modules[idx])
	}
	//
	return util.None[Module]()
}

// Modules returns information about the modules declared within this program.
func (p Program) Modules() []Module {
	return p.modules
}
