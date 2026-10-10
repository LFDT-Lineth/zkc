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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
)

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
				// Register bitwidths at this program point
				widths = bitwidths.Get(pp)
			)
			// Check bitwidth of each used register is valid.
			for _, r := range accessesOf(bc, f, env) {
				var (
					err   error
					loc   = fmt.Sprintf("function %s[%s]", f.Name(), pp)
					reg   = f.Register(r.id)
					width = widths.bitwidths[r.id]
				)
				//
				if !width.IsDefined() {
					err = fmt.Errorf("%s: register %s not defined", loc, reg.Name())
				} else if r.check && !width.InBounds(r.expected) {
					var aStr = accessString(r.expected, width.current.Unwrap())
					//
					err = fmt.Errorf("%s: register %s out-of-bounds (%s)", loc, reg.Name(), aStr)
				} else {
					continue
				}
				//
				errors = append(errors, err)
			}
		}
	}
	//
	return errors
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
	// Sanity check register is defined.
	if current.IsEmpty() {
		return dfa.NewTransfer(in, next)
	}
	// Sanity check
	util.Assert(current.Unwrap().HasValue(), "invalid checkcast on native register")
	// Intersect incoming bitwidth
	bitwidth = min(current.Unwrap().Unwrap(), bitwidth)
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

func accessString(target, actual util.Option[uint]) string {
	var (
		tgt = target.MapOr("𝔽", func(bw uint) string {
			return fmt.Sprintf("u%d", bw)
		})
		//
		act = actual.MapOr("𝔽", func(bw uint) string {
			if bw >= math.MaxUint16 {
				return "?"
			}
			//
			return fmt.Sprintf("u%d", bw)
		})
	)
	//
	return fmt.Sprintf("%s :> %s", tgt, act)
}
