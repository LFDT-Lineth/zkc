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

import "github.com/LFDT-Lineth/zkc/pkg/util"

// Program represents a self-contained bytecode program with a given entry
// point.
type Program struct {
	// The bytecode sequence itself.
	bytecodes []uint32
	// Symbols associated with bytecode offsets
	symbols map[uint32]string
}

// NewProgram constructs a new bytecode program with a given entry point.
func NewProgram(bytecodes []uint32, symbols map[uint32]string) Program {
	return Program{
		bytecodes,
		symbols,
	}
}

// Bytecodes returns the underlying bytecode sequence.
func (p Program) Bytecodes() (codes []Bytecode) {
	return decode(p.bytecodes)
}

// SymbolAt determines whether or not there is a symbol associated with a given
// instruction address.
func (p Program) SymbolAt(address Address) util.Option[string] {
	if sym, ok := p.symbols[address]; ok {
		return util.Some(sym)
	}
	//
	return util.None[string]()
}
