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
