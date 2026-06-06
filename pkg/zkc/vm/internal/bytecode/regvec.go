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
package bytecode

import "fmt"

// RegVec is a "register vector".  That is, a set of n consecutively indexed
// registers.
type RegVec struct {
	// Base identifies the first register in the vector (i.e. that with the
	// least index).
	Base Reg
	// Len identifies the length of the vector.
	Len uint16
}

// NewRegVec constructs a new register vector from an array of registers.  These
// registers must be consecutively indexed, else this will panic.
func NewRegVec(regs ...Reg) RegVec {
	// sanity checks
	for i := 1; i < len(regs); i++ {
		if regs[i-1]+1 != regs[i] {
			panic("register vector must be consecutive")
		}
	}
	//
	return RegVec{
		regs[0],
		uint16(len(regs)),
	}
}

func (p RegVec) String() string {
	switch p.Len {
	case 1:
		return fmt.Sprintf("r%d", p.Base)
	case 2:
		return fmt.Sprintf("r%d;r%d", p.Base, p.Base+1)
	default:
		return fmt.Sprintf("r%d;,,;r%d", p.Base, p.Base+p.Len-1)
	}
}
