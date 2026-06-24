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

// Package descriptor describes the static structure of a compiled bytecode
// program, independently of the bytecode instruction stream itself.  A program
// is made up of modules, where each module is either a function (an executable
// unit backed by bytecode or a native circuit) or a memory (a ROM, RAM, etc).
// Every module exposes a set of registers (inputs, outputs and computed values)
// which are ultimately mapped onto columns in the resulting trace.  The
// interfaces and types defined here provide a read-only, descriptive view over
// those components.
package descriptor

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bytecode determines the type of a bytecode instruction.
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// RegisterId determines the type of register identifiers.
type RegisterId = bytecode.RegisterId

// ModuleId detemines the type of module identifiers.
type ModuleId = bytecode.ModuleId
