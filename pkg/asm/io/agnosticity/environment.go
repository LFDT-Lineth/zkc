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
package agnosticity

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Register defines the notion of a register within a function.
type Register = register.Register

// RegisterId abstracts the notion of a register id.
type RegisterId = register.Id

// RegisterSplittingEnvironment represents an environment to assist with register splitting.
// Specifically, it maintains the list of registers as they were before
// splitting, along with the list as they are after splitting and, finally, a
// mapping between them.
type RegisterSplittingEnvironment struct {
	// Maximum permitted width for a register.
	maxWidth uint
	// Set of unsplit registers (i.e. as they were before).
	regsBefore []Register
	// Set of split registers (i.e. as they are after splitting).  Observe that
	// this can include extra registers are allocated to implement the split
	// (e.g. for holding carry flags).
	regsAfter []Register
	// Mapping from indices in regsBefore to indices in regsAfter
	regMap []uint
}

// NewRegisterSplittingEnvironment constructs a new register splitting environment for a given set
// of registers and a desired maximum register width.
func NewRegisterSplittingEnvironment(maxWidth uint, registers []Register) *RegisterSplittingEnvironment {
	var (
		// Mapping from old register ids to new register ids.
		mapping []uint = make([]uint, len(registers))
		//
		splitRegisters []Register
	)
	//
	for i, reg := range registers {
		// Map old id to new id
		mapping[i] = uint(len(splitRegisters))
		// Check whether splitting required.
		if reg.Width() > maxWidth {
			// Yes!
			splitRegisters = append(splitRegisters, SplitRegister(maxWidth, reg)...)
		} else {
			splitRegisters = append(splitRegisters, reg)
		}
	}
	//
	return &RegisterSplittingEnvironment{
		maxWidth,
		registers,
		splitRegisters,
		mapping,
	}
}

// MaxWidth returns the maximum permitted register width.
func (p *RegisterSplittingEnvironment) MaxWidth() uint {
	return p.maxWidth
}

// RegistersBefore returns the set of registers as they appear before splitting.
func (p *RegisterSplittingEnvironment) RegistersBefore() []Register {
	return p.regsBefore
}

// RegistersAfter returns the set of registers as they appear after splitting.
func (p *RegisterSplittingEnvironment) RegistersAfter() []Register {
	return p.regsAfter
}

// SplitSourceRegisters splits a given set of source registers into "packets" of
// limbs.  For example, suppose r0 and r1 are source registers of bitwidth
// (respectively) 16bits and 8bits.  Then, splitting for a maximum width of 8
// yields 2 packets: {{r0'0,r1'0}, {r0'1}}
func (p *RegisterSplittingEnvironment) SplitSourceRegisters(sources ...RegisterId) [][]RegisterId {
	ntargets := make([][]RegisterId, MaxNumberOfLimbs(p.maxWidth, p.regsBefore, sources))
	//
	for _, target := range sources {
		ntarget := p.regMap[target.Unwrap()]
		reg := p.regsBefore[target.Unwrap()]
		// Determine split parameters
		n := NumberOfLimbs(p.maxWidth, reg.Width())
		// Split up n limbs
		for j := uint(0); j != n; j++ {
			limbId := register.NewId(ntarget + j)
			ntargets[j] = append(ntargets[j], limbId)
		}
	}
	//
	return ntargets
}

// SplitTargetRegisters splits a set of registers, e.g. for an assignment.  For
// example, suppose we have:
//
// > b,x,y = ...
//
// Where x,y are 16bit registers and b is a 1bit overflow.  For a maximum
// register width of 8bits, the above is transformed into:
//
// > b,x'1,x'0',y'1,y'0 = ...
//
// And this set of expanded target registers is returned.
func (p *RegisterSplittingEnvironment) SplitTargetRegisters(targets ...RegisterId) []RegisterId {
	var ntargets []RegisterId
	//
	for _, target := range targets {
		ntarget := p.regMap[target.Unwrap()]
		reg := p.regsBefore[target.Unwrap()]
		n := NumberOfLimbs(p.maxWidth, reg.Width())
		// Split into n limbs
		for j := uint(0); j != n; j++ {
			limbId := register.NewId(ntarget + j)
			ntargets = append(ntargets, limbId)
		}
	}
	//
	return ntargets
}

// AllocateTargetLimbs allocates upto maxWidth bits from a given set of target
// limbs.
func (p *RegisterSplittingEnvironment) AllocateTargetLimbs(targetLimbs []RegisterId) (uint,
	[]RegisterId, []RegisterId) {
	//
	var (
		width  = uint(0)
		n      = 0
		target = targetLimbs[n].Unwrap()
	)
	// Determine how many limbs to use
	for n < len(targetLimbs) && width+p.regsAfter[target].Width() < p.maxWidth {
		width = width + p.regsAfter[target].Width()
		n++
		//
		if n < len(targetLimbs) {
			target = targetLimbs[n].Unwrap()
		}
	}
	//
	return width, slices.Clone(targetLimbs[:n]), targetLimbs[n:]
}

// AllocateCarryRegister allocates a carry flag to hold bits which "overflow" the
// left-hand side of an assignment (i.e. where sourceWidth is greater than
// targetWidth).
func (p *RegisterSplittingEnvironment) AllocateCarryRegister(targetWidth uint, sourceWidth uint) RegisterId {
	var (
		overflowRegId = uint(len(p.regsAfter))
		// Default padding (for now)
		padding big.Int
	)
	// Sanity check
	if targetWidth > sourceWidth {
		// should be
		panic(fmt.Sprintf("unreachable (target width %d vs source width %d)", targetWidth, sourceWidth))
	} else if targetWidth == sourceWidth {
		// Indicates carry flag not required (e.g. because no carry in lower
		// portion of addition).
		return register.UnusedId()
	}
	// Determine number of bits of overflow
	overflowWidth := sourceWidth - targetWidth
	// Construct register for holding overflow
	overflowRegister := register.NewComputed(fmt.Sprintf("c$%d", overflowRegId), overflowWidth, padding)
	// Allocate overflow register
	p.regsAfter = append(p.regsAfter, overflowRegister)
	//
	return register.NewId(overflowRegId)
}
