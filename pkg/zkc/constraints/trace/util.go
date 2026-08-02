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

import "github.com/LFDT-Lineth/zkc/pkg/zkc/vm"

// Copy all words out of the given state, and assign them into the give array
// after conversion.
func copyState[W Word[W], F Element[F]](st vm.State[W], fields []F) {
	var frame = st.Frame()
	// Copy over state registers
	for i, v := range frame {
		var val F
		// Copy over data
		// TODO: support larger word sizes.
		fields[i] = val.SetUint64(v.Uint64())
	}
}

// Taken the given address, split it into the necessary components and write
// into the row
func copyAddressLines[W Word[W], F Element[F]](address uint64, lines []vm.Register[W], row []F) {
	var acc vm.Uint64
	// Initialise accumulator
	acc = acc.SetUint64(address)
	// process address lines in reverse order since the most significant line
	// always comes first.
	for i := len(lines); i > 0; i-- {
		var (
			val F
			// determine bitwidth of ith line
			bitwidth = uint64(lines[i-1].Bitwidth().Unwrap())
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
func copyDataLines[W Word[W], F Element[F]](address uint64, lines []vm.Register[W], data []W, row []F) {
	var offset = int(address) * len(lines)
	//
	for i := range lines {
		var val F
		// TODO: support larger word sizes.
		row[i] = val.SetUint64(data[offset+i].Uint64())
	}
}

// Ensure the given array contains only zero values, whilst returning that array
// for convenience.
func zeroOut[F Element[F]](values []F) []F {
	var zero F
	// Copy over state registers
	for i := range values {
		// Copy over data
		values[i] = zero
	}
	//
	return values
}
