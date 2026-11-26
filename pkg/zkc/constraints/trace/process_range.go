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
package trace

import (
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// rangeModule tracks the trace of a recursive range module "$range_uN", so
// that the values held in memory columns of width N can be added to it.  Rows
// reach this module from two sources:
//
//   - Calls executed by the machine.  A function register (or cast) of width N
//     is range-checked by a call "$range_uN(r)" inserted by AddRangeChecks, as
//     are the halves destructured by a wider range module.  Each such call is
//     traced as a row, like that of any other function.
//   - Memory columns.  A memory has no bytecode to host such a call, yet its
//     columns of width N are range-proved by a lookup into this module all the
//     same.  Hence, the values they hold are added whilst the trace of the
//     memory is built.
//
// A value is added at most once: the values already present (whether traced
// from a call, or added from a memory) are recorded, and adding one of them
// again does nothing.
type rangeModule[F Element[F]] struct {
	fn     vm.RangeFunction
	module *trace.ModuleBuilder[F]
	// ret identifies the $ret column, which marks a row as active.
	ret uint
	// lo / hi are the range modules of the two halves, or nil for a half which
	// is narrow enough to be checked against a static table.
	lo, hi *rangeModule[F]
	// tracked indicates whether some memory column is (transitively) checked
	// against this module.  Otherwise, nothing is ever added to it and the
	// values traced into it need not be recorded.
	tracked bool
	// seen records the values present in this module.
	seen map[uint64]struct{}
	// row is scratch space for building a row of this module.
	row []F
}

// rangeColumn identifies a memory column whose values must be added to a
// recursive range module.
type rangeColumn[F Element[F]] struct {
	column uint
	target *rangeModule[F]
}

// initRangeModules locates the recursive range modules of the given program.
// It returns them indexed by module identifier (nil for every other module)
// and, for each memory, the columns which are range-proved against one of them.
func initRangeModules[W Word[W], F Element[F]](program vm.Program[W], modules []*trace.ModuleBuilder[F],
) ([]*rangeModule[F], [][]rangeColumn[F]) {
	var (
		ranges  = make([]*rangeModule[F], len(modules))
		columns = make([][]rangeColumn[F], len(modules))
		byWidth = make(map[uint]*rangeModule[F])
	)
	//
	for width, fn := range vm.RangeFunctions(program) {
		var (
			f      = program.Module(fn.Module).(*vm.Function[W])
			module = modules[fn.Module]
		)
		// The row format assumed by add is that of a one-line function.
		util.Assert(f.IsOneLine(), "range module \"%s\" is not a one-line function", f.Name())
		//
		byWidth[width] = &rangeModule[F]{
			fn:     fn,
			module: module,
			ret:    uint(len(f.Registers())),
			seen:   make(map[uint64]struct{}),
			row:    make([]F, module.Width()),
		}
		ranges[fn.Module] = byWidth[width]
	}
	// Link each module to those of its halves.
	for _, r := range byWidth {
		r.lo, r.hi = byWidth[r.fn.LoWidth], byWidth[r.fn.HiWidth]
	}
	// A memory column needs a recursive range module exactly when it is too
	// wide for a static table, which is when one exists for its width.
	for i, m := range program.Modules() {
		if _, ok := m.(*vm.Memory[W]); !ok {
			continue
		}
		//
		for j, c := range modules[i].Descriptor().Columns {
			if !c.Bitwidth.HasValue() {
				continue
			}
			//
			if target := byWidth[c.Bitwidth.Unwrap()]; target != nil {
				columns[i] = append(columns[i], rangeColumn[F]{uint(j), target})
				target.track()
			}
		}
	}
	//
	return ranges, columns
}

// track marks this module, and those of its halves, as having values added to
// them.
func (p *rangeModule[F]) track() {
	if p == nil || p.tracked {
		return
	}
	//
	p.tracked = true
	p.lo.track()
	p.hi.track()
}

// observe records a value traced into this module by a call.  This is only
// needed for a tracked module.
func (p *rangeModule[F]) observe(value uint64) {
	p.seen[value] = struct{}{}
}

// add ensures this module holds a row for the given value, appending one if
// not, along with the rows needed in turn to range-check its two halves.
//
// NOTE: values are limited to 64 bits, as they are throughout the tracer (see
// wordToField).
func (p *rangeModule[F]) add(value uint64) {
	if _, ok := p.seen[value]; ok {
		return
	}
	//
	p.seen[value] = struct{}{}
	//
	var (
		lo, hi = splitHalves(value, p.fn.LoWidth)
		row    = zeroOut(p.row)
	)
	//
	row[p.fn.Value] = field.Uint64[F](value)
	row[p.fn.Lo] = field.Uint64[F](lo)
	row[p.fn.Hi] = field.Uint64[F](hi)
	row[p.ret] = field.Uint64[F](1)
	//
	p.module.Append(row...)
	//
	if p.lo != nil {
		p.lo.add(lo)
	}
	//
	if p.hi != nil {
		p.hi.add(hi)
	}
}

// addRangeRows adds, for a given row of a memory, the value held in each of the
// given columns to the range module it is checked against.  For a padding row
// (which holds zero in every column), this adds zero.
func addRangeRows[F Element[F]](columns []rangeColumn[F], row []F) {
	for _, c := range columns {
		c.target.add(row[c.column].Uint64())
	}
}

// splitHalves destructures a value into its low half (the least significant
// loWidth bits) and its high half (the remaining bits).
func splitHalves(value uint64, loWidth uint) (lo, hi uint64) {
	return value & ((uint64(1) << loWidth) - 1), value >> loWidth
}
