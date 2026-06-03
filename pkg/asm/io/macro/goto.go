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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Goto provides an unconditional branching instruction to a given instructon.
type Goto struct {
	Target uint
}

// Bind any labels contained within this instruction using the given label map.
func (p *Goto) Bind(labels []uint) {
	p.Target = labels[p.Target]
}

// Execute implementation for Instruction interface.
func (p *Goto) Execute(state io.State) uint {
	return p.Target
}

// Lower implementation for Instruction interface.
func (p *Goto) Lower(pc uint) micro.Instruction {
	// Lowering here produces an instruction containing a single microcode.
	return micro.NewInstruction(&micro.Jmp{Target: p.Target})
}

// RegistersRead implementation for Instruction interface.
func (p *Goto) RegistersRead() []io.RegisterId {
	return nil
}

// RegistersWritten implementation for Instruction interface.
func (p *Goto) RegistersWritten() []io.RegisterId {
	return nil
}

func (p *Goto) String(fn register.Map) string {
	return fmt.Sprintf("goto %d", p.Target)
}

// Validate implementation for Instruction interface.
func (p *Goto) Validate(fieldWidth uint, fn register.Map) error {
	return nil
}
