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
package vm

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter/encoding"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W Word[W]] = bytecode.Bytecode[W]

// BytecodeModule describes a moddule, such as a function or memory
type BytecodeModule[W Word[W]] = descriptor.Module[W]

// BytecodeFunction describes a function
type BytecodeFunction[W Word[W]] = descriptor.Function[W]

// BytecodeMemory describes a memory
type BytecodeMemory[W Word[W]] = descriptor.Memory[W]

// BytecodeRegister describes a register
type BytecodeRegister[W Word[W]] = descriptor.Register[W]

// BytecodeInterpreter is an optimised bytecode interpreter for executing word
// instructions.
type BytecodeInterpreter[W Word[W]] = interpreter.Interpreter[W]

// BytecodeProgram represents a bytecode assembly program.
type BytecodeProgram[W Word[W]] = descriptor.Program[W]

// BytecodeBinary represents a compiled bytecode program, along with
// accompanying symbolic information.  This is primarily useful for debugging.
type BytecodeBinary[W Word[W]] = encoding.Binary[W]

// BytecodeEnvironment provides information about the enclosing environment of a
// bytecode, and is primarily for debugging and validation.
type BytecodeEnvironment = bytecode.Environment

// NewBytecodeInterpreter constructs an interpreter for executing the given
// bytecode program.  The modulus is the prime characteristic of the surrounding
// field, used when executing native field instructions.
func NewBytecodeInterpreter[W word.Word[W]](program BytecodeProgram[W], modulus W) *BytecodeInterpreter[W] {
	return interpreter.New(program, modulus)
}

// CompileProgram compiles a program descriptor into an binary (i.e. executable)
// bytecode program.
func CompileProgram[W word.Word[W]](p BytecodeProgram[W]) BytecodeBinary[W] {
	return interpreter.CompileProgram(p)
}
