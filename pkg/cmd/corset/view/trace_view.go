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
package view

import (
	"slices"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
)

// TraceView provides an abstract view of a trace.  As such, it may not include
// all modules found in a trace; rather it may include some subset matching a
// predicate, or only those which are publically visible, etc.
type TraceView interface {
	// Filter this view to produce a more focused view.
	Filter(TraceFilter) TraceView
	// Access ith module view
	Module(uint) ModuleView
	// Sort the modules of this view
	Sort(func(sc.ModuleId, sc.ModuleId) int) TraceView
	// Determine number of modules in this view
	Width() uint
}

// ============================================================================
// Implementation
// ============================================================================

type traceView struct {
	modules []ModuleView
}

// Filter implementation for TraceView interface
func (p *traceView) Filter(filter TraceFilter) TraceView {
	var modules []ModuleView
	//
	for _, m := range p.modules {
		if cf := filter.Module(m.Data().Id()); cf != nil {
			modules = append(modules, m.Filter(cf))
		}
	}
	//
	return &traceView{modules}
}

// Module implementation for TraceView interface
func (p *traceView) Module(ith uint) ModuleView {
	return p.modules[ith]
}

func (p *traceView) Sort(cmp func(l, r sc.ModuleId) int) TraceView {
	var nmodules = slices.Clone(p.modules)
	//
	slices.SortFunc(nmodules, func(l, r ModuleView) int {
		return cmp(l.Data().Id(), r.Data().Id())
	})
	//
	return &traceView{nmodules}
}

// Width implementation for TraceView interface
func (p *traceView) Width() uint {
	return uint(len(p.modules))
}
