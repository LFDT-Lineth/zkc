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
package agnostic

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/poly"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

var (
	zero     big.Int
	one      big.Int
	minusOne big.Int
)

// CombinedWidthOfRegisters returns the combined bitwidth of all limbs.  For example,
// suppose we have three limbs: x:u8, y:u8, z:u11.  Then the combined width is
// 8+8+11=27.
func CombinedWidthOfRegisters(mapping register.Map, registers ...register.LimbId) uint {
	var (
		width uint
	)
	//
	for _, rid := range registers {
		width += mapping.Register(rid).Width()
	}
	//
	return width
}

func init() {
	zero = *big.NewInt(0)
	one = *big.NewInt(1)
	minusOne = *big.NewInt(-1)
}

// ============================================================================
// VariableSplitter
// ============================================================================

// VariableSplitter is responsible for splitting one or more variables into
// smaller limbs using a given maximum bitwidth.  Furthermore, it can be used to
// efficiently substitute those variables for their limbs in a given polynomial.
type VariableSplitter struct {
	// Allocator used for allocating limbs
	mapping RegisterAllocator
	// Bitwidth to split variables
	bitwidth uint
	// Holds limbs for all split variables
	limbs [][]register.Id
	// Holds limb widths for all split variables
	limbWidths [][]uint
}

// NewVariableSplitter constructs a new splitter for a given bitwidth.confui
func NewVariableSplitter(mapping RegisterAllocator, bitwidth uint) VariableSplitter {
	return VariableSplitter{mapping, bitwidth, nil, nil}
}

// SplitVariables splits a given set of variables into limbs of maximum bitwidth
// configured for this splitter.  This produces a set of equations which map
// each variable to its limbs.
func (p *VariableSplitter) SplitVariables(vars bit.Set) (constraints []Equation) {
	// Split each variable in turn
	for iter := vars.Iter(); iter.HasNext(); {
		// Identify variable to split
		var (
			v          = register.NewId(iter.Next())
			constraint Equation
		)
		// Split the variable
		constraint = p.SplitVariable(v)
		// Include constraint needed to enforce split
		constraints = append(constraints, constraint)
	}
	//
	return constraints
}

// SplitVariable splits a single variable into limbs of the given maximum
// bitwidth configured for this splitter, and produces an equation mapping that
// variable to its limbs.
func (p *VariableSplitter) SplitVariable(rid register.Id) Equation {
	//
	var (
		reg = p.mapping.Register(rid)
		//
		lhs DynamicPolynomial
		// Determine necessary widths
		limbWidths = register.LimbWidths(p.bitwidth, reg.Width())
		// Construct filler for limbs
		filler Computation = term.NewRegisterAccess[word.BigEndian, Computation](rid, reg.Width(), 0)
	)
	// Allocate limbs with corresponding filler
	limbs := p.allocate(rid, limbWidths, filler)
	// Construct constraint connecting reg and limbs
	lhs = lhs.Set(poly.NewMonomial(one, rid.AccessOf(reg.Width())))
	// Done
	return NewEquation(lhs, LimbPolynomial(reg.Width(), 0, limbs, limbWidths))
}

// StaticSplitVars splits a given set of variables into limbs of maximum bitwidth
// configured for this splitter.  This produces a set of equations which map
// each variable to its limbs.
func (p *VariableSplitter) StaticSplitVars(vars bit.Set) (constraints []Assignment) {
	// Split each variable in turn
	for iter := vars.Iter(); iter.HasNext(); {
		// Identify variable to split
		var (
			v          = register.NewId(iter.Next())
			assignment Assignment
		)
		// Split the variable
		assignment = p.StaticSplitVar(v)
		// Include constraint needed to enforce split
		constraints = append(constraints, assignment)
	}
	//
	return constraints
}

// StaticSplitVar splits a single variable into limbs of the given maximum
// bitwidth configured for this splitter, and produces an equation mapping that
// variable to its limbs.
func (p *VariableSplitter) StaticSplitVar(rid register.Id) Assignment {
	//
	var (
		reg = p.mapping.Register(rid)
		// Determine necessary widths
		limbWidths = register.LimbWidths(p.bitwidth, reg.Width())
		rhs        StaticPolynomial
	)
	// Allocate limbs with corresponding filler
	limbs := p.allocate(rid, limbWidths, nil)
	// Done
	return NewAssignment(limbs, rhs.Set(poly.NewMonomial(one, rid)))
}

// Apply the splitting to a given polynomial by substituting through all split
// variables for their allocated limbs, whilst leaving all others untouched.
func (p *VariableSplitter) Apply(poly DynamicPolynomial) DynamicPolynomial {
	// Construct cache
	var cache = make(map[register.AccessId]DynamicPolynomial, 0)
	//
	return SubstitutePolynomial(poly, func(reg register.AccessId) DynamicPolynomial {
		var index = reg.Unwrap()
		// Check whether variable is split, or not.
		if index < uint(len(p.limbs)) && p.limbs[index] != nil {
			// Check whether already cached
			if rp, ok := cache[reg]; ok {
				// Yes, so return immediately
				return rp
			}
			// No, so build and cache
			rp := LimbPolynomial(reg.MaskWidth(), reg.RelativeShift(), p.limbs[index], p.limbWidths[index])
			// Cache result
			cache[reg] = rp
			// Done
			return rp
		}
		//
		return nil
	})
}

// StaticApply the splitting to a given polynomial by substituting through all
// split variables for their allocated limbs, whilst leaving all others
// untouched.
func (p *VariableSplitter) StaticApply(poly StaticPolynomial) StaticPolynomial {
	// Construct cache
	var cache = make(map[register.Id]StaticPolynomial, 0)
	//
	return SubstitutePolynomial(poly, func(rid register.Id) StaticPolynomial {
		var index = rid.Unwrap()
		// Check whether variable is split, or not.
		if index < uint(len(p.limbs)) && p.limbs[index] != nil {
			// Check whether already cached
			if rp, ok := cache[rid]; ok {
				// Yes, so return immediately
				return rp
			}
			//
			reg := p.mapping.Register(rid)
			// No, so build and cache
			rp := StaticLimbPolynomial(reg.Width(), p.limbs[index], p.limbWidths[index])
			// Cache result
			cache[rid] = rp
			// Done
			return rp
		}
		//
		return nil
	})
}

// Allocate the limbs for a given register using the provided widths.  This
// records the constructs limbs and widths internally for later use.
func (p *VariableSplitter) allocate(rid register.Id, limbWidths []uint, filler Computation) []register.Id {
	var (
		reg = p.mapping.Register(rid)
		//
		limbs []register.Id
	)
	//
	if filler == nil {
		limbs = p.mapping.AllocateN(reg.Name(), limbWidths)
	} else {
		limbs = p.mapping.AllocateWithN(reg.Name(), filler, limbWidths...)
	}
	//
	if uint(len(p.limbs)) <= rid.Unwrap() {
		nlimbs := make([][]register.Id, 2*(rid.Unwrap()+1))
		nwidths := make([][]uint, 2*(rid.Unwrap()+1))
		//
		copy(nlimbs, p.limbs)
		copy(nwidths, p.limbWidths)
		p.limbs = nlimbs
		p.limbWidths = nwidths
	}
	// Record for later use
	p.limbs[rid.Unwrap()] = limbs
	p.limbWidths[rid.Unwrap()] = limbWidths
	//
	return limbs
}
