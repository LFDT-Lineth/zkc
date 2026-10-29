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
package view

import (
	"slices"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// TraceView provides an abstract view of a trace.  As such, it may not include
// all modules found in a trace; rather it may include some subset matching a
// predicate, or only those which are publically visible, etc.
type TraceView[F field.Element[F]] interface {
	// Filter this view to produce a more focused view.
	Filter(TraceFilter) TraceView[F]
	// Access ith module view
	Module(uint) ModuleView[F]
	// Sort the modules of this view
	Sort(func(sc.ModuleId, sc.ModuleId) int) TraceView[F]
	// Determine number of modules in this view
	Width() uint
}

// ============================================================================
// Implementation
// ============================================================================

type traceView[F field.Element[F]] struct {
	modules []ModuleView[F]
}

// Filter implementation for TraceView interface
func (p *traceView[F]) Filter(filter TraceFilter) TraceView[F] {
	var modules []ModuleView[F]
	//
	for _, m := range p.modules {
		if cf := filter.Module(m.Data().Id()); cf != nil {
			modules = append(modules, m.Filter(cf))
		}
	}
	//
	return &traceView[F]{modules}
}

// Module implementation for TraceView interface
func (p *traceView[F]) Module(ith uint) ModuleView[F] {
	return p.modules[ith]
}

func (p *traceView[F]) Sort(cmp func(l, r sc.ModuleId) int) TraceView[F] {
	var nmodules = slices.Clone(p.modules)
	//
	slices.SortFunc(nmodules, func(l, r ModuleView[F]) int {
		return cmp(l.Data().Id(), r.Data().Id())
	})
	//
	return &traceView[F]{nmodules}
}

// Width implementation for TraceView interface
func (p *traceView[F]) Width() uint {
	return uint(len(p.modules))
}
