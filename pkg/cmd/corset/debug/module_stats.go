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
	"math"
	"math/big"

	cmd_util "github.com/consensys/go-corset/pkg/cmd/corset/util"
	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema"
	sc "github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/termio"
)

// PrintModuleStats prints out various items of information about the modules in a given schema.
func PrintModuleStats[F field.Element[F]](stack cmd_util.SchemaStack[F], maxCellWidth uint, sorter uint) {
	var (
		//
		schema      = stack.ConcreteSchema()
		summarisers = getModuleSummarisers[F]()
		m           = 1 + uint(len(summarisers))
		n           = schema.Width()
		// Go!
		tbl = termio.NewFormattedTable(m, n+1)
	)
	// Set column titles
	for i := uint(0); i < uint(len(summarisers)); i++ {
		tbl.Set(i+1, 0, termio.NewText(summarisers[i].name))
	}
	// Compute column data
	for i := range n {
		var (
			mod = schema.Module(i)
			row = summariseModule(mod, summarisers)
		)
		// Set row
		tbl.SetRow(i+1, row...)
	}
	//
	tbl.SetMaxWidths(maxCellWidth)
	tbl.Sort(1, termio.NewTableSorter().
		SortNumericalColumn(sorter).
		Invert())
	tbl.Print(true)
}

func summariseModule[F any](mod schema.Module[F], summarisers []ModuleSummariser[F]) []termio.FormattedText {
	var (
		row = make([]termio.FormattedText, len(summarisers)+1)
	)
	//
	row[0] = termio.NewText(mod.Name().String())
	//
	for i, s := range summarisers {
		summary := fmt.Sprintf("%d", s.summary(mod))
		row[i+1] = termio.NewText(summary)
	}
	//
	return row
}

// ModuleSummariser abstracts the notion of a function which summarises some
// aspect of a given module (e.g. how many constraints it has).
type ModuleSummariser[F any] struct {
	name    string
	summary func(sc.Module[F]) int
}

func getModuleSummarisers[F field.Element[F]]() []ModuleSummariser[F] {
	return []ModuleSummariser[F]{
		// Constraints
		moduleConstraintCounter("Constraints", func(sc.Constraint[F]) bool { return true }),
		// Columns
		{"Columns", func(m sc.Module[F]) int { return int(m.Width()) }},
		// Assignments
		{"Assignments", func(m sc.Module[F]) int { return int(m.Assignments().Count()) }},
		// Complexity
		{"Complexity", complexityAnalysis[F]},
	}
}

func moduleConstraintCounter[F any](title string, includes func(schema.Constraint[F]) bool) ModuleSummariser[F] {
	return ModuleSummariser[F]{
		name: title,
		summary: func(schema sc.Module[F]) int {
			sum := 0

			for iter := schema.Constraints(); iter.HasNext(); {
				if includes(iter.Next()) {
					sum++
				}
			}

			return sum
		},
	}
}

func complexityAnalysis[F field.Element[F]](m sc.Module[F]) int {
	var sum big.Int
	//
	for iter := m.Constraints(); iter.HasNext(); {
		ith := iter.Next()
		//
		if c, ok := ith.(term.Costable); ok {
			var val = new(big.Int).SetUint64(uint64(c.Complexity()))
			sum.Add(&sum, val)
		}
	}
	//
	if sum.BitLen() < 63 {
		return int(sum.Int64())
	}
	// overflow
	return math.MaxInt
}
