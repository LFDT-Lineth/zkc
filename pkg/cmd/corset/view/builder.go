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
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
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
	// Schema for the trace being viewed
	schema schema.Schema[F]
	// Formatting to use
	formatting TraceFormatting
	// Optional source map information.  This is primarily used to determine
	srcmap util.Option[corset.SourceMap]
	// Optional predicate determining whether a given module is publicly visible.
	// When set, it overrides the public flag otherwise derived from the source
	// map (or the default).  This is used by callers without a source map (e.g.
	// zkc) to hide synthetic modules such as range-check tables.
	visibility util.Option[func(string) bool]
}

// NewBuilder constructs a default builder.
func NewBuilder[F field.Element[F]](schema schema.Schema[F]) Builder[F] {
	return Builder[F]{util.None[CellRefSet](), false, 16, 16, schema,
		DefaultFormatter(), util.None[corset.SourceMap](), util.None[func(string) bool]()}
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

// WithSourceMap applies source-mapping information to the view.  The main
// benefit of this is that it includes display modifiers.
func (p Builder[F]) WithSourceMap(srcmap corset.SourceMap) Builder[F] {
	var builder = p
	//
	builder.srcmap = util.Some(srcmap)
	//
	return builder
}

// WithVisibility applies a predicate determining whether a given module is
// publicly visible.  When set, it overrides the public flag otherwise derived
// from the source map (or the default).
func (p Builder[F]) WithVisibility(public func(string) bool) Builder[F] {
	var builder = p
	//
	builder.visibility = util.Some(public)
	//
	return builder
}

// Build the viewing window for this trace.
func (p Builder[F]) Build(trace tr.Shard[F]) TraceView {
	var windows []ModuleView
	//
	srcmap, enums := extractSourceMap(p.srcmap)
	//
	for i := range trace.Width() {
		trMod := trace.Module(i)
		scMod := p.schema.Module(i)
		//
		public, columns := extractSourceMapData(trMod, p.computed, srcmap)
		// Override visibility with the caller-supplied predicate (if any).
		if p.visibility.HasValue() {
			public = p.visibility.Unwrap()(trMod.Name())
		}
		//
		data := newModuleData(i, scMod, trMod, public, enums, columns)
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

func extractSourceMap(optSrcmap util.Option[corset.SourceMap]) (map[string]corset.SourceModule, []corset.Enumeration) {
	var (
		mapping = make(map[string]corset.SourceModule)
		enums   []corset.Enumeration
	)

	if optSrcmap.HasValue() {
		var srcmap = optSrcmap.Unwrap()
		//
		for _, module := range srcmap.Flatten(concreteModules) {
			mapping[module.Name] = module
		}
		//
		enums = srcmap.Enumerations
	}
	//
	return mapping, enums
}

func extractSourceMapData[F field.Element[F]](trMod tr.Module[F], computed bool,
	srcmap map[string]corset.SourceModule) (bool, []SourceColumn) {
	//
	var (
		public  = true
		columns []SourceColumn
		seen    = make(map[uint]bool)
		name    = trMod.Name()
	)
	//
	if m, ok := srcmap[name]; ok {
		public = m.Public
		// Extract column info
		columns = extractSourceColumns(file.NewAbsolutePath(""),
			m.Selector, m.Columns, m.Submodules)
		// Mark all as seen
		for _, column := range columns {
			seen[column.Register.Unwrap()] = true
		}
	}
	//
	if computed {
		// Add any registers not already seen
		for i := range trMod.Width() {
			ith := trMod.Descriptor().Columns[i]
			//
			if _, ok := seen[i]; !ok {
				rid := register.NewId(i)
				//
				columns = append(columns, SourceColumn{
					Name:     ith.Name,
					Display:  0,
					Computed: true,
					Selector: util.None[string](),
					Register: rid,
				})
			}
		}
	}
	//
	return public, columns
}

// ExtractSourceColumns extracts source column descriptions for a given module
// based on the corset source mapping.  This is particularly useful when you
// want to show the original name for a column (e.g. when its in a perspective),
// rather than the raw register name.
func extractSourceColumns(path file.Path, selector util.Option[string],
	columns []corset.SourceColumn, submodules []corset.SourceModule) []SourceColumn {
	//
	var srcColumns []SourceColumn
	//
	for _, col := range columns {
		//
		name := path.Extend(col.Name).String()[1:]
		//
		srcColumns = append(srcColumns, SourceColumn{
			Name:     name,
			Display:  col.Display,
			Computed: col.Computed,
			Selector: selector,
			Register: col.Register.Register(),
		})
	}
	//
	for _, submod := range submodules {
		// Curiously, it only makes sense to recurse on virtual modules here.
		if submod.Virtual {
			subpath := path.Extend(submod.Name)
			subSrcColumns := extractSourceColumns(*subpath, submod.Selector,
				submod.Columns, submod.Submodules)
			//
			srcColumns = append(srcColumns, subSrcColumns...)
		}
	}
	//
	return srcColumns
}

func concreteModules(m *corset.SourceModule) bool {
	return !m.Virtual
}
