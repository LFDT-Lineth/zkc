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
package constraints

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	pow "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// ToRegisters converts an array of register descriptors into an array of scheme
// registers.
func toRegisters[W vm.Word[W], F field.Element[F]](registers []vm.Register[W]) []register.Register[F] {
	var regs = make([]register.Register[F], len(registers))
	//
	for i, r := range registers {
		regs[i] = r.ToRawRegister[F]()
	}
	//
	return regs
}

// toFieldElements converts a slice of words into a slice of field elements.
// This mirrors the memory-contents lowering previously performed by
// WordToFieldMachine.
func toFieldElements[W vm.Word[W], F field.Element[F]](contents []W) []F {
	var elements = make([]F, len(contents))
	//
	for i, w := range contents {
		var f F
		// TODO: support larger fields
		elements[i] = f.SetUint64(w.Uint64())
	}
	//
	return elements
}

// FoldContents folds the contents of a memory into a multi-dimensional representation.
func foldContents[F field.Element[F]](inputs, outputs []register.Register[F], contents []F) [][]F {
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
	for i := range len(contents) {
		var (
			// Determine table row
			row = uint64(i / nOutputs)
			// Determine output index
			output = nInputs + (i % nOutputs)
			// Extract row data
			ith = rows[row]
		)
		// Construct row (if not previously constructed)
		if ith == nil {
			ith = make([]F, nInputs+nOutputs)
			fillAddressLines(row, ith[:nInputs], inputs[:nInputs])
			rows[row] = ith
		}
		//
		ith[output] = contents[i]
	}
	//
	return rows
}

// padStaticTables pads the rows of a static reference table to the next
// power-of-two height, by duplicating the last row.
func padStaticTables[F field.Element[F]](rows [][]F) [][]F {
	if len(rows) == 0 {
		panic("A static table can't be declared empty")
	}
	//
	var target = pow.NextPowerOfTwo(uint(len(rows)))
	//
	for len(rows) < int(target) {
		rows = append(rows, slices.Clone(rows[len(rows)-1]))
	}
	//
	return rows
}

func fillAddressLines[F field.Element[F]](address uint64, row []F, lines []register.Register[F]) {
	// Least signicant word first
	var acc vm.Uint64
	// Initialise accumulator
	acc = acc.SetUint64(address)
	// process address lines in reverse order since the most significant line
	// always comes first.
	for i := len(lines); i > 0; i-- {
		var (
			val F
			// determine bitwidth of ith line
			bitwidth = uint64(lines[i-1].Width())
			// Slice out bitwidth bits
			slice = acc.Slice(uint(bitwidth))
		)
		// Assign u64 (slice as field element)
		row[i-1] = val.SetUint64(slice.Uint64())
		// Shift down address
		acc = acc.Shr64(bitwidth)
	}
}

// newLimbsMap constructs the register → limb mapping for a given (pre-split)
// program, mirroring exactly how register splitting lays out limbs within each
// module.
func newLimbsMap[W vm.Word[W], F field.Element[F]](config field.Config, modules ...vm.Module[W]) module.LimbsMap[F] {
	var ms []register.LimbsMap[F] = array.Map(modules, func(_ uint, m vm.Module[W]) register.LimbsMap[F] {
		var (
			regs  = m.Registers()
			limbs = make([][]register.Limb[F], len(regs))
		)
		// Split registers into limbs
		for i, r := range regs {
			limbs[i] = toRegisters[W, F](vm.SplitIntoLimbs(config.RegisterWidth, r))
		}
		// Construct limbs map
		return register.NewLimbsMap(m.Name(), toRegisters[W, F](regs), limbs)
	})
	//
	return module.NewLimbsMap(ms...)
}
