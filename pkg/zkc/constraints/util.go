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
package constraints

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

func newLimbsMap(config field.Config, modules ...vm.Module) module.LimbsMap {
	var ms []register.Map = array.Map(modules, func(_ uint, m vm.Module) register.Map {
		name := trace.ModuleName{Name: m.Name(), Multiplier: 1}
		return register.ArrayMap(name, m.Registers()...)
	})
	// NOTE: generic parameter is meaningless, and only retained for backwards
	// compatibility.
	return module.NewLimbsMap[uint](config, ms...)
}

// FoldContents folds the contents of a memory into a multi-dimensional representation.
func foldContents[F field.Element[F]](inputs, outputs []register.Register, contents []F) [][]F {
	var (
		nInputs  = len(inputs)
		nOutputs = len(outputs)
		nRows    = len(contents) / nOutputs
	)
	// Compute upper bound
	if nRows*nOutputs != len(contents) {
		nRows++
	}
	//
	rows := make([][]F, nRows)
	//
	for i := 0; i < len(contents); i++ {
		var (
			// Determine table row
			row = uint(i / nOutputs)
			// Determine output index
			output = nInputs + (i % nOutputs)
			// Extract row data
			ith = rows[row]
		)
		// Construct row (if not previously constructed)
		if ith == nil {
			ith = make([]F, nInputs+nOutputs)
			fillAddressLine(row, ith, inputs)
			rows[row] = ith
		}
		//
		ith[output] = contents[i]
	}
	//
	return rows
}

func fillAddressLine[F field.Element[F]](index uint, row []F, inputs []register.Register) {
	var address F
	//
	if len(inputs) != 1 {
		panic("support multi-address static memories")
	}
	//
	row[0] = address.SetUint64(uint64(index))
}
