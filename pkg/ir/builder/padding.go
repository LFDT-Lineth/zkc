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
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// padModules pads every (non-static) module in mods up to its target height,
// as determined by the padding strategy in the given config.  Static
// reference tables are left completely untouched, since their (fixed)
// contents are determined entirely by the schema and never need padding.
//
// Every non-static module is always detached from whatever trace alignModule
// aligned it from, even when no padding is actually required (i.e. when the
// computed front padding is zero): alignModule may have aliased its columns
// directly from the trace given to AlignAndPad, and that trace can be shared
// with (and reused by) the caller, so the modules returned from here must
// never retain that aliasing.
func padModules[F field.Element[F]](config Config, schema sc.AnySchema[F], mods []ArrayModule[F],
) ([]ArrayModule[F], []error) {
	var (
		columns, errs = flattenTrace(schema, trace.NewArray(mods))
		data          []array.MutArray[F]
		mapfn         = paddingMapFn(config, schema, mods)
	)
	//
	if config.Parallel {
		data = array.ParallelMap(columns, mapfn)
	} else {
		data = array.Map(columns, mapfn)
	}
	//
	return rebuildModules(mods, columns, data), errs
}

// paddingMapFn constructs the per-column function used to pad a single column
// identified by its (module,column) pair, as produced by flattenTrace.
// Columns belonging to a static module, and columns which are not yet
// assigned (e.g. an unfilled computed column, prior to expansion), are passed
// through unchanged.
func paddingMapFn[F field.Element[F]](config Config, schema sc.AnySchema[F], mods []ArrayModule[F],
) func(uint, util.Pair[uint, uint]) array.MutArray[F] {
	return func(_ uint, p util.Pair[uint, uint]) array.MutArray[F] {
		var (
			mid   = p.Left
			col   = mods[mid].MutColumn(p.Right)
			scMod = schema.Module(mid)
		)
		//
		if col == nil || scMod.IsStatic() {
			return col
		}
		//
		var (
			zero   F
			height = mods[mid].Height()
			target = config.Padding(height, 1)
			front  uint
		)
		// Only pad when the module falls short of its target.
		if target > height {
			front = target - height
		}
		//
		return col.Pad(front, 0, zero)
	}
}

// rebuildModules regroups a flat array of (possibly padded) columns -- as
// produced by mapping over the (module,column) pairs from flattenTrace --
// back into their enclosing modules, using each module's original descriptor
// (which padding never changes).
func rebuildModules[F field.Element[F]](mods []ArrayModule[F], columns []util.Pair[uint, uint],
	padded []array.MutArray[F]) []ArrayModule[F] {
	var (
		result  = make([]ArrayModule[F], len(mods))
		buffers = make([][]array.MutArray[F], len(mods))
	)
	// Regroup padded columns by their enclosing module.
	for i, p := range columns {
		var mid = p.Left
		//
		if buffers[mid] == nil {
			buffers[mid] = make([]array.MutArray[F], mods[mid].Width())
		}
		//
		buffers[mid][p.Right] = padded[i]
	}
	// Reconstruct each module using its original descriptor.
	for mid, mod := range mods {
		result[mid] = trace.NewCompactModule(mod.Descriptor(), buffers[mid]...)
	}
	//
	return result
}
