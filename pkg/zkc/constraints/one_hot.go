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
	"cmp"
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
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
//   - a set of single-atom conjuncts ⋁ᵢ (cond ^ bi != 0) covering every bit of a group is
//     replaced by the single conjunct (cond ^ default == 0).
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

// rewriteComplementDisjuncts applies the second rewrite rule:
// turning ⋁ᵢ (cond ^ bi != 0)
// into (cond ^ default == 0) when the disjuncts cover every bit of a group.
func rewriteComplementDisjuncts(cond dfa.BranchCondition, g oneHotGroup) dfa.BranchCondition {
	var (
		conjuncts = cond.Conjuncts()
		buckets   []complementBucket
		replaced  = make([]bool, len(conjuncts))
		rewritten []dfa.BranchCondition
	)
	// Bucket the candidate conjuncts by their remainder.
	for i, conjunct := range conjuncts {
		if bit, rest, ok := splitGroupBitAtom(conjunct, g); ok {
			bucketOf(&buckets, rest, bit.Left.Forwarding).add(bit.Left.Id, i)
		}
	}
	// Only a complete cover of the group's bits may be substituted.
	for _, b := range buckets {
		if len(b.covered) != len(g.bits) {
			continue
		}
		//
		for _, m := range b.members {
			replaced[m] = true
		}
		//
		atoms := append(append([]dfa.BranchEquality{}, b.remainder...),
			logical.EqualsConst(dfa.NewBranchId(b.forwarding, g.dflt), zero))
		rewritten = append(rewritten, conditionOfAtoms(atoms))
	}
	//
	if len(rewritten) == 0 {
		return cond
	}
	// Reassemble: the rewritten conjuncts followed by the untouched ones.
	out := rewritten[0]
	//
	for _, r := range rewritten[1:] {
		out = out.Or(r)
	}
	//
	for i, conjunct := range conjuncts {
		if !replaced[i] {
			out = out.Or(conditionOfAtoms(conjunct.Atoms()))
		}
	}
	//
	return out
}

// oneHotPiece describes one disjunct of a one-hot disjunction split by
// splitOneHotDisjunction: the group bit it tests, plus its guard atoms — the
// disjunct's atoms beyond the shared remainder, each a width-1 register tested
// against zero (so it admits an arithmetic 0/1 indicator).
type oneHotPiece struct {
	bit    register.Id
	guards []dfa.BranchEquality
}

// splitOneHotDisjunction analyses a branch condition 'cond' to see whether it
// is logically equivalent to a condition of the form
//
//	common ∧   ∨  (c ∧ guards_c)
//	          c∈S
//
// where
// - S is a collection of cases c in some one-hot-group G
// - common is what can be factored out independently of the subcase c ∈ S
// - guards_c, c ∈ S, are optional case-dependent refinements of the condition
//
// Exactly one member is ever set, so the disjunction is the sum
// Σ_{c ∈ S} c·⟦guards_c⟧ — which lets pathSelectorConstraint bind
// the selector at degree 1 however large S is.
//
// Each disjunct fires on a set of cases: start from all of them, then (c != 0)
// narrows the set to {c} and (c == 0) drops c from it.  A disjunct with no case
// atom keeps all cases.  S is the union of those sets.
func splitOneHotDisjunction(cond dfa.BranchCondition, groups []oneHotGroup,
) (common []dfa.BranchEquality, pieces []oneHotPiece, ok bool) {
	conjuncts := cond.Conjuncts()
	// A single disjunct already translates cheaply; the arithmetic form only
	// pays off against a genuine disjunction.
	if cond.IsTrue() || cond.IsFalse() || len(conjuncts) < 2 {
		return nil, nil, false
	}
	// Each dispatch allocates fresh bit registers, so groups are disjoint and
	// at most one can match.
	for _, g := range groups {
		if common, pieces, ok = splitDisjuncts(conjuncts, g); ok {
			return common, pieces, true
		}
	}
	//
	return nil, nil, false
}

