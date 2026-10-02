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
		current:  util.Some(bitwidth.UnwrapOr(0)),
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

// Assign the corresponding register a given width.
func (p Bitwidth) Assign(width uint) Bitwidth {
	p.current = util.Some(width)
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

func newBitwidths[W Word[W]](f descriptor.Function[W]) Bitwidths {
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
		changed = true
	}
	// Done
	return Bitwidths{bitwidths}, changed
}

// Assign a given set of bits across a given set of registers.
func (p Bitwidths) Assign(lhs []RegisterId, rhs util.Option[uint]) Bitwidths {
	var bitwidths = slices.Clone(p.bitwidths)
	// Sanity check
	util.Assert(!rhs.IsEmpty() || len(lhs) == 1, "cannot destruct native register")
	//
	if rhs.IsEmpty() {
		bitwidths[lhs[0]] = p.bitwidths[lhs[0]].Assign(0)
	} else {
		var bitwidth = rhs.Unwrap()
		//
		for i, l := range lhs {
			var bw uint
			if i+1 == len(lhs) {
				bw = bitwidth
			} else {
				// FIXME: does this make sense?
				bw = p.bitwidths[l].bitwidth.Unwrap()
			}
			//
			bitwidths[l] = p.bitwidths[l].Assign(bw)
		}
	}
	//
	return Bitwidths{bitwidths}
}

// validate the bitwidth of all registers within the function's control-flow
// graph.  This employs a forward dataflow analysis to propagate bitwidth
// information through the program.
func validateRegisterBitwidth[W Word[W]](f *descriptor.Function[W], env Environment[W]) (errors []error) {
	var (
		transferFn = func(pp ProgramPoint, bc Bytecode[W], in Bitwidths) []dfa.Transfer[Bitwidths] {
			return transferRegisterWidths(pp, bc, in, f, env)
		}
		// Bitwidths on entry
		init = newBitwidths(*f)
		//
		bitwidths = dfa.ForwardDataFlowAnalysis(*f, init, transferFn)
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
			for _, r := range bytecodeUses(bc, *f, env) {
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

func bytecodeUses[W Word[W]](bc Bytecode[W], f descriptor.Function[W], env Environment[W]) []RegisterId {
	switch bc := bc.(type) {
	case *bytecode.Call[W]:
		var f = env.Module(bc.Target).Unwrap().(*descriptor.Function[W])
		// For an unsafe call, we don't check the argumetns
		if f.HasUnsafeArgs() {
			return nil
		}
	case *bytecode.Ret[W]:
		// For a return bytecode, force the output registers of the enclosing
		// function to be checked.
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

func transferRegisterWidths[W Word[W]](pp ProgramPoint, bc Bytecode[W], in Bitwidths, f *descriptor.Function[W],
	env Environment[W]) []dfa.Transfer[Bitwidths] {
	//
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
	// Non-branching bytecodes
	case *bytecode.Arith[W]:
		edges = append(edges, transferArith(pp, bc, in, f))
	case *bytecode.Bitwise[W]:
		edges = append(edges, transferBitwise(pp, bc, in))
	case *bytecode.Call[W]:
		if !bc.Never {
			edges = append(edges, transferCall(pp, bc, in, env))
		}
	case *bytecode.Debug[W]:
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(in, next))
	case *bytecode.DivRem[W]:
		edges = append(edges, transferDivRem(pp, bc, in))
	case *bytecode.FieldArith[W]:
		edges = append(edges, transferFieldArith(pp, bc, in))
	case *bytecode.UintToField[W]:
		edges = append(edges, transferUintToField(pp, bc, in))
	case *bytecode.FieldToUint[W]:
		edges = append(edges, transferFieldToUint(pp, bc, in))
	case *bytecode.Intrinsic[W]:
		edges = append(edges, transferIntrinsic(pp, bc, in))
	case *bytecode.ReadWrite[W]:
		edges = append(edges, transferReadWrite(pp, bc, in, env))
	default:
		panic("unknown bytecode encountered")
	}
	//
	return edges
}

func transferArith[W Word[W]](pp ProgramPoint, bc *bytecode.Arith[W], in Bitwidths, env descriptor.RegisterMap[W]) dfa.Transfer[Bitwidths] {
	var (
		next    = pp.Skip(0)
		bits    util.Option[uint]
		sources = bc.Source
	)
	//
	switch bc.Op {
	case bytecode.OP_ADD:
		bits = descriptor.CalculateAddBitwidth(sources, bc.Constant, env)
	case bytecode.OP_MUL:
		bits = descriptor.CalculateMulBitwidth(sources, bc.Constant, env)
	case bytecode.OP_SUB:
		bits = descriptor.CalculateSubBitwidth(sources, bc.Constant, env)
	default:
		panic("unknown arithmetic instruction")
	}
	//
	return dfa.NewTransfer(in.Assign(bc.Target, bits), next)
}

func transferBitwise[W Word[W]](pp ProgramPoint, bc *bytecode.Bitwise[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferCall[W Word[W]](pp ProgramPoint, bc *bytecode.Call[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferDivRem[W Word[W]](pp ProgramPoint, bc *bytecode.DivRem[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferFieldArith[W Word[W]](pp ProgramPoint, bc *bytecode.FieldArith[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferUintToField[W Word[W]](pp ProgramPoint, bc *bytecode.UintToField[W], in Bitwidths,
) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferFieldToUint[W Word[W]](pp ProgramPoint, bc *bytecode.FieldToUint[W], in Bitwidths,
) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferIntrinsic[W Word[W]](pp ProgramPoint, bc *bytecode.Intrinsic[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	panic("got here")
}

func transferReadWrite[W Word[W]](pp ProgramPoint, bc *bytecode.ReadWrite[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	panic("got here")
}
