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
package constraints

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// oneHotGroup records the one-hot register family declared by a single
// Dispatch bytecode: its case bits together with the default register, which
// the enclosing vector constrains to be the complement of the bits' sum (see
// bytecode.Dispatch and transform.LowerSwitch).  On any trace satisfying those
// constraints, "default != 0" holds exactly when every bit is clear.
type oneHotGroup struct {
	// bits of the group (the dispatch's case bits).
	bits map[register.Id]bool
	// dflt is the default register: 1 exactly when every bit is clear.
	dflt register.Id
}

// collectOneHotGroups gathers the one-hot groups declared by the Dispatch
// bytecodes of a vector.  The groups may only be used to rewrite branch
// conditions drawn from that same vector, since the constraints backing the
// one-hot invariant are emitted for its rows.
func collectOneHotGroups[W vm.Word[W]](codes []vm.Bytecode[W]) []oneHotGroup {
	var groups []oneHotGroup
	//
	for _, code := range codes {
		if d, ok := code.(*vm.BytecodeDispatch[W]); ok {
			bits := make(map[register.Id]bool, len(d.Cases))
			//
			for _, c := range d.Cases {
				bits[register.NewId(uint(c.Bit))] = true
			}
			//
			groups = append(groups, oneHotGroup{bits, register.NewId(uint(d.Default))})
		}
	}
	//
	return groups
}

// rewriteOneHotConditions substitutes, within a branch condition, the
// syntactic complement forms produced by Dispatch fall-through edges with the
// equivalent single atom over the dispatch's default register:
//
//   - a conjunct containing (b == 0) for every bit of a group has those atoms
//     replaced by (default != 0), turning the degree-n default-body guard into
//     a single degree-1 atom;
//
//   - a set of single-atom conjuncts (b != 0) covering every bit of a group is
//     replaced by the single conjunct (default == 0) — the shape the
//     complement takes after negation (e.g. within constancy conditions).
//
// Both substitutions preserve the condition's meaning on any trace satisfying
// the enclosing vector's constraints (default = 1 - sum of bits, everything a
// bit), but NOT on arbitrary traces: they must only be applied to conditions
// of bytecodes within the vector declaring the group.  The rewrite happens
// here — at the constraint-translation boundary — rather than in the DFA,
// because the complement form is what lets path conditions cancel where the
// dispatch's edges rejoin; substituting the default atom there instead would
// burden every bytecode after the join with an irreducible disjunction.
func rewriteOneHotConditions(cond dfa.BranchCondition, groups []oneHotGroup) dfa.BranchCondition {
	// Nothing to rewrite in a trivial condition (and the rules below never
	// produce one from a non-trivial condition).
	if cond.IsTrue() || cond.IsFalse() {
		return cond
	}
	// Each dispatch allocates fresh bit registers, so groups are disjoint and
	// the rules apply independently per group.
	for _, g := range groups {
		cond = rewriteComplementConjuncts(cond, g)
		cond = rewriteComplementDisjuncts(cond, g)
	}
	//
	return cond
}

// rewriteComplementConjuncts applies the first rewrite rule: within each
// conjunct, a complete cover of (bit == 0) atoms becomes (default != 0).
func rewriteComplementConjuncts(cond dfa.BranchCondition, g oneHotGroup) dfa.BranchCondition {
	var (
		out     dfa.BranchCondition
		changed bool
	)
	//
	for i, conjunct := range cond.Conjuncts() {
		ith, ch := rewriteConjunct(conjunct, g)
		changed = changed || ch
		//
		if i == 0 {
			out = ith
		} else {
			out = out.Or(ith)
		}
	}
	//
	if !changed {
		return cond
	}
	//
	return out
}

// rewriteConjunct rewrites a single conjunct, reporting whether a substitution
// took place.  Conjuncts covering only a strict subset of the group's bits are
// left untouched, since the equivalence only holds for the full complement.
func rewriteConjunct(conjunct dfa.BranchConjunction, g oneHotGroup) (dfa.BranchCondition, bool) {
	var (
		kept       []dfa.BranchEquality
		matched    int
		forwarding bool
	)
	//
	for _, atom := range conjunct.Atoms() {
		if isGroupBitAtom(atom, g, true) {
			matched++
			forwarding = atom.Left.Forwarding
		} else {
			kept = append(kept, atom)
		}
	}
	// Only a complete cover of the group's bits may be substituted.
	if matched != len(g.bits) {
		return conditionOfAtoms(conjunct.Atoms()), false
	}
	//
	kept = append(kept, logical.NotEqualsConst(dfa.NewBranchId(forwarding, g.dflt), zero))
	//
	return conditionOfAtoms(kept), true
}

// rewriteComplementDisjuncts applies the second rewrite rule: a set of
// single-atom (bit != 0) conjuncts covering the whole group becomes the single
// conjunct (default == 0).
func rewriteComplementDisjuncts(cond dfa.BranchCondition, g oneHotGroup) dfa.BranchCondition {
	var (
		covered    = make(map[register.Id]bool)
		forwarding bool
	)
	//
	for _, conjunct := range cond.Conjuncts() {
		if atoms := conjunct.Atoms(); len(atoms) == 1 && isGroupBitAtom(atoms[0], g, false) {
			covered[atoms[0].Left.Id] = true
			forwarding = atoms[0].Left.Forwarding
		}
	}
	// Only a complete cover of the group's bits may be substituted.
	if len(covered) != len(g.bits) {
		return cond
	}
	//
	out := logical.NewProposition(logical.EqualsConst(dfa.NewBranchId(forwarding, g.dflt), zero))
	//
	for _, conjunct := range cond.Conjuncts() {
		if atoms := conjunct.Atoms(); len(atoms) == 1 && isGroupBitAtom(atoms[0], g, false) {
			continue
		}
		//
		out = out.Or(conditionOfAtoms(conjunct.Atoms()))
	}
	//
	return out
}

// isGroupBitAtom reports whether the given atom tests one of the group's bits
// against the constant zero, with the given sign (true for ==, false for !=).
func isGroupBitAtom(atom dfa.BranchEquality, g oneHotGroup, sign bool) bool {
	if atom.Sign != sign || !atom.Right.HasSecond() || atom.Left.Width != 1 {
		return false
	}
	//
	value := atom.Right.Second()
	//
	return value.Sign() == 0 && g.bits[atom.Left.Id]
}

// conditionOfAtoms reconstructs a branch condition holding the conjunction of
// the given atoms.
func conditionOfAtoms(atoms []dfa.BranchEquality) dfa.BranchCondition {
	var out = dfa.TRUE
	//
	for i, atom := range atoms {
		ith := logical.NewProposition(atom)
		//
		if i == 0 {
			out = ith
		} else {
			out = out.And(ith)
		}
	}
	//
	return out
}

// Alias for the big integer representation of 0.
var zero big.Int = *big.NewInt(0)
