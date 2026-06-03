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
package builder

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// TraceSplitting splits a given set of raw columns according to a given
// register mapping or, otherwise, simply lowers them.
func TraceSplitting[F field.Element[F]](parallel bool, tf lt.TraceFile, mapping module.LimbsMap) (array.Builder[F],
	[]lt.Module[F], []error) {
	//
	var (
		stats   = util.NewPerfStats()
		builder array.Builder[F]
		modules []lt.Module[F]
		err     []error
	)
	//
	if parallel {
		builder, modules, err = parallelTraceSplitting[F](tf, mapping)
	} else {
		builder, modules, err = sequentialTraceSplitting[F](tf, mapping)
	}
	//
	stats.Log("Trace splitting")
	//
	return builder, modules, err
}

func sequentialTraceSplitting[F field.Element[F]](ltf lt.TraceFile, mapping module.LimbsMap) (array.Builder[F],
	[]lt.Module[F], []error) {
	//
	var (
		modules = make([]lt.Module[F], ltf.Width())
		// Allocate fresh array builder
		builder = array.NewStaticBuilder[F]()
		errors  []error
	)
	//
	for i := range ltf.Width() {
		var (
			ith     = ltf.Module(i)
			columns []lt.Column[F] // Access mapping for enclosing module
			modmap  = mapping.ModuleOf(ith.Name())
		)
		//
		for j := range ith.Width() {
			var (
				jth         = ith.Column(j)
				split, errs = splitRawColumn(jth, builder, modmap)
			)
			//
			columns = append(columns, split...)
			errors = append(errors, errs...)
		}
		//
		modules[i] = lt.NewModule(ith.Name(), columns)
	}
	//
	return builder, modules, errors
}

func parallelTraceSplitting[F field.Element[F]](ltf lt.TraceFile, mapping module.LimbsMap) (array.Builder[F],
	[]lt.Module[F], []error) {
	//
	var (
		ncols = lt.NumberOfColumns(ltf.RawModules())
		//
		splits = make([][][]lt.Column[F], ltf.Width())
		// Allocate fresh array builder
		builder = array.NewStaticBuilder[F]()
		// Construct a communication channel split columns.
		c = make(chan splitResult[F], ncols)
		//
		errors []error
	)
	// Split column concurrently
	for i := range ltf.Width() {
		var (
			ith = ltf.Module(i)
			// Access mapping for enclosing module
			modmap = mapping.ModuleOf(ith.Name())
		)
		// Initiali split array
		splits[i] = make([][]lt.Column[F], ith.Width())
		//
		for j := range ith.Width() {
			var jth = ith.Column(j)
			// Start go-routine for this column
			go func(mid, cid uint, column tr.Column[word.BigEndian], mapping module.LimbsMap) {
				// Send outcome back
				data, errors := splitRawColumn(column, builder, modmap)
				c <- splitResult[F]{mid, cid, data, errors}
			}(i, j, jth, mapping)
		}
	}
	// Collect results
	for range ncols {
		// Read from channel
		res := <-c
		// Assign split
		splits[res.module][res.column] = res.data
		errors = append(errors, res.errors...)
	}
	// Flatten split
	return builder, flatten(ltf, splits), errors
}

func flatten[W any](tf lt.TraceFile, splits [][][]lt.Column[W]) []lt.Module[W] {
	var (
		modules = make([]lt.Module[W], len(splits))
		index   = 0
	)
	// Flattern all columns
	for i, ith := range splits {
		var columns []lt.Column[W]
		//
		for _, jth := range ith {
			columns = append(columns, jth...)
			index++
		}
		//
		modules[i] = lt.NewModule(tf.Module(uint(i)).Name(), columns)
	}
	//
	return modules
}

// SplitRawColumn splits a given raw column using the given register mapping.
func splitRawColumn[F field.Element[F]](col tr.Column[word.BigEndian], builder array.Builder[F],
	modmap register.LimbsMap) ([]lt.Column[F], []error) {
	//
	var (
		height uint
		//
		reg, regExists = modmap.HasRegister(col.Name())
		// Determine register id for this column (we can assume it exists)
		limbIds = modmap.LimbIds(reg)
		// Determine limbs of this register
		limbs = register.LimbsOf(modmap, limbIds)
	)
	// Check whether register is known
	if !regExists {
		// Unknown register --- this is an error
		return nil, []error{fmt.Errorf("unknown register \"%s\"", col.Name())}
	} else if len(limbIds) == 1 {
		// No, this register was not split into any limbs.  Therefore, no need
		// to split the column into any limbs.
		return []lt.Column[F]{lowerRawColumn(col, builder)}, nil
	}
	// Proceed with splitting column data
	var (
		// Construct temporary place holder for new array data.
		arrays = make([]array.MutArray[F], len(limbIds))
		// Construct final columns
		columns = make([]lt.Column[F], len(limbIds))
	)
	// Check whether data present or not.  Observe computed columns will have
	// nil here (i.e. since their values have not yet been computed).
	if col.Data() != nil {
		// Calculate register height.  Observe that computed registers will have nil
		// for their data at this point since they haven't been computed yet.
		height = col.Data().Len()
		// Yes, must split into two or more limbs of given widths.
		limbWidths := register.WidthsOfLimbs(modmap, modmap.LimbIds(reg))
		//
		for i, limb := range limbs {
			arrays[i] = builder.NewArray(height, limb.Width())
		}
		// Deconstruct all data
		for i := range height {
			// Extract ith data
			ith := col.Data().Get(i)
			// Assign split components
			if !setSplitWord(ith, i, arrays, limbWidths) {
				err := fmt.Errorf("row %d of column %s is out-of-bounds (%s)", i, col.Name(), ith.String())
				return nil, []error{err}
			}
		}
	}
	// Construct final columns
	for i, limb := range limbs {
		columns[i] = lt.NewColumn[F](limb.Name(), arrays[i])
	}
	// Done
	return columns, nil
}

// split a given field element into a given set of limbs, where the least
// significant comes first.  NOTE: this is really a temporary function which
// should be eliminated when RawColumn is moved away from fr.Element.
func setSplitWord[F field.Element[F]](val word.BigEndian, row uint, arrays []array.MutArray[F], widths []uint) bool {
	// FIXME: following is not efficient, as it allocates memory and does quite
	// a lot of work overall.
	var elements, ok = field.SplitWord[F](val, widths)
	// Sanity check split successful
	if ok {
		//
		for i := range widths {
			arrays[i] = arrays[i].Set(row, elements[i])
		}
	}

	return ok
}

// SplitResult is returned by worker threads during parallel trace splitting.
type splitResult[W any] struct {
	module uint
	column uint
	data   []lt.Column[W]
	errors []error
}
