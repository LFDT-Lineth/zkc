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
package post

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// ProcessAccessOnceMemory materializes the trace rows for a read-only (ROM) or
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
func ProcessAccessOnceMemory[W vm.Word[W], F Element[F]](m Memory[W]) rtrace.ArrayModule[F] {
	var (
		one      = field.Uint64[F](1)
		geometry = m.Geometry()
		// Extract memory contents
		data = m.Contents()
		// number of address lines
		nAddr = geometry.AddressLines()
		// number of data lines
		nData = geometry.DataLines()
		// Calculate height of trace based on geometry
		height = uint64(len(data)) / uint64(geometry.DataLines())
		// Determine full set of registers (inc. any selector lines required)
		regs = determineAomRegisters(m.Registers(), geometry.AddressLines())
		// Construct initially empty rows
		rows = make([][]F, height+1)
	)
	// Initialise first row as padding row
	rows[0] = make([]F, len(regs))
	// Iterate and process each row, one at a time.
	for i := range height {
		var row = make([]F, len(regs))
		// copy over address lines
		copyAddressLines(i, geometry.AddressRegisters(), row)
		// copy over data lines
		copyDataLines(i, geometry.DataRegisters(), data, row[nAddr:])
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
		rows[i+1] = row
	}
	//
	return rtrace.NewArrayModule(m.Name(), regs, rows...)
}

// Determine the full set of registers required for the trace of this memory.
func determineAomRegisters(registers []register.Register, nAddressLines uint) []rtrace.Register {
	var (
		// Copy over all address / data lines
		regs = toRtraceRegisters(registers)
		// Bitwidth for binary selector lines
		u1 = util.Some([]uint{1})
	)
	// Add access bit for distinguishing padding rows.
	regs = append(regs, rtrace.NewRegister(io.ACCESS_BIT_NAME, u1))
	// For multi-lane memory, add one selector bit for each lane
	if nAddressLines > 1 {
		// Only add address selector lines if there is more than one address
		// line.
		for k := range nAddressLines {
			regs = append(regs, rtrace.NewRegister(io.AtFlagName(k), u1))
		}
	}
	//
	return regs
}

// Taken the given address, split it into the necessary components and write
// into the row
func copyAddressLines[F Element[F]](address uint64, lines []register.Register, row []F) {
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

// Copy over the row data from the data
func copyDataLines[W Word[W], F Element[F]](address uint64, lines []register.Register, data []W, row []F) {
	var offset = int(address) * len(lines)
	//
	for i := range lines {
		var val F
		//
		row[i] = val.SetBytes(data[offset+i].BigInt().Bytes())
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
