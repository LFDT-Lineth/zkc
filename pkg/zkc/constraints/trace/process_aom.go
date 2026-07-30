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
package trace

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// InitAccessOnceMemory initialises a trace module for a read-only (ROM) or
// write-once (WOM) memory: one row per cell with consecutive addresses (single-
// or multi-limb, via splitAddress) plus the cell's data.  The format of a trace
// row for an access-once memory is:
//
// +---------+------+--------+------+------+-----+
// | ADDRESS | DATA | ACCESS | AT_0 | AT_1 | ... |
// +---------+------+--------+------+------+-----+
//
// Here, ADDRESS is the set of declared input registers, whilst DATA is the set
// of declared output registers.
func initAccessOnceMemory[W Word[W], F Element[F], M ModuleBuilder[F, M]](m vm.Memory[W]) (module M) {
	var (
		// Number of address lines
		nAddressLines = m.NumInputs()
		// Copy over all address / data lines
		regs = array.Map(m.Registers(), toRtraceRegister)
		// Bitwidth for binary selector lines
		u1 = util.Some[uint](1)
	)
	// Add access bit for distinguishing padding rows.
	regs = append(regs, rtrace.NewColumnDescriptor(io.ACCESS_BIT_NAME, u1))
	// For multi-lane memory, add one selector bit for each lane
	if nAddressLines > 1 {
		// Only add address selector lines if there is more than one address
		// line.
		for k := range nAddressLines {
			regs = append(regs, rtrace.NewColumnDescriptor(io.AtFlagName(k), u1))
		}
	}
	//
	return module.Initialise(m.Name(), regs)
}

// traceAccessOnceMemory materialises the trace rows for a read-only (ROM) or
// write-once (WOM) memory
func traceAccessOnceMemory[W vm.Word[W], F Element[F]](m vm.RuntimeMemory[W], module Module[F], scratch []F) {
	var (
		one      = field.Uint64[F](1)
		geometry = m.Descriptor()
		// Extract memory contents
		data = m.Contents()
		// Number of address lines
		nAddr = geometry.NumInputs()
		// Number of data lines
		nData = geometry.NumOutputs()
		// Calculate height of trace based on geometry
		height = uint64(len(data)) / uint64(nData)
		//
		width = module.Width()
	)
	// Initialise first row as padding row
	module.Append(paddingRow(scratch[:width])...)
	// Iterate and process each row, one at a time.
	for i := range height {
		var row = scratch[:width]
		// copy over address lines
		copyAddressLines(i, geometry.AddressRegisters(), row)
		// copy over data lines
		copyDataLines(i, geometry.DataRegisters(), data, row[nAddr:])
		// zero out selectors
		zeroOut(row[nAddr+nData:])
		// fill out accept line (always one except in padding)
		row[nAddr+nData] = one
		// check for multi-lane address
		if nAddr > 1 && i > 0 {
			// determine least significant non-zero limb in the address line
			k := findLeastSignificantNonZeroLimb(row[:nAddr])
			// mark pivot for comparison
			row[nAddr+nData+k+1] = one
		}
		// Done
		module.Append(row...)
	}
}

// Given an array of field elements, identify the least significant (i.e.
// highest indexed) non-zero value.  This determines which address selector
// should be enabled.
func findLeastSignificantNonZeroLimb[F Element[F]](row []F) uint {
	// process address lines in reverse order since the most significant line
	// always comes first.
	for i := uint(len(row)); i > 0; i-- {
		if row[i-1].Cmp64(0) != 0 {
			return i - 1
		}
	}
	//
	panic("memory address bandwidth exceeded")
}

func paddingRow[F Element[F]](row []F) []F {
	var zero F
	//
	for i := range row {
		row[i] = zero
	}
	//
	return row
}
