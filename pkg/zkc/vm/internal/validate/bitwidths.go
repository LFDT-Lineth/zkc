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
package validate

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

// RegWidth defines the type of "register bitwidths".  The purpose of this is to
// try and make the code a bit more readable.
type RegWidth = util.Option[uint]

// Bitwidth represents the (optional) bitwidth of a given register, where none
// indicates a native (field) register.  This structure maintains an invariant
// which can be checked via the Invariant() method.
type Bitwidth struct {
	// safe registers use dedicated bounds checks to enforce bitwidth and, as
	// such, can be assumed to be always within bounds.
	safe bool
	// Declared bitwidth of this register.
	bitwidth RegWidth
	// Current bitwidth of this register, or none if is undefined.
	current util.Option[RegWidth]
}

// Invariant checks that the internal data structure invariant holds.
func (p Bitwidth) Invariant() {
	var (
		c = p.current
		b = p.bitwidth
	)
	//
	util.Assert(c.IsEmpty() || c.Unwrap().IsEmpty() == b.IsEmpty(),
		"native and uint registers cannot mix")
}

// Construct bitwidth for register which may not have been defined.
func undefinedBitwidth(safe bool, bitwidth RegWidth) Bitwidth {
	return Bitwidth{
		safe:     safe,
		bitwidth: bitwidth,
		current:  util.None[RegWidth](),
	}
}

// Construct bitwidth for register which has definitely been defined.
func definedBitwidth(safe bool, bitwidth RegWidth) Bitwidth {
	return Bitwidth{
		safe:     safe,
		bitwidth: bitwidth,
		current:  util.Some(bitwidth),
	}
}

// IsDefined returns true if the corresponding register is definitely assigned
// at the given point.
func (p Bitwidth) IsDefined() bool {
	return p.current.HasValue()
}

// InBounds returns true if the corresponding register holds a value within the
// given bounds at the given program point.
func (p Bitwidth) InBounds(bitwidth RegWidth) bool {
	if p.current.IsEmpty() {
		return false
	}
	//
	var current = p.current.Unwrap()
	// Sanity check
	util.Assert(current.IsEmpty() == bitwidth.IsEmpty(), "native and uint registers cannot mix")
	// Bounds check
	return bitwidth.IsEmpty() || current.Unwrap() <= bitwidth.Unwrap()
}

// Assign the corresponding register a given width.
func (p Bitwidth) Assign(width RegWidth) Bitwidth {
	// We can assume that safe registers never overflow their bounds, as they
	// are guaranteed to have range constraints.
	if p.safe {
		util.Assert(p.bitwidth.HasValue(), "native registers cannot be safe")
		//
		width = minWidth(width, p.bitwidth)
	}
	//
	p.current = util.Some(width)
	// Check invariant
	p.Invariant()
	//
	return p
}

// Join another bitwidth entry into this, whilst reporting whether or not
// anything changed.
func (p *Bitwidth) Join(o Bitwidth) bool {
	var old = p.current
	// sanity check
	util.Assert(p.bitwidth == o.bitwidth, "malformed bitwidth flow set")
	//
	if p.current.HasValue() && o.current.HasValue() {
		// definitely assigned
		p.current = util.Some(maxWidth(p.current.Unwrap(), o.current.Unwrap()))
	} else {
		// not definitely assigned
		p.current = util.None[RegWidth]()
	}
	// Check invariant
	p.Invariant()
	// Check for change
	return old != p.current
}

// Bitwidths maps each register in a given function to an optional bitwidth.
// This is none if the given register is not yet defined, otherwise it is the
// maximum bitwidth on any path to the given program point.
type Bitwidths struct {
	bitwidths []Bitwidth
}

func newBitwidths[W Word[W]](f descriptor.Function[W]) Bitwidths {
	var bitwidths = make([]Bitwidth, f.Width())
	//
	for i, reg := range f.Registers() {
		if reg.IsInput() || reg.IsZeroWidth() {
			bitwidths[i] = definedBitwidth(reg.IsSafe(), reg.Bitwidth())
		} else {
			bitwidths[i] = undefinedBitwidth(reg.IsSafe(), reg.Bitwidth())
		}
	}
	//
	return Bitwidths{bitwidths}
}

// IsBottom implementation for dfa.FlowSet interface.
func (p Bitwidths) IsBottom() bool {
	return p.bitwidths == nil
}

// Join implementation for dfa.FlowSet interface.
func (p Bitwidths) Join(other Bitwidths) (Bitwidths, bool) {
	var (
		bitwidths []Bitwidth
		changed   = false
	)
	//
	if p.bitwidths != nil {
		bitwidths = slices.Clone(p.bitwidths)
		//
		for i := range other.bitwidths {
			c := bitwidths[i].Join(other.bitwidths[i])
			changed = changed || c
		}
	} else {
		bitwidths = slices.Clone(other.bitwidths)
		changed = !other.IsBottom()
	}
	// Done
	return Bitwidths{bitwidths}, changed
}

// Assign assigns a given number of bits of data from the right-hand side across
// a given register on the left-hand side.
func (p Bitwidths) Assign(lhs RegisterId, rhs RegWidth) Bitwidths {
	var bitwidths = slices.Clone(p.bitwidths)
	//
	if lhs == bytecode.DISCARD {
		// do nothing since value is being discarded
	} else {
		bitwidths[lhs] = p.bitwidths[lhs].Assign(rhs)
	}
	//
	return Bitwidths{bitwidths}
}

// AssignAll assigns a given number of bits of data from the right-hand side
// across a given set of registers on the left-hand side.
func (p Bitwidths) AssignAll(lhs []RegisterId, rhs RegWidth) Bitwidths {
	var bitwidths = slices.Clone(p.bitwidths)
	// Sanity check
	util.Assert(!rhs.IsEmpty() || len(lhs) == 1, "cannot destruct native register")
	//
	if rhs.IsEmpty() {
		// Native register
		bitwidths[lhs[0]] = p.bitwidths[lhs[0]].Assign(rhs)
	} else {
		var bitwidth = rhs.Unwrap()
		//
		for i, l := range lhs {
			var bw uint
			// NOTE: discard cannot (at this time) appear here.  This is because
			// discard registers are currently only suppored for the target(s)
			// of a call.
			util.Assert(l != bytecode.DISCARD, "unexpected discard register")
			//
			if i+1 == len(lhs) {
				bw = bitwidth
			} else {
				bw = p.bitwidths[l].bitwidth.Unwrap()
			}
			//
			bitwidths[l] = p.bitwidths[l].Assign(util.Some(bw))
			// Reduce bitwidth by amount assigned to register
			bitwidth -= min(bitwidth, bw)
		}
	}
	//
	return Bitwidths{bitwidths}
}

func maxWidth(l, r RegWidth) RegWidth {
	util.Assert(l.IsEmpty() == r.IsEmpty(), "native and uint registers cannot mix")
	//
	if l.IsEmpty() || r.IsEmpty() {
		return util.None[uint]()
	}
	//
	return util.Some(max(l.Unwrap(), r.Unwrap()))
}

func minWidth(l, r RegWidth) RegWidth {
	util.Assert(l.IsEmpty() == r.IsEmpty(), "native and uint registers cannot mix")
	//
	if l.IsEmpty() || r.IsEmpty() {
		return util.None[uint]()
	}
	//
	return util.Some(min(l.Unwrap(), r.Unwrap()))
}
