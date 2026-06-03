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
package macro

import (
	"encoding/gob"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
)

// Alias for big integer representation of 1.
var one big.Int = *big.NewInt(1)

// Register is an alias for insn.Register
type Register = io.Register

// Instruction provides an abstract notion of a macro "machine instruction".
// Here, macro is intended to imply that the instruction may break down into
// multiple underlying "micro instructions".
type Instruction interface {
	io.Instruction
	// Lower this (macro) instruction into a sequence of one or more micro
	// instructions.
	Lower(pc uint) micro.Instruction
}

// BranchInstruction captures those instructions which may branch to some
// location.
type BranchInstruction interface {
	// Bind any labels contained within this instruction using the given label map.
	Bind(labels []uint)
}

func init() {
	gob.Register(Instruction(&Assign{}))
	gob.Register(Instruction(&Call{}))
	gob.Register(Instruction(&Cast{}))
	gob.Register(Instruction(&Division{}))
	gob.Register(Instruction(&Fail{}))
	gob.Register(Instruction(&Goto{}))
	gob.Register(Instruction(&IfGoto{}))
	gob.Register(Instruction(&IfThenElse{}))
	gob.Register(Instruction(&Return{}))
}
