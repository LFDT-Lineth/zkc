// Copyright Consensys Incorporated
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
package transform

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Allocator is a useful alias
type Allocator[W Word[W]] = split.Allocator[W]

// Bytecode provides a convenient alias
type Bytecode[W Word[W]] = bytecode.Bytecode[W]

// BytecodeVector provides a convenient alias
type BytecodeVector[W Word[W]] = bytecode.Vector[W]

// Environment provides a useful alias
type Environment[W Word[W]] = bytecode.Environment[W]

// Function provides a useful alias
type Function[W Word[W]] = descriptor.Function[W]

// Register provides a useful alias
type Register[W Word[W]] = descriptor.Register[W]

// RegisterMap provides a useful alias
type RegisterMap[W Word[W]] = descriptor.RegisterMap[W]

// ProgramPoint provides a convenient alias
type ProgramPoint = descriptor.ProgramPoint

// Word provides a convenient alias
type Word[W any] = word.Word[W]
