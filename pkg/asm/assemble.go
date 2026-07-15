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
package asm

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/asm/program"
	"github.com/LFDT-Lineth/zkc/pkg/ir/hir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Register describes a single register within a function.
type Register = io.Register

// MicroComponent is a component whose instructions (if applicable) are
// themselves micro instructions.  A micro function represents the lowest
// representation of a function, where each instruction is made up of
// microcodes.
type MicroComponent = io.Component[micro.Instruction]

// MicroFunction is a function whose instructions are themselves micro
// instructions.  A micro function represents the lowest representation of a
// function, where each instruction is made up of microcodes.
type MicroFunction = io.Function[micro.Instruction]

// MicroProgram represents a set of components at the micro level.
type MicroProgram = io.Program[micro.Instruction]

// MicroHirProgram represents a mixed assembly and legacy program, where
// assembly functions are composed from micro instructions.
type MicroHirProgram = MixedProgram[word.BigEndian, micro.Instruction, hir.Module]

// MicroMirProgram represents a mixed assembly and legacy program, where
// assembly functions are composed from micro instructions.
type MicroMirProgram[F field.Element[F]] = MixedProgram[F, micro.Instruction, mir.Module[F]]

// MicroModule is an instance of schema.Module which encapsulates a MicroFunction[F].
type MicroModule[F field.Element[F]] = program.Module[F, micro.Instruction]