// splitDisjuncts performs the analysis of splitOneHotDisjunction against a
// single one-hot group.  Every disjunct contributes the members it can fire on
// — no member atom at all means any member, (m != 0) narrows to m, (m == 0)
// removes m — together with the guards it imposes on them.  A member reached
// from several disjuncts keeps the weakest requirement: an unguarded
// occurrence subsumes a guarded one, and two differing guard sets are declined
// (their disjunction is not expressible as a guard list).
func splitDisjuncts(conjuncts []dfa.BranchConjunction, g oneHotGroup,
) (common []dfa.BranchEquality, pieces []oneHotPiece, ok bool) {
	var (
		sets   = make([]map[register.Id]bool, len(conjuncts))
		others = make([][]dfa.BranchEquality, len(conjuncts))
		guards = make([][]dfa.BranchEquality, len(conjuncts))
		byId   = make(map[register.Id][]dfa.BranchEquality)
	)
	// Split every disjunct into the members it fires on and its other atoms.
	for i, conjunct := range conjuncts {
		sets[i] = g.members()
		//
		for _, atom := range conjunct.Atoms() {
			switch {
			case !isIndicatorAtom(atom) || !g.isMember(atom.Left.Id):
				others[i] = append(others[i], atom)
			case atom.Sign:
				// (m == 0): the active member is not m.
				delete(sets[i], atom.Left.Id)
			case sets[i][atom.Left.Id]:
				// (m != 0): the active member is m.
				sets[i] = map[register.Id]bool{atom.Left.Id: true}
			default:
				// (m != 0) contradicting an earlier atom: fires never.
				sets[i] = nil
			}
		}
	}
	// The shared side condition: atoms common to every disjunct (always at
	// least the position atom folded in by lookupSourceSelector).
	common = others[0]
	//
	for _, other := range others[1:] {
		common = intersectAtoms(common, other)
	}
	// Whatever a disjunct requires beyond the shared side condition guards its
	// members, so it must admit an arithmetic 0/1 indicator.
	for i, other := range others {
		for _, atom := range other {
			if containsAtom(common, atom) {
				continue
			} else if !isIndicatorAtom(atom) {
				return nil, nil, false
			}
			//
			guards[i] = append(guards[i], atom)
		}
	}
	// Accumulate each member's guards over the disjuncts it appears in.
	for i, set := range sets {
		for m := range set {
			switch prev, seen := byId[m]; {
			case !seen:
				byId[m] = guards[i]
			case len(prev) == 0 || len(guards[i]) == 0:
				byId[m] = nil
			case !equalAtoms(prev, guards[i]):
				return nil, nil, false
			}
		}
	}
	// An empty member set means the condition cannot fire at all; leave that
	// (unexpected) case to the propositional translation.
	if len(byId) == 0 {
		return nil, nil, false
	}
	// Order the pieces by register id, for determinism.
	ids := make([]register.Id, 0, len(byId))
	//
	for m := range byId {
		ids = append(ids, m)
	}
	//
	slices.SortFunc(ids, func(l, r register.Id) int { return cmp.Compare(l.Unwrap(), r.Unwrap()) })
	//
	for _, m := range ids {
		pieces = append(pieces, oneHotPiece{bit: m, guards: byId[m]})
	}
	//
	return common, pieces, true
}

// members returns the member set of the group: its bits plus the default
// register.
func (g oneHotGroup) members() map[register.Id]bool {
	out := make(map[register.Id]bool, len(g.bits)+1)
	//
	for bit := range g.bits {
		out[bit] = true
	}
	//
	out[g.dflt] = true
	//
	return out
}

// isMember reports whether the given register is a member of the group (one
// of its bits or its default register).
func (g oneHotGroup) isMember(id register.Id) bool {
	return g.bits[id] || id == g.dflt
}

// intersectAtoms returns the atoms of lhs also present in rhs, preserving
// lhs's order.
func intersectAtoms(lhs, rhs []dfa.BranchEquality) []dfa.BranchEquality {
	var out []dfa.BranchEquality
	//
	for _, atom := range lhs {
		if containsAtom(rhs, atom) {
			out = append(out, atom)
		}
	}
	//
	return out
}

// containsAtom reports whether the given atom occurs in the slice.
func containsAtom(atoms []dfa.BranchEquality, atom dfa.BranchEquality) bool {
	for _, ith := range atoms {
		if ith.Cmp(atom) == 0 {
			return true
		}
	}
	//
	return false
}

// isIndicatorAtom reports whether the atom tests a width-1 register against
// the constant zero (either sign), i.e. whether it admits an arithmetic 0/1
// indicator: the register itself for !=, its complement for ==.
func isIndicatorAtom(atom dfa.BranchEquality) bool {
	if !atom.Right.HasSecond() || atom.Left.Width != 1 {
		return false
	}
	//
	value := atom.Right.Second()
	//
	return value.Sign() == 0
}

// complementBucket collects the conjuncts sharing a remainder (their atoms
// minus the single group-bit atom), together with the group bits they cover.
type complementBucket struct {
	remainder  []dfa.BranchEquality
	forwarding bool
	covered    map[register.Id]bool
	members    []int
}

func (p *complementBucket) add(bit register.Id, member int) {
	p.covered[bit] = true
	p.members = append(p.members, member)
}

// bucketOf returns the bucket with the given remainder and forwarding,
// creating it if none exists yet.
func bucketOf(buckets *[]complementBucket, remainder []dfa.BranchEquality,
	forwarding bool) *complementBucket {
	//
	for i := range *buckets {
		b := &(*buckets)[i]
		//
		if b.forwarding == forwarding && equalAtoms(b.remainder, remainder) {
			return b
		}
	}
	//
	*buckets = append(*buckets, complementBucket{remainder, forwarding,
		make(map[register.Id]bool), nil})
	//
	return &(*buckets)[len(*buckets)-1]
}

// splitGroupBitAtom splits a conjunct into its (bit != 0) atom over the group
// and the remaining atoms, provided the conjunct tests exactly one bit of the
// group; otherwise ok is false.
func splitGroupBitAtom(conjunct dfa.BranchConjunction, g oneHotGroup,
) (bit dfa.BranchEquality, rest []dfa.BranchEquality, ok bool) {
	var found int
	//
	for _, atom := range conjunct.Atoms() {
		if isGroupBitAtom(atom, g, false) {
			bit = atom
			found++
		} else {
			rest = append(rest, atom)
		}
	}
	//
	return bit, rest, found == 1
}

// equalAtoms reports whether two (sorted) atom slices are identical.
func equalAtoms(lhs, rhs []dfa.BranchEquality) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	//
	for i := range lhs {
		if lhs[i].Cmp(rhs[i]) != 0 {
			return false
		}
	}
	//
	return true
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
