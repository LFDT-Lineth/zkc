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
	"math"
	"reflect"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

// Bitwidth represents the (optional) bitwidth of a given register, where none
// indicates a native (field) register.
type Bitwidth struct {
	// safe registers use dedicated bounds checks to enforce bitwidth and, as
	// such, can be assumed to be always within bounds.
	safe bool
	// Declared bitwidth of this register.
	bitwidth util.Option[uint]
	// Current bitwidth of this register (zero if native), or none if is
	// undefined.
	current util.Option[uint]
}

// Construct bitwidth for register which may not have been defined.
func undefinedBitwidth(safe bool, bitwidth util.Option[uint]) Bitwidth {
	return Bitwidth{
		safe:     safe,
		bitwidth: bitwidth,
		current:  util.None[uint](),
	}
}

// Construct bitwidth for register which has definitely been defined.
func definedBitwidth(safe bool, bitwidth util.Option[uint]) Bitwidth {
	return Bitwidth{
		safe:     safe,
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
	// We can assume that unsafe registers never overflow their bounds, as they
	// are guaranteed to have range constraints.
	if p.safe && p.bitwidth.HasValue() {
		width = min(width, p.bitwidth.Unwrap())
	}
	//
	p.current = util.Some(width)
	//
	return p
}

func (p Bitwidth) String() string {
	var (
		decl = p.bitwidth.MapOr("𝔽", func(bw uint) string {
			return fmt.Sprintf("u%d", bw)
		})
		//
		actual = p.current.MapOr("?", func(bw uint) string {
			if bw >= math.MaxUint16 {
				return "?"
			}
			//
			return fmt.Sprintf("u%d", bw)
		})
	)
	//
	return fmt.Sprintf("%s :> %s", decl, actual)
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
func (p Bitwidths) Assign(lhs RegisterId, rhs util.Option[uint]) Bitwidths {
	var bitwidths = slices.Clone(p.bitwidths)
	//
	if lhs == bytecode.DISCARD {
		// do nothing since value is being discarded
	} else if rhs.IsEmpty() {
		// Native register
		bitwidths[lhs] = p.bitwidths[lhs].Assign(0)
	} else {
		// Determine actual bits on the right-hand side
		var bitwidth = rhs.Unwrap()
		// Assign bits
		bitwidths[lhs] = p.bitwidths[lhs].Assign(bitwidth)
	}
	//
	return Bitwidths{bitwidths}
}

// AssignAll assigns a given number of bits of data from the right-hand side
// across a given set of registers on the left-hand side.
func (p Bitwidths) AssignAll(lhs []RegisterId, rhs util.Option[uint]) Bitwidths {
	var bitwidths = slices.Clone(p.bitwidths)
	// Sanity check
	util.Assert(!rhs.IsEmpty() || len(lhs) == 1, "cannot destruct native register")
	//
	if rhs.IsEmpty() {
		// Native register
		bitwidths[lhs[0]] = p.bitwidths[lhs[0]].Assign(0)
	} else {
		var bitwidth = rhs.Unwrap()
		//
		for i, l := range lhs {
			var bw uint
			// NOTE: discard cannot (at this time) appear here.  This is because
			// discard registers are currently only support for the target(s) of
			// a call.
			util.Assert(l != bytecode.DISCARD, "unexpected discard register")
			//
			if i+1 == len(lhs) {
				bw = bitwidth
			} else {
				// FIXME: does this make sense?
				bw = p.bitwidths[l].bitwidth.Unwrap()
			}
			//
			bitwidths[l] = p.bitwidths[l].Assign(bw)
			// Reduce bitwidth by amount assigned to register
			bitwidth -= min(bitwidth, bw)
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
					errors = append(errors, fmt.Errorf("register %s::%s not defined at %s", f.Name(), reg.Name(), pp))
				} else if !width.InBounds() {
					errors = append(errors, fmt.Errorf("register %s::%s out-of-bounds at %s (%s)", f.Name(), reg.Name(), pp, width))
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
	case *bytecode.CheckCast[W]:
		// Ignore checkcast opcode, since this is an actual check.
		return nil
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
	case *bytecode.Cat[W]:
		edges = append(edges, transferCat(pp, bc, in, f))
	case *bytecode.CheckCast[W]:
		edges = append(edges, transferCheckCast(pp, bc, in))
	case *bytecode.Debug[W]:
		// Create fall-thru edge
		edges = append(edges, dfa.NewTransfer(in, next))
	case *bytecode.DivMod[W]:
		edges = append(edges, transferDivMod(pp, bc, in, env))
	case *bytecode.FieldArith[W]:
		edges = append(edges, transferFieldArith(pp, bc, in))
	case *bytecode.UintToField[W]:
		edges = append(edges, transferUintToField(pp, bc, in))
	case *bytecode.FieldToUint[W]:
		edges = append(edges, transferFieldToUint(pp, bc, in, env))
	case *bytecode.Intrinsic[W]:
		edges = append(edges, transferIntrinsic(pp, bc, in, env))
	case *bytecode.ReadWrite[W]:
		edges = append(edges, transferReadWrite(pp, bc, in, env))
	default:
		var t = reflect.TypeOf(bc)
		panic(fmt.Sprintf("unknown bytecode (%s)", t.String()))
	}
	//
	return edges
}

func transferArith[W Word[W]](pp ProgramPoint, bc *bytecode.Arith[W], in Bitwidths, env descriptor.RegisterMap[W],
) dfa.Transfer[Bitwidths] {
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
		// NOTE: subtraction can always underflow and, therefore, we cannot
		// assume anything about the result.
		bits = util.Some[uint](math.MaxUint)
	default:
		panic("unknown arithmetic bytecode")
	}
	//
	return dfa.NewTransfer(in.AssignAll(bc.Target, bits), next)
}

func transferBitwise[W Word[W]](pp ProgramPoint, bc *bytecode.Bitwise[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	var (
		next     = pp.Skip(0)
		bitwidth = uint(bc.Bitwidth)
	)
	//
	return dfa.NewTransfer(in.Assign(bc.Target, util.Some(bitwidth)), next)
}

func transferCall[W Word[W]](pp ProgramPoint, bc *bytecode.Call[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	var (
		next = pp.Skip(0)
		// NOTE: following guaranteed by earlier validation phases
		callee  = env.Module(bc.Target).Unwrap().(*descriptor.Function[W])
		returns = callee.Outputs()
	)
	// Assign each return to the corresponding return register.
	for i, v := range bc.Returns {
		in = in.Assign(v, returns[i].Bitwidth())
	}
	//
	return dfa.NewTransfer(in, next)
}

func transferCat[W Word[W]](pp ProgramPoint, bc *bytecode.Cat[W], in Bitwidths, rmap descriptor.RegisterMap[W],
) dfa.Transfer[Bitwidths] {
	var (
		next = pp.Skip(0)
		// Determine bitwidth of right-hand side
		bitwidth = descriptor.BitwidthOf(rmap, bc.Sources...)
	)
	//
	return dfa.NewTransfer(in.AssignAll(bc.Targets, bitwidth), next)
}

func transferCheckCast[W Word[W]](pp ProgramPoint, bc *bytecode.CheckCast[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	var (
		next     = pp.Skip(0)
		current  = in.bitwidths[bc.Target].current
		bitwidth = uint(bc.Bitwidth)
	)
	// Intersect incoming bitwidth
	if current.HasValue() {
		bitwidth = min(current.Unwrap(), bitwidth)
	}
	// Update bitwidth for variable
	in = in.Assign(bc.Target, util.Some(bitwidth))
	//
	return dfa.NewTransfer(in, next)
}

func transferDivMod[W Word[W]](pp ProgramPoint, bc *bytecode.DivMod[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	var (
		next     = pp.Skip(0)
		bitwidth = env.Register(bc.Dividend).Bitwidth()
	)
	//
	in = in.Assign(bc.Quotient, bitwidth)
	// NOTE: following could be made more precise by taking into account maximum
	// bitwidth of the divisor.  However, does not seem worth the effort.
	in = in.Assign(bc.Remainder, bitwidth)
	//
	return dfa.NewTransfer(in, next)
}

func transferFieldArith[W Word[W]](pp ProgramPoint, bc *bytecode.FieldArith[W], in Bitwidths) dfa.Transfer[Bitwidths] {
	var next = pp.Skip(0)
	// Assign target as native register
	return dfa.NewTransfer(in.Assign(bc.Target, util.None[uint]()), next)
}

func transferUintToField[W Word[W]](pp ProgramPoint, bc *bytecode.UintToField[W], in Bitwidths,
) dfa.Transfer[Bitwidths] {
	var next = pp.Skip(0)
	// Assign target as native register
	return dfa.NewTransfer(in.Assign(bc.Target, util.None[uint]()), next)
}

func transferFieldToUint[W Word[W]](pp ProgramPoint, bc *bytecode.FieldToUint[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	var (
		next     = pp.Skip(0)
		bitwidth = env.Field().BitLen()
	)
	// Assign field bits across the target registers
	return dfa.NewTransfer(in.AssignAll(bc.Target, util.Some(bitwidth)), next)
}

func transferIntrinsic[W Word[W]](pp ProgramPoint, bc *bytecode.Intrinsic[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	switch bc.Op {
	case bytecode.DIV_HINT:
		return transferUnsafeIntrinsic(pp, bc, in)
	case bytecode.WIDE_SHL, bytecode.WIDE_SHR, bytecode.WIDE_DIVMOD:
		return transferSafeIntrinsic(pp, bc, in, env)
	default:
		panic("unknown intrinsic bytecode")
	}
}

// Translate an "unsafe intrinsic".  That is one which can be used for tracing
// and be translated into constraints.  As such, the target value are "hints"
// (i.e. they can be given any possible value) and, hence, they must be
// constrained.  To manage this, we simply assign them the maximum possible
// bitwidth.
func transferUnsafeIntrinsic[W Word[W]](pp ProgramPoint, bc *bytecode.Intrinsic[W], in Bitwidths,
) dfa.Transfer[Bitwidths] {
	var next = pp.Skip(0)
	//
	for _, t := range bc.Targets {
		in = in.AssignAll(t.Registers(), util.Some[uint](math.MaxUint))
	}
	//
	return dfa.NewTransfer(in, next)
}

// Translate a "safe intrinsic".  That is one which only exists for fast-mode
// execution, and is never used for tracing or being translated into
// constraints.
func transferSafeIntrinsic[W Word[W]](pp ProgramPoint, bc *bytecode.Intrinsic[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	var next = pp.Skip(0)
	//
	for _, t := range bc.Targets {
		for _, r := range t.Registers() {
			var (
				bitwidth = env.Register(r).Bitwidth()
			)
			//
			in = in.Assign(r, bitwidth)
		}
	}
	//
	return dfa.NewTransfer(in, next)
}

func transferReadWrite[W Word[W]](pp ProgramPoint, bc *bytecode.ReadWrite[W], in Bitwidths, env Environment[W],
) dfa.Transfer[Bitwidths] {
	var (
		next = pp.Skip(0)
		// NOTE: following guaranteed by earlier validation phases
		callee  = env.Module(bc.Id).Unwrap().(*descriptor.Memory[W])
		returns = callee.Outputs()
	)
	// Only reads update local registers
	if !bc.Write {
		// Assign each return to the corresponding return register.
		for i, v := range bc.Data {
			in = in.Assign(v, returns[i].Bitwidth())
		}
	}
	//
	return dfa.NewTransfer(in, next)
}
