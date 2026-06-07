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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

type Interpreter[W word.Word[W]] struct {
	// Program counter position for the bytecode interpreter
	pc uint32
	// Frame pointer position for the bytecode interpreter
	fp       uint32
	bytecode []uint32
	// word stack
	wordStack heap.Heap[W]
	// call stack
	callStack heap.Heap[uint32]
	// read-only memories
	roms []memory.StaticArray[W]
	// write-once memories
	woms []memory.StaticArray[W]
	// random-access memories
	rams []memory.StaticArray[W]
	// bipartite random-access memories
	brams []memory.BiPartiteRandomAccess[W]
}

// NewInterpreter constructs a new bytecode interpreter for the given program.
func NewInterpreter[W word.Word[W]](program Program) *Interpreter[W] {
	return &Interpreter[W]{
		bytecode: program.bytecodes,
	}
}

// Boot implementation of Core interface
func (p *Interpreter[W]) Boot(fun string) error {
	panic("todo")
}

// Execute implementation of Core interface
func (p *Interpreter[W]) Execute(steps uint) (uint, error) {
	panic("todo")
}

// Inputs implementation of Core interface
func (p *Interpreter[W]) Inputs() iter.Iterator[memory.InputOutput[W]] {
	var riter = iter.NewArrayIterator(p.roms)
	return iter.NewCastIterator[memory.StaticArray[W], memory.InputOutput[W]](riter)
}

// Outputs implementation of Core interface
func (p *Interpreter[W]) Outputs() iter.Iterator[memory.InputOutput[W]] {
	var riter = iter.NewArrayIterator(p.woms)
	return iter.NewCastIterator[memory.StaticArray[W], memory.InputOutput[W]](riter)
}
