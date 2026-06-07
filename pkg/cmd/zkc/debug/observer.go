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
package debug

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// TraceObserver prints a trace
type TraceObserver[W vm.Word[W]] struct {
	CostReport *codegen.CostReport
	depth      uint
	fun        *vm.Function[vm.WordInstruction]
	insn       vm.WordInstruction
	pc         vm.ProgramCounter
	activeCost []activeCostLabel
}

type activeCostLabel struct {
	label string
	depth uint
}

// Initialise implementation for Observer interface
func (p *TraceObserver[W]) Initialise(machine *vm.WordMachine[W]) {

}

// PreExecution implementation for Observer interface
func (p *TraceObserver[W]) PreExecution(machine *vm.WordMachine[W]) {
	var (
		n = machine.Depth()
	)
	//
	if n > 0 {
		p.pruneCostLabels(n)

		if n != p.depth {
			fmt.Println()
			p.enterFunction(machine)
			fmt.Print(p.callStack(machine))
			fmt.Println()
		}
		//
		p.writeInstruction(machine)
		p.recordCost(n)
	}
}

// PostExecution implementation for Observer interface
func (p *TraceObserver[W]) PostExecution(machine *vm.WordMachine[W]) {
	var (
		n = machine.Depth()
	)
	//
	if n > 0 {
		if n == p.depth {
			p.writeState(machine)
		}

		fmt.Println()
	}
}

func (p *TraceObserver[W]) enterFunction(machine *vm.WordMachine[W]) {
	var (
		n     = machine.Depth()
		frame = machine.StackFrame(0)
	)
	//
	p.depth = n
	p.fun = frame.Function()
	p.insn = nil
}

func (p *TraceObserver[W]) writeInstruction(machine *vm.WordMachine[W]) {
	var (
		frame = machine.StackFrame(0)
		pc    = frame.PC()
		vec   = frame.Vector(pc.Macro())
	)
	//
	p.pc = pc
	p.insn = vec.Codes[pc.Micro()]
}

func (p *TraceObserver[W]) pruneCostLabels(depth uint) {
	for len(p.activeCost) > 0 && p.activeCost[len(p.activeCost)-1].depth > depth {
		p.activeCost = p.activeCost[:len(p.activeCost)-1]
	}
}

func (p *TraceObserver[W]) recordCost(depth uint) {
	if p.CostReport == nil {
		return
	}

	var labels []string

	for _, active := range p.activeCost {
		labels = append(labels, active.label)
	}

	direct := p.CostReport.LabelsOf(p.insn)
	labels = append(labels, direct...)
	p.CostReport.AddDynamic(labels...)

	if _, ok := p.insn.(*instruction.Call); ok {
		for _, label := range direct {
			p.activeCost = append(p.activeCost, activeCostLabel{
				label: label,
				depth: depth + 1,
			})
		}
	}
}

func (p *TraceObserver[W]) writeState(machine *vm.WordMachine[W]) {
	var (
		frame  = machine.StackFrame(0)
		base   = instruction.NewSystemMap(p.fun.RegisterMap(), machine.Modules())
		values = make(map[uint]string)
	)
	// Collect register values. In PostExecution, sources still hold their pre-execution values
	// (unmodified by the instruction), while definitions hold their post-execution values.
	// Definitions are added last so that when a register appears on both sides, the
	// post-execution value is shown.
	for _, r := range p.insn.Uses() {
		values[r.Unwrap()] = frame.Load(r).Text(16)
	}

	for _, r := range p.insn.Definitions() {
		values[r.Unwrap()] = frame.Load(r).Text(16)
	}
	//
	annotated := &annotatedMap[W]{base: base, values: values}
	insnStr := fmt.Sprintf("[%02x.%02x] %s", p.pc.Macro(), p.pc.Micro(), p.insn.String(annotated))
	fmt.Print(insnStr)
}

func (p *TraceObserver[W]) callStack(machine *vm.WordMachine[W]) string {
	var builder strings.Builder
	//
	for i := p.depth; i > 0; i = i - 1 {
		var ith = machine.StackFrame(i - 1)
		//
		fmt.Fprintf(&builder, "> %s ", ith.Signature())
	}
	//
	return builder.String()
}

// annotatedMap wraps a SystemMap and annotates each register name with its
// current value as "[0xVAL]", producing inline value display in instruction strings.
type annotatedMap[W vm.Word[W]] struct {
	base   instruction.SystemMap
	values map[uint]string // register index → hex value string (no "0x" prefix)
}

func (a *annotatedMap[W]) Register(id register.Id) register.Register {
	reg := a.base.Register(id)
	if val, ok := a.values[id.Unwrap()]; ok {
		return register.New(reg.Kind(), reg.Name()+" [0x"+val+"]", reg.Width(), *reg.Padding())
	}

	return reg
}

func (a *annotatedMap[W]) Module(id uint) instruction.Module { return a.base.Module(id) }

func (a *annotatedMap[W]) Name() trace.ModuleName { return a.base.Name() }

func (a *annotatedMap[W]) HasRegister(name string) (register.Id, bool) {
	return a.base.HasRegister(name)
}

func (a *annotatedMap[W]) Registers() []register.Register { return a.base.Registers() }

func (a *annotatedMap[W]) String() string { return a.base.String() }
