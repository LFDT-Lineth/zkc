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
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// TraceLowering simply converts columns from their current big endian word
// representation into the appropriate field representation without performing
// any splitting.  This is only required for traces which are "pre-expanded".
// Such traces typically arise in testing, etc.
func TraceLowering[F field.Element[F]](parallel bool, tf lt.TraceFile) (array.Builder[F], []lt.Module[F]) {
	var (
		stats   = util.NewPerfStats()
		builder array.Builder[F]
		cols    []lt.Module[F]
	)
	//
	if parallel {
		builder, cols = parallelLowering[F](tf)
	} else {
		builder, cols = sequentialLowering[F](tf)
	}
	//
	stats.Log("Trace lowering")
	//
	return builder, cols
}

func sequentialLowering[F field.Element[F]](ltf lt.TraceFile) (array.Builder[F], []lt.Module[F]) {
	var (
		loweredModules = make([]lt.Module[F], ltf.Width())
		builder        = array.NewStaticBuilder[F]()
	)
	//
	for i := range ltf.Width() {
		var (
			m              = ltf.Module(i)
			loweredColumns = make([]lt.Column[F], m.Width())
		)

		for j := range m.Width() {
			loweredColumns[j] = lowerRawColumn(m.Column(j), builder)
		}
		//
		loweredModules[i] = lt.NewModule[F](m.Name(), loweredColumns)
	}
	//
	return builder, loweredModules
}

func parallelLowering[F field.Element[F]](ltf lt.TraceFile) (array.Builder[F], []lt.Module[F]) {
	//
	var (
		// Construct new pool
		builder = array.NewStaticBuilder[F]()
		// Collect all (module, column) pairs into a flat slice.
		jobs = make([]loweringJob, 0, lt.NumberOfColumns(ltf.RawModules()))
	)
	// Build the job list: one job per column across all modules.
	for i := range ltf.Width() {
		var ith = ltf.Module(i)
		for j := range ith.Width() {
			jobs = append(jobs, loweringJob{i, j, ith.Column(j)})
		}
	}
	// Lower all columns in parallel using a worker pool.
	results := array.ParallelMap(jobs, func(_ uint, job loweringJob) result[F] {
		return result[F]{job.module, job.col, lowerRawColumn(job.rawCol, builder)}
	})
	// Construct lowered modules with enough blank columns.
	loweredModules := make([]lt.Module[F], ltf.Width())
	//
	for i := range ltf.Width() {
		var ith = ltf.Module(i)
		//
		loweredModules[i] = lt.NewModule(ith.Name(), make([]lt.Column[F], ith.Width()))
	}
	// Assign lowered columns into their modules.
	for _, res := range results {
		loweredModules[res.module].Columns[res.column] = res.data
	}
	// Done
	return builder, loweredModules
}

// loweringJob represents a single column to be lowered within a module.
type loweringJob struct {
	module uint
	col    uint
	rawCol tr.Column[word.BigEndian]
}

type result[F any] struct {
	module uint
	column uint
	data   lt.Column[F]
}

// lowerRawColumn lowers a given raw column into a given field implementation.
func lowerRawColumn[F field.Element[F]](column tr.Column[word.BigEndian], builder array.Builder[F]) lt.Column[F] {
	var (
		data  = column.Data()
		ndata array.MutArray[F]
	)
	// Observe that computed registers will have nil for their data at this
	// point since they haven't been computed yet.  Therefore, we don't need to
	// do anything with them.
	if data != nil {
		ndata = builder.NewArray(data.Len(), data.BitWidth())
		//
		for i := range data.Len() {
			var val F
			// Initial word from big endian bytes.
			val = val.SetBytes(data.Get(i).Bytes())
			//
			ndata = ndata.Set(i, val)
		}
	}
	//
	return lt.NewColumn(column.Name(), ndata)
}
