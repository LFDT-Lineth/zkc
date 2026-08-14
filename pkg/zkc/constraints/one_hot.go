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

// splitOneHotDisjunction detects a disjunction whose disjuncts are gated on
// the members (bits or the default register) of a single one-hot group,
// returning the atoms shared by every disjunct (rest) and one piece per
// member the disjunction fires on.  Under the group's one-hot invariant
// exactly one member is set, so the pieces are mutually exclusive and
// Σᵢ bitᵢ·⟦guardsᵢ⟧ is exactly the boolean "one of the disjuncts holds",
// which licenses the arithmetic selector binding of pathSelectorConstraint —
// whose degree no longer grows with the number of disjuncts.
//
// Two shapes are recognised:
//
//   - ⋁ᵢ (rest ∧ bitᵢ != 0 ∧ guardsᵢ): distinct positive member tests, each
//     with per-disjunct guards (width-1 tests against zero);
//
//   - ⋁ᵢ (rest ∧ memberAtomsᵢ): unguarded member tests of either sign, where
//     each disjunct denotes the set of members it fires on — (m != 0) narrows
//     it to {m}, (m == 0) removes m — and the pieces are the union of those
//     sets.  This absorbs disjuncts subsumed under one-hot semantics, e.g.
//     (m == 0) ∨ (m' != 0) collapses to the members other than m.
func splitOneHotDisjunction(cond dfa.BranchCondition, groups []oneHotGroup,
) (rest []dfa.BranchEquality, pieces []oneHotPiece, ok bool) {
	conjuncts := cond.Conjuncts()
	// A single disjunct already translates cheaply; the arithmetic form only
	// pays off against a genuine disjunction.
	if cond.IsTrue() || cond.IsFalse() || len(conjuncts) < 2 {
		return nil, nil, false
	}
	// Each dispatch allocates fresh bit registers, so groups are disjoint and
	// at most one can match.
	for _, g := range groups {
		if rest, pieces, ok = splitOneHotDisjuncts(conjuncts, g); ok {
			return rest, pieces, true
		}
		//
		if rest, pieces, ok = splitMemberSetDisjuncts(conjuncts, g); ok {
			return rest, pieces, true
		}
	}
	//
	return nil, nil, false
}

// splitOneHotDisjuncts attempts the split of splitOneHotDisjunction against a
// single one-hot group.
func splitOneHotDisjuncts(conjuncts []dfa.BranchConjunction, g oneHotGroup,
) (rest []dfa.BranchEquality, pieces []oneHotPiece, ok bool) {
	var (
		covered    = make(map[register.Id]bool, len(conjuncts))
		remainders = make([][]dfa.BranchEquality, len(conjuncts))
	)
	// Every disjunct must test exactly one distinct member of the group.
	for i, conjunct := range conjuncts {
		bit, ith, ok := splitGroupMemberAtom(conjunct, g)
		//
		if !ok || covered[bit.Left.Id] {
			return nil, nil, false
		}
		//
		covered[bit.Left.Id] = true
		remainders[i] = ith

		pieces = append(pieces, oneHotPiece{bit: bit.Left.Id})
	}
	// The shared remainder: atoms common to every disjunct (always at least
	// the position atom folded in by lookupSourceSelector).
	rest = remainders[0]
	//
	for _, remainder := range remainders[1:] {
		rest = intersectAtoms(rest, remainder)
	}
	// Whatever a disjunct tests beyond the shared remainder must be a guard.
	for i, remainder := range remainders {
		for _, atom := range remainder {
			if containsAtom(rest, atom) {
				continue
			} else if !isIndicatorAtom(atom) {
				return nil, nil, false
			}
			//
			pieces[i].guards = append(pieces[i].guards, atom)
		}
	}
	//
	return rest, pieces, true
}

// splitMemberSetDisjuncts attempts the split of splitOneHotDisjunction in its
// unguarded form: every disjunct shares the same remainder and constrains the
// group's active member only through member atoms (of either sign).  Each
// disjunct then denotes a set of members — a positive atom (m != 0) narrows it
// to {m}, a negative atom (m == 0) removes m — and, since exactly one member
// is set, the disjunction fires exactly when the active member lies in the
// union of those sets.
func splitMemberSetDisjuncts(conjuncts []dfa.BranchConjunction, g oneHotGroup,
) (rest []dfa.BranchEquality, pieces []oneHotPiece, ok bool) {
	var union = make(map[register.Id]bool)
	//
	for i, conjunct := range conjuncts {
		var (
			set = g.members()
			ith []dfa.BranchEquality
		)
		//
		for _, atom := range conjunct.Atoms() {
			switch {
			case !isIndicatorAtom(atom) || !g.isMember(atom.Left.Id):
				ith = append(ith, atom)
			case atom.Sign:
				// (m == 0): the active member is not m.
				delete(set, atom.Left.Id)
			case set[atom.Left.Id]:
				// (m != 0): the active member is m.
				set = map[register.Id]bool{atom.Left.Id: true}
			default:
				// (m != 0) contradicting an earlier atom: fires never.
				set = nil
			}
		}
		// All disjuncts must share the same remainder.
		if i == 0 {
			rest = ith
		} else if !equalAtoms(rest, ith) {
			return nil, nil, false
		}
		//
		for m := range set {
			union[m] = true
		}
	}
	// An empty union means the condition cannot fire at all; leave that
	// (unexpected) case to the propositional translation.
	if len(union) == 0 {
		return nil, nil, false
	}
	// Order the pieces by register id, for determinism.
	members := make([]register.Id, 0, len(union))
	//
	for m := range union {
		members = append(members, m)
	}
	//
	slices.SortFunc(members, func(l, r register.Id) int { return cmp.Compare(l.Unwrap(), r.Unwrap()) })
	//
	for _, m := range members {
		pieces = append(pieces, oneHotPiece{bit: m})
	}
	//
	return rest, pieces, true
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

// splitGroupMemberAtom splits a conjunct into its (member != 0) atom over the
// group — where a member is either one of the group's bits or its default
// register — and the remaining atoms, provided the conjunct tests exactly one
// member; otherwise ok is false.
func splitGroupMemberAtom(conjunct dfa.BranchConjunction, g oneHotGroup,
) (member dfa.BranchEquality, rest []dfa.BranchEquality, ok bool) {
	var found int
	//
	for _, atom := range conjunct.Atoms() {
		if isGroupMemberAtom(atom, g) {
			member = atom
			found++
		} else {
			rest = append(rest, atom)
		}
	}
	//
	return member, rest, found == 1
}

// isGroupMemberAtom reports whether the given atom tests one of the group's
// members (a bit or the default register) as non-zero.
func isGroupMemberAtom(atom dfa.BranchEquality, g oneHotGroup) bool {
	return !atom.Sign && isIndicatorAtom(atom) &&
		(g.bits[atom.Left.Id] || atom.Left.Id == g.dflt)
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
