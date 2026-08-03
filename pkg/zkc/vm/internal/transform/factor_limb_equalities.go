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
package transform

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FactorLimbEqualities rewrites each equality SkipIf (EQ/NEQ) comparing two
// multi-limb register vectors, materialising each limb inequality into its own
// 1-bit register and testing the resulting bit vector against zero instead.
//
// After register splitting, the branch condition of
//
//	skip_if x == y S    (x, y split into limbs x1..xn / y1..yn)
//
// translates into the product of one normalisation per limb pair,
// Π (1 - (xk - yk)·inv), of degree 2n: the limb differences are
// sign-indefinite, so their normalisations cannot be merged into a single
// sum-based one.  This rewrite bounds the comparison degree independently of
// the limb count:
//
//	skip_if x1 == y1 2 ; b1 = 1 ; skip 1 ; b1 = 0    (one diamond per limb)
//	...
//	skip_if xn == yn 2 ; bn = 1 ; skip 1 ; bn = 0
//	skip_if b1..bn == 0 S                            (original condition/target)
//
// Each bk holds whether its limb pair *differs* and its writes are guarded by
// a single limb (in)equality (degree 2).  Since the bk are binary and hence
// sign-definite, the limb comparisons of the final condition "b1..bn == 0"
// merge into a single degree-2 normalisation of their sum during MIR lowering
// (unlike the original limb differences).  Each diamond reconverges
// immediately, so path conditions of subsequent codes are unaffected (the
// consensus rule in logical.simplify collapses the complementary branches).
//
// The bits deliberately record inequality — bk = (xk != yk) — rather than the
// more natural equality, so that the final skip compares against zero instead
// of against a vector of ones: comparison against zero is the canonical sign-definite merge in both
// polarities: "b1==0 ∧ .. ∧ bn==0" merges into a single check of sum(bk)
// (see nextSumConjunct), and its negation "b1!=0 ∨ .. ∨ bn!=0" likewise
// merges into a single non-zero check (see lowerDisjunct).  Equality
// against one still merges, but only via the oriented-subtraction rewrite
// of each "bk - 1" into "1 - bk".
//
// Comparisons against constants are deliberately left unchanged: their
// sign-definite limb differences (against zero or maximal limbs, via oriented
// subtraction) are already merged into a single normalisation during MIR
// lowering, which factoring into bits would defeat.
//
// NOTE: this transform must run after SplitRegisters (multi-limb comparisons
// only exist post-split) and before AddRangeConstraints (which constrains the
// fresh 1-bit registers to be binary).
func FactorLimbEqualities[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = factorLimbEqualitiesFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func factorLimbEqualitiesFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
	)

	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, b Bytecode[W]) []Bytecode[W] {
			return factorLimbEqualityCode(b, alloc)
		})
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), nvecs)
}

func factorLimbEqualityCode[W word.Word[W]](b Bytecode[W], registers split.Allocator[W]) []Bytecode[W] {
	si, ok := b.(*bytecode.SkipIf[W])
	if !ok || !isEqualityCondition(si.Op) || !si.Right.IsRegisterVector() {
		return []Bytecode[W]{b}
	}
	//
	var (
		lhs = si.Left.Registers()
		rhs = si.Right.AsRegisters()
	)
	// Only multi-limb comparisons benefit: single-limb comparisons already
	// translate to a single normalisation.  Operands always have the same limb
	// count post-split (see splitSkipIf), hence the length check is defensive.
	if len(lhs) < 2 || len(lhs) != len(rhs) {
		return []Bytecode[W]{b}
	}
	//
	var (
		zero  = word.Const64[W](0)
		one   = word.Const64[W](1)
		n     = len(lhs)
		bits  = make([]bytecode.RegisterId, n)
		zeros = make([]W, n)
		insns []Bytecode[W]
	)
	// Materialise each limb inequality into a fresh bit via a diamond.
	for k := range n {
		bits[k] = registers.Allocate("", util.Some[uint](1))
		zeros[k] = zero
		//
		insns = append(insns,
			// skip_if xk == yk 2  => limbs equal, jump to "bk = 0"
			bytecode.NewSkipIf(bytecode.CONDITION_EQ, 2,
				bytecode.NewRegisterVector(lhs[k]),
				bytecode.NewRegisterOperand[W](rhs[k])),
			// bk = 1  (limbs differ)
			bytecode.LoadConst(bits[k], one),
			// skip 1  => jump over "bk = 0"
			bytecode.NewSkip[W](1),
			// bk = 0  (limbs equal)
			bytecode.LoadConst(bits[k], zero),
		)
	}
	// Finally, re-emit the original skip testing "b1..bn == 0" (respectively
	// "!= 0" for NEQ), which holds exactly when the original condition does.
	return append(insns, bytecode.NewSkipIf(si.Op, si.Skip,
		bytecode.NewRegisterVector(bits...),
		bytecode.NewConstantOperand(zeros...)))
}
