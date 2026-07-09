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
package rtrace

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// register describes an individual register in a row-major trace module.
type register struct {
	// Holds the name of this register.
	name string
	// Holds the optional bitwidth of this register.
	bitwidth util.Option[uint]
	// Holds the limbs backing this register.
	limbs []limb
}

var _ Register = register{}

// NewRegister constructs a register with the given name and limb bitwidths.
// If no limb bitwidths are provided, the register is treated as native and has
// one limb without a fixed bitwidth.  Otherwise, the register bitwidth is the
// sum of the given limb bitwidths.
func NewRegister(name string, limbs util.Option[[]uint]) Register {
	var (
		regLimbs  []limb
		totalSize uint
	)
	//
	if limbs.IsEmpty() {
		return register{name, util.None[uint](), []limb{
			newLimb(0, 0, util.None[uint]()),
		}}
	}
	//
	widths := limbs.Unwrap()
	if len(widths) == 0 {
		panic("register must have at least one limb")
	}

	totalSize, regLimbs = constructFixedWidthLimbs(widths)

	return register{name, util.Some(totalSize), regLimbs}
}

// constructFixedWidthLimbs constructs one provisional limb for each fixed-width
// limb bitwidth and returns their combined register bitwidth.  Limb and
// register identifiers are provisional here; NewArrayModule assigns final
// module-wide identifiers when the descriptor is attached to a module.
func constructFixedWidthLimbs(widths []uint) (uint, []limb) {
	var (
		limbs     []limb
		totalSize uint
	)
	//
	for i, width := range widths {
		if totalSize+width < totalSize {
			panic("register bitwidth overflow")
		}
		//
		totalSize += width
		limbs = append(limbs, newLimb(uint(i), 0, util.Some(width)))
	}
	//
	return totalSize, limbs
}

func newRegisterFromDescriptor(reg Register, registerId uint, firstLimbId uint) (register, []limb) {
	var (
		sourceLimbs = reg.Limbs().Collect()
		regLimbs    = make([]limb, len(sourceLimbs))
		totalSize   uint
		nativeLimbs uint
	)
	//
	if len(sourceLimbs) == 0 {
		panic(fmt.Sprintf("register %q must have at least one limb", reg.Name()))
	}
	//
	for i, l := range sourceLimbs {
		bitwidth := l.Bitwidth()
		//
		if bitwidth.IsEmpty() {
			nativeLimbs++
		} else {
			width := bitwidth.Unwrap()
			if totalSize+width < totalSize {
				panic("register bitwidth overflow")
			}
			//
			totalSize += width
		}
		//
		regLimbs[i] = newLimb(firstLimbId+uint(i), registerId, bitwidth)
	}
	//
	if nativeLimbs > 1 {
		panic(fmt.Sprintf("register %q has multiple limbs without bitwidths", reg.Name()))
	} else if nativeLimbs == 1 {
		return register{reg.Name(), util.None[uint](), regLimbs}, regLimbs
	}
	//
	return register{reg.Name(), util.Some(totalSize), regLimbs}, regLimbs
}

// Name returns the name of this register.
func (p register) Name() string {
	return p.name
}

// Bitwidth returns the optional bitwidth of this register.
func (p register) Bitwidth() util.Option[uint] {
	return p.bitwidth
}

// Limbs returns the limbs backing this register.
func (p register) Limbs() iter.Iterator[Limb] {
	arr := iter.NewArrayIterator(p.limbs)
	return iter.NewCastIterator[limb, Limb](arr)
}

// ----------------------------------------------------------------------------

// limb describes an individual limb in a row-major trace module.
type limb struct {
	// Holds the identifier of this limb.
	limbId uint
	// Holds the identifier of the register backing this limb.
	registerId uint
	// Holds the optional bitwidth of this limb.
	bitwidth util.Option[uint]
}

var _ Limb = limb{}

func newLimb(limbId uint, registerId uint, bitwidth util.Option[uint]) limb {
	return limb{limbId, registerId, bitwidth}
}

// LimbId returns the identifier of this limb.
func (p limb) LimbId() uint {
	return p.limbId
}

// RegisterId returns the identifier of the register backing this limb.
func (p limb) RegisterId() uint {
	return p.registerId
}

// Bitwidth returns the optional bitwidth of this limb.
func (p limb) Bitwidth() util.Option[uint] {
	return p.bitwidth
}
