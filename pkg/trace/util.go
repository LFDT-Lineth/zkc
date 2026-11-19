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
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// QualifiedColumnName returns the fully qualified name of a given column.
func QualifiedColumnName(module string, column string) string {
	if module == "" {
		return column
	}

	return fmt.Sprintf("%s.%s", module, column)
}

// MapShard constructs a copy of the given shard where every column is
// replaced by the result of applying a given function to it.
func MapShard[F field.Element[F]](shard Shard[F], f func(uint, uint, array.Array[F]) array.Array[F]) Shard[F] {
	var modules = slices.Clone(shard.modules)
	//
	for mid, mod := range modules {
		// Clone column array
		var columns = slices.Clone(mod.columns)
		// Assign clone
		modules[mid].columns = columns
		// Map column data
		for cid, data := range mod.columns {
			columns[cid] = f(uint(mid), uint(cid), data)
		}
	}
	//
	return NewShard(modules)
}

// Collect forces every shard of a lazy trace and combines them into a single
// shard holding the complete trace.  Modules are combined as follows: a
// replicated module is identical in every shard and, hence, is taken from the
// first shard only; otherwise, the rows of a module are concatenated in shard
// order (i.e. including any padding rows contained in each shard).  If any
// errors arise from forcing the shards, then those are returned and no shard is
// produced.
//
// NOTE: this holds every shard in memory at once and, hence, defeats the
// purpose of a lazy trace.  It is intended for small traces (e.g. testing).
func Collect[F field.Element[F]](tr LazyTrace[F]) (util.Option[Shard[F]], []error) {
	var (
		shards = make([]Shard[F], tr.Len())
		// Collect all shards into the array
		errors = tr.Apply(func(id uint, shard Shard[F]) {
			shards[id] = shard
		})
	)
	//
	if len(errors) != 0 {
		return util.None[Shard[F]](), errors
	}
	//
	return util.Some(combineShards(shards)), nil
}

func combineShards[F field.Element[F]](shards []Shard[F]) Shard[F] {
	// Check for the easy cases, where there is nothing to combine.
	switch len(shards) {
	case 0:
		return NewShard[F](nil)
	case 1:
		return shards[0]
	}
	//
	var modules = make([]Module[F], shards[0].Width())
	// Sanity check shards are consistent
	for i, shard := range shards {
		if shard.Width() != uint(len(modules)) {
			panic(fmt.Sprintf("shard %d has mismatched module count (%d vs %d)", i, shard.Width(), len(modules)))
		}
	}
	//
	for mid := range modules {
		modules[mid] = combineModule(uint(mid), shards)
	}
	//
	return NewShard(modules)
}

func combineModule[F field.Element[F]](mid uint, shards []Shard[F]) Module[F] {
	var first = shards[0].modules[mid]
	// Replicated modules are identical in every shard.
	if first.descriptor.Replicated {
		return first
	}
	//
	var (
		height  uint
		columns = make([]array.Array[F], len(first.columns))
	)
	// Determine combined height (whilst sanity checking consistency)
	for _, shard := range shards {
		var ith = shard.modules[mid]
		// Sanity checks
		util.Assert(ith.Name() == first.Name(), "malformed shard (incorrect module name)")
		util.Assert(len(ith.columns) == len(columns), "malformed shard (incorrect module width)")
		//
		height += ith.height
	}
	//
	for cid := range columns {
		columns[cid] = combineColumn(mid, uint(cid), shards)
	}
	//
	return Module[F]{height, first.descriptor, columns}
}

func combineColumn[F field.Element[F]](mid uint, cid uint, shards []Shard[F]) array.Array[F] {
	var bitwidth uint
	// Determine a bitwidth which is sufficient for the data of every shard.
	// This must come from the data itself (rather than the column descriptor)
	// as appending a value which does not fit silently truncates it.
	for _, shard := range shards {
		var col = shard.modules[mid].columns[cid]
		// A column which is unassigned in some shard cannot be assigned in the
		// combined shard.
		if col == nil {
			return nil
		}
		//
		bitwidth = max(bitwidth, col.BitWidth())
	}
	//
	var data = array.Alloc[F](bitwidth)
	//
	for _, shard := range shards {
		var col = shard.modules[mid].columns[cid]
		//
		for i := range col.Len() {
			data = data.Append(col.Get(i))
		}
	}
	//
	return data.Build()
}
