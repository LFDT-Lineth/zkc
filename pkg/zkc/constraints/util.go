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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

func newLimbsMap[W vm.Word[W]](config field.Config, modules ...vm.Module[W]) module.LimbsMap {
	var ms []register.Map = array.Map(modules, func(_ uint, m vm.Module[W]) register.Map {
		name := trace.ModuleName{Name: m.Name(), Multiplier: 1}
		return register.ArrayMap(name, toRegisters(m.Registers())...)
	})
	// NOTE: generic parameter is meaningless, and only retained for backwards
	// compatibility.
	return module.NewLimbsMap[uint](config, ms...)
}

// ToRegisters converts an array of register descriptors into an array of scheme
// registers.
func toRegisters[W vm.Word[W]](registers []vm.Register[W]) []register.Register {
	var regs = make([]register.Register, len(registers))
	//
	for i, r := range registers {
		var (
			bitwidth uint = math.MaxUint
		)
		// Determine bitwidth (if applicable)
		if !r.IsNative() {
			bitwidth = r.Bitwidth().Unwrap()
		}
		//
		regs[i] = register.New(r.Kind(), r.Name(), bitwidth, *r.Padding().BigInt())
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

		elements[i] = f.SetBytes(w.BigInt().Bytes())
	}
	//
	return elements
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

func fillAddressLines[F field.Element[F]](address uint64, row []F, lines []register.Register) {
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
