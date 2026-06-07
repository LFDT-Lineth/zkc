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
package codegen

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
)

// CostReport accumulates generated WIR micro-instruction counts for source
// statements annotated with #[cost:label].
type CostReport struct {
	annotations   []costAnnotation
	labels        map[Instruction][]string
	staticTotals  map[string]uint
	dynamicTotals map[string]uint
}

type costAnnotation struct {
	label    string
	function uint
	direct   uint
	calls    []uint
}

// NewCostReport constructs an empty cost report.
func NewCostReport() *CostReport {
	return &CostReport{
		labels:        make(map[Instruction][]string),
		staticTotals:  make(map[string]uint),
		dynamicTotals: make(map[string]uint),
	}
}

// Add records the annotated statement's direct WIR cost and function calls.
func (p *CostReport) Add(label string, function uint, vec VectorInstruction) {
	if p == nil {
		return
	}

	p.annotations = append(p.annotations, costAnnotation{
		label:    label,
		function: function,
		direct:   uint(len(vec.Codes)),
		calls:    collectCalls(vec),
	})

	for _, code := range vec.Codes {
		p.labels[code] = append(p.labels[code], label)
	}
}

// Finalize computes inclusive costs once all function bodies are available.
func (p *CostReport) Finalize(modules []vm.Module) {
	if p == nil {
		return
	}

	p.staticTotals = make(map[string]uint, len(p.annotations))

	for _, annotation := range p.annotations {
		total := annotation.direct

		for _, call := range annotation.calls {
			total += p.functionCost(call, modules, map[uint]bool{annotation.function: true})
		}

		p.staticTotals[annotation.label] += total
	}
}

// LabelsOf returns the cost labels attached to a compiled WIR instruction.
func (p *CostReport) LabelsOf(insn Instruction) []string {
	if p == nil {
		return nil
	}

	return p.labels[insn]
}

// AddDynamic records an executed WIR micro-instruction against active labels.
func (p *CostReport) AddDynamic(labels ...string) {
	if p == nil {
		return
	}

	for _, label := range labels {
		p.dynamicTotals[label]++
	}
}

func collectCalls(vec VectorInstruction) []uint {
	var calls []uint

	for _, code := range vec.Codes {
		if call, ok := code.(*instruction.Call); ok {
			calls = append(calls, call.Id)
		}
	}

	return calls
}

func (p *CostReport) functionCost(id uint, modules []vm.Module, active map[uint]bool) uint {
	if int(id) >= len(modules) || active[id] {
		return 0
	}

	fn, ok := modules[id].(*Function)
	if !ok || fn.IsNative() {
		return 0
	}

	active[id] = true
	defer delete(active, id)

	var total uint

	for _, vec := range fn.Code() {
		total += uint(len(vec.Codes))

		for _, call := range collectCalls(vec) {
			total += p.functionCost(call, modules, active)
		}
	}

	return total
}

// StaticTotals returns a copy of the static inclusive costs keyed by label.
func (p *CostReport) StaticTotals() map[string]uint {
	if p == nil {
		return nil
	}

	totals := make(map[string]uint, len(p.staticTotals))

	for label, cost := range p.staticTotals {
		totals[label] = cost
	}

	return totals
}

// DynamicTotals returns a copy of the executed costs keyed by label.
func (p *CostReport) DynamicTotals() map[string]uint {
	if p == nil {
		return nil
	}

	totals := make(map[string]uint, len(p.dynamicTotals))

	for label, cost := range p.dynamicTotals {
		totals[label] = cost
	}

	return totals
}
