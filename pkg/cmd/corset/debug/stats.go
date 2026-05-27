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
	"reflect"

	cmd_util "github.com/consensys/go-corset/pkg/cmd/corset/util"
	"github.com/consensys/go-corset/pkg/ir/air"
	"github.com/consensys/go-corset/pkg/ir/assignment"
	"github.com/consensys/go-corset/pkg/ir/mir"
	"github.com/consensys/go-corset/pkg/schema"
	sc "github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/termio"
)

// PrintStats is used for printing summary information about a constraint set,
// such as the number and type of constraints, etc.
func PrintStats[F field.Element[F]](stack cmd_util.SchemaStack[F]) {
	var (
		schema      = stack.ConcreteSchema()
		summarisers = getSummerisers[F]()
		//
		m   = uint(len(summarisers))
		tbl = termio.NewFormattedTable(2, m)
	)
	// Go!
	for i := uint(0); i < m; i++ {
		ith := summarisers[i]
		row := make([]termio.FormattedText, 2)
		row[0] = termio.NewText(ith.name)

		count := ith.summary(schema)
		row[1] = termio.NewText(fmt.Sprintf("%d", count))

		tbl.SetRow(i, row...)
	}
	//
	tbl.SetMaxWidths(64)
	tbl.Print(true)
}

// ============================================================================
// Schema Summarisers
// ============================================================================

type schemaSummariser[F any] struct {
	name    string
	summary func(sc.AnySchema[F]) int
}

func getSummerisers[F field.Element[F]]() []schemaSummariser[F] {
	return []schemaSummariser[F]{
		// Constraints
		constraintCounter("Constraints", func(sc.Constraint[F]) bool { return true }),
		constraintCounter("Vanishing", isVanishingConstraint[F]),
		constraintCounter("Lookups", isLookupConstraint[F]),
		constraintCounter("Permutations", isPermutationConstraint[F]),
		constraintCounter("Range", isRangeConstraint[F]),
		// Assignments
		assignmentCounter[F]("Computed Columns",
			reflect.TypeOf((*assignment.ComputedRegister[F])(nil))),
		assignmentCounter[F]("Computation Columns",
			reflect.TypeOf((*assignment.NativeComputation[F])(nil))),
		assignmentCounter[F]("Lexicographic Orderings",
			reflect.TypeOf((*assignment.LexicographicSort[F])(nil))),
		assignmentCounter[F]("Sorted Permutations",
			reflect.TypeOf((*assignment.SortedPermutation[F])(nil))),
		// Columns
		columnCounter[F](),
		columnWidthSummariser[F](1, 1),
		columnWidthSummariser[F](2, 4),
		columnWidthSummariser[F](5, 8),
		columnWidthSummariser[F](9, 16),
		columnWidthSummariser[F](17, 32),
		columnWidthSummariser[F](33, 64),
		columnWidthSummariser[F](65, 128),
		columnWidthSummariser[F](129, 256),
	}
}

func isVanishingConstraint[F field.Element[F]](c schema.Constraint[F]) bool {
	switch c := c.(type) {
	case air.VanishingConstraint[F]:
		return true
	case mir.Constraint[F]:
		_, ok := c.Unwrap().(mir.VanishingConstraint[F])
		return ok
	}
	//
	return false
}

func isLookupConstraint[F field.Element[F]](c schema.Constraint[F]) bool {
	switch c := c.(type) {
	case air.LookupConstraint[F]:
		return true
	case mir.Constraint[F]:
		_, ok := c.Unwrap().(mir.LookupConstraint[F])
		return ok
	}
	//
	return false
}

func isPermutationConstraint[F field.Element[F]](c schema.Constraint[F]) bool {
	switch c := c.(type) {
	case air.PermutationConstraint[F]:
		return true
	case mir.Constraint[F]:
		_, ok := c.Unwrap().(mir.PermutationConstraint[F])
		return ok
	}
	//
	return false
}

func isRangeConstraint[F field.Element[F]](c schema.Constraint[F]) bool {
	switch c := c.(type) {
	case air.RangeConstraint[F]:
		return true
	case mir.Constraint[F]:
		_, ok := c.Unwrap().(mir.RangeConstraint[F])
		return ok
	}
	//
	return false
}

func constraintCounter[F any](title string, includes func(schema.Constraint[F]) bool) schemaSummariser[F] {
	return schemaSummariser[F]{
		name: title,
		summary: func(schema sc.AnySchema[F]) int {
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

func assignmentCounter[F field.Element[F]](title string, types ...reflect.Type) schemaSummariser[F] {
	return schemaSummariser[F]{
		name: title,
		summary: func(schema sc.AnySchema[F]) int {
			sum := 0
			for _, t := range types {
				sum += typeOfCounter(schema, t)
			}

			return sum
		},
	}
}

func typeOfCounter[F field.Element[F]](schema sc.AnySchema[F], dyntype reflect.Type) int {
	count := 0

	for m := range schema.Width() {
		for iter := schema.Module(m).Assignments(); iter.HasNext(); {
			ith := iter.Next()
			if dyntype == reflect.TypeOf(ith) {
				count++
			}
		}
	}

	return count
}

func columnCounter[F field.Element[F]]() schemaSummariser[F] {
	return schemaSummariser[F]{
		name: "Columns (all)",
		summary: func(schema sc.AnySchema[F]) int {
			count := 0
			for m := range schema.Width() {
				count += int(schema.Module(m).Width())
			}

			return count
		},
	}
}

func columnWidthSummariser[F field.Element[F]](lowWidth uint, highWidth uint) schemaSummariser[F] {
	return schemaSummariser[F]{
		name: fmt.Sprintf("Columns (%d..%d bits)", lowWidth, highWidth),
		summary: func(schema sc.AnySchema[F]) int {
			count := 0

			for i := schema.Modules(); i.HasNext(); {
				m := i.Next()
				for c := uint(0); c < m.Width(); c++ {
					ith := m.Register(register.NewId(c))
					//
					if ith.IsNative() {
						continue
					}
					//
					ithWidth := ith.Width()
					if ithWidth >= lowWidth && ithWidth <= highWidth {
						count++
					}
				}
			}

			return count
		},
	}
}
