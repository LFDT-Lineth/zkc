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
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// CellRefSet defines a type for sets of cell references.
type CellRefSet = set.AnySortedSet[tr.CellRef]

// Builder is responsible for building the viewing window for a trace.
type Builder[F field.Element[F]] struct {
	// Optional set of cells on which the constructed view should focus.
	cells util.Option[CellRefSet]
	// Computed indicates whether or not to show (low-level) computed registers
	computed bool
	// Default cell width to use
	cellWidth uint
	// Default title width to use
	titleWidth uint
	// Formatting to use
	formatting TraceFormatting
	// Schema for the trace being viewed
	schema util.Option[sc.Schema[F]]
	// Optional predicate determining whether a given module is publicly visible.
	// When not set, all modules are considered public.  This is used by callers
	// (e.g. zkc) to hide synthetic modules such as range-check tables.
	visibility util.Option[func(string) bool]
}

// NewBuilder constructs a default builder.
func NewBuilder[F field.Element[F]]() Builder[F] {
	return Builder[F]{util.None[CellRefSet](), false, 16, 16, DefaultFormatter(), util.None[sc.Schema[F]](),
		util.None[func(string) bool]()}
}

// WithCellWidth sets the maximum width of any cell in the view.
func (p Builder[F]) WithCellWidth(cellWidth uint) Builder[F] {
	var builder = p
	//
	builder.cellWidth = cellWidth
	//
	return builder
}

// WithTitleWidth sets the maximum width of any row title in the view.
func (p Builder[F]) WithTitleWidth(titleWidth uint) Builder[F] {
	var builder = p
	//
	builder.titleWidth = titleWidth
	//
	return builder
}

// WithComputed determines whether to show low-level computed registers.
func (p Builder[F]) WithComputed(computed bool) Builder[F] {
	var builder = p
	//
	builder.computed = computed
	//
	return builder
}

// WithFormatting determines what formatting to use when rendering this view.
func (p Builder[F]) WithFormatting(formatting TraceFormatting) Builder[F] {
	var builder = p
	//
	builder.formatting = formatting
	//
	return builder
}

// WithSchema applies schema information to the view.  The main
// benefit of this is that it offers support for constant-columns.
func (p Builder[F]) WithSchema(schema sc.Schema[F]) Builder[F] {
	var builder = p
	//
	builder.schema = util.Some(schema)
	//
	return builder
}

// WithVisibility applies a predicate determining whether a given module is
// publicly visible.  When not set, all modules are considered public.
func (p Builder[F]) WithVisibility(public func(string) bool) Builder[F] {
	var builder = p
	//
	builder.visibility = util.Some(public)
	//
	return builder
}

// Build the viewing window for this trace.
func (p Builder[F]) Build(trace tr.Shard[F]) TraceView {
	if p.schema.HasValue() {
		return p.buildWithSchema(p.schema.Unwrap(), trace)
	}
	//
	return p.buildWithoutSchema(trace)
}

func (p Builder[F]) buildWithSchema(schema sc.Schema[F], trace tr.Shard[F]) TraceView {
	var windows []ModuleView
	// Assign all constant columns (if applicable)
	trace = sc.AssignConstantColumns(p.schema.Unwrap(), trace)
	//
	for i := range trace.Width() {
		var (
			trMod   = trace.Module(i)
			scMod   = schema.Module(i)
			columns []SourceColumn
		)
		//
		for i := range trMod.Width() {
			var (
				rid   = register.NewId(i)
				trCol = trMod.Descriptor().Columns[i]
				scCol = scMod.Register(rid)
			)
			//
			if !scCol.IsComputed() || p.computed {
				columns = append(columns, SourceColumn{
					Name:     trCol.Name,
					Computed: scCol.IsComputed(),
					Register: rid,
				})
			}
		}
		//
		data := newModuleData(i, trMod, p.isPublic(trMod.Name()), columns)
		// construct initial module view
		windows = append(windows, &moduleView[F]{
			window:     data.Window(),
			cellWidth:  p.cellWidth,
			titleWidth: p.titleWidth,
			formatting: p.formatting.Module(data),
			data:       data,
		})
	}
	//
	return &traceView{windows}
}

func (p Builder[F]) buildWithoutSchema(trace tr.Shard[F]) TraceView {
	var windows []ModuleView
	//
	for i := range trace.Width() {
		var (
			trMod   = trace.Module(i)
			columns []SourceColumn
		)
		for i := range trMod.Width() {
			var (
				ith = trMod.Descriptor().Columns[i]
				rid = register.NewId(i)
			)
			//
			columns = append(columns, SourceColumn{
				Name:     ith.Name,
				Computed: false,
				Register: rid,
			})
		}
		//
		data := newModuleData(i, trMod, p.isPublic(trMod.Name()), columns)
		// construct initial module view
		windows = append(windows, &moduleView[F]{
			window:     data.Window(),
			cellWidth:  p.cellWidth,
			titleWidth: p.titleWidth,
			formatting: p.formatting.Module(data),
			data:       data,
		})
	}
	//
	return &traceView{windows}
}

// Determine whether a given module is publicly visible.  Modules are public by
// default, unless a visibility predicate is given.
func (p Builder[F]) isPublic(name string) bool {
	if p.visibility.HasValue() {
		return p.visibility.Unwrap()(name)
	}
	//
	return true
}
