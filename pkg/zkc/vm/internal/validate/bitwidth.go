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
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Bitwidth represents the (optional) bitwidth of a given register, where none
// indicates a native (field) register.
type Bitwidth struct {
	// Declared bitwidth of this register.
	bitwidth util.Option[uint]
	// Current bitwidth of this register (zero if native), or none if is
	// undefined.
	current util.Option[uint]
}

// Construct bitwidth for register which may not have been defined.
func undefinedBitwidth(bitwidth util.Option[uint]) Bitwidth {
	return Bitwidth{
		bitwidth: bitwidth,
		current:  util.None[uint](),
	}
}

// Construct bitwidth for register which has definitely been defined.
func definedBitwidth(bitwidth util.Option[uint]) Bitwidth {
	return Bitwidth{
		bitwidth: bitwidth,
		current:  bitwidth,
	}
}

// IsDefined returns true if the corresponding register is definitely assigned
// at the given point.
func (p Bitwidth) IsDefined() bool {
	return p.current.HasValue()
}

// InBounds returns true if the corresponding register holds a value within its
// given bitwidth bounds at the given program point.
func (p Bitwidth) InBounds() bool {
	if p.current.IsEmpty() {
		return false
	} else if p.bitwidth.HasValue() {
		// Bounds check
		return p.current.Unwrap() <= p.bitwidth.Unwrap()
	}
	// Native registers are always in bounds.
	return true
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
		p.current = util.Some(max(p.current.Unwrap(), o.current.Unwrap()))
	} else {
		// not definitely assigned
		p.current = util.None[uint]()
	}
	// Check for change
	return old != p.current
}

// Bitwidths maps each register in a given function to an optional bitwidth.
// This is none if the given register is not yet defined, otherwise it is the
// maximum bitwidth on any path to the given program point.
type Bitwidths struct {
	bitwidths []Bitwidth
}

func newBitwidths[W word.Word[W]](f descriptor.Function[W]) Bitwidths {
	var bitwidths = make([]Bitwidth, f.Width())
	//
	for i, reg := range f.Registers() {
		if reg.IsInput() {
			bitwidths[i] = definedBitwidth(reg.Bitwidth())
		} else {
			bitwidths[i] = undefinedBitwidth(reg.Bitwidth())
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
	}
	// Done
	return Bitwidths{bitwidths}, changed
}

// validate the bitwidth of all registers within the function's control-flow
// graph.  This employs a forward dataflow analysis to propagate bitwidth
// information through the program.
func validateRegisterBitwidth[W word.Word[W]](f descriptor.Function[W]) (errors []error) {
	var (
		// Bitwidths on entry
		init = newBitwidths(f)
		//
		bitwidths = dfa.ForwardDataFlowAnalysis(f, init, transferRegisterWidths)
	)
	// Sanity check the bytecodes of each vector
	for macro, vec := range f.Vectors() {
		for micro, bc := range vec.Bytecodes {
			var (
				// Program point for this bytecode
				pp = ProgramPoint{Macro: uint(macro), Micro: uint(micro)}
				// Register bitwdiths at this program point
				widths = bitwidths.Get(pp)
			)
			// Check bitwidth of each used register is valid.
			for _, r := range bytecodeUses(bc, f) {
				var (
					reg   = f.Register(r)
					width = widths.bitwidths[r]
				)
				//
				if !width.IsDefined() {
					errors = append(errors, fmt.Errorf("register %s not defined at %s", reg.Name(), pp))
				} else if !width.InBounds() {
					errors = append(errors, fmt.Errorf("register %s out-of-bounds at %s", reg.Name(), pp))
				}
			}
			// FIXME: sanity check return bytecodes
		}
	}
	//
	return errors
}

func bytecodeUses[W word.Word[W]](bc Bytecode[W], f descriptor.Function[W]) []RegisterId {
	// For a return bytecode, force the output registers of the enclosing
	// function to be checked.
	if _, ok := bc.(*bytecode.Ret[W]); ok {
		var rids = make([]RegisterId, f.NumOutputs())
		//
		for i := range f.NumOutputs() {
			rids[i] = RegisterId(i + f.NumInputs())
		}
		//
		return rids
	}
	//
	return bc.Uses()
}

func transferRegisterWidths[W word.Word[W]](pp ProgramPoint, bc Bytecode[W], in Bitwidths) []dfa.Transfer[Bitwidths] {
	var (
		edges []dfa.Transfer[Bitwidths]
		next  = pp.Skip(0)
	)
	//
	switch bc := bc.(type) {
	case *bytecode.Fail[W], *bytecode.Ret[W]:
		return nil
	case *bytecode.Jmp[W]:
		// Determine jump target
		target := ProgramPoint{Macro: uint(bc.Target), Micro: 0}
		// Create transfer edge
		edges = append(edges, dfa.NewTransfer(in, target))
	case *bytecode.Skip[W]:
		// Determine target bytecode
		var target = pp.Skip(uint(bc.Skip))
		// Create transfer edge
		edges = append(edges, dfa.NewTransfer(in, target))
	case *bytecode.SkipIf[W]:
		// Determine next logical bytecode
		var target = pp.Skip(uint(bc.Skip))
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(in, next))
		// Create transfer edge
		edges = append(edges, dfa.NewTransfer(in, target))
	case *bytecode.Switch[W]:
		for _, c := range bc.Cases {
			// Determine target bytecode
			var target = pp.Skip(uint(c.Skip))
			// Create transfer edge
			edges = append(edges, dfa.NewTransfer(in, target))
		}
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(in, next))
	case *bytecode.Dispatch[W]:
		for _, c := range bc.Cases {
			// Determine target bytecode
			var target = pp.Skip(uint(c.Skip))
			// Create transfer edge
			edges = append(edges, dfa.NewTransfer(in, target))
		}
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(in, next))
	default:
		var bitwidths = slices.Clone(in.bitwidths)
		// simple transfer function!!!
		for _, r := range bc.Definitions() {
			// FIXME: this is totally broken!!!
			bitwidths[r] = definedBitwidth(bitwidths[r].bitwidth)
		}
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(Bitwidths{bitwidths}, next))
	}
	//
	return edges
}
