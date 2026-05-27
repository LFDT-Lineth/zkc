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
package transform

import (
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	finsn "github.com/consensys/go-corset/pkg/zkc/vm/instruction/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/function"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
)

// Monomial is a useful alias
type Monomial = finsn.Monomial

// Polynomial is a useful alias
type Polynomial = finsn.Polynomial

// SystemMap is a useful alias
type SystemMap = instruction.SystemMap

// Module is a useful alias
type Module = machine.Module

// WordFunction is a useful alias
type WordFunction = function.Function[WordInstruction]

// FieldFunction is a useful alias
type FieldFunction = function.Function[instruction.Field]

// Vector is a useful alias
type Vector[I instruction.Instruction] = instruction.Vector[I]

// WordInstruction is a useful alias
type WordInstruction = instruction.Word

// VectorInstruction is a useful alias
type VectorInstruction = Vector[WordInstruction]

// RegisterAllocator provides a simple means of allocating new registers
type RegisterAllocator = register.Allocator[int]
