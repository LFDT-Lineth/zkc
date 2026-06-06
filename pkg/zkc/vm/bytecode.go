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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode encapsulates a single bytecode instruction.
type Bytecode[W Word[W]] = bytecode.Bytecode[W]

// BytecodeInterpreter is an optimised bytecode interpreter for executing word
// instructions.
type BytecodeInterpreter[W Word[W]] = bytecode.Interpreter[W]

// BytecodeProgram represents a compiled bytecode program, along with
// accompanying symbolic information.
type BytecodeProgram = bytecode.Program

// DecodeBytecodes decodes a given bytecode program into a bytecode sequence.
func DecodeBytecodes[W word.Word[W]](p BytecodeProgram) []Bytecode[W] {
	return bytecode.Decode[W](p)
}
