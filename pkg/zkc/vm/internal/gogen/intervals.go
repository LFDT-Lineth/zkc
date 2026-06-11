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

package gogen

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// intervals is the flow-sensitive bound analysis for one function: an
// inclusive upper bound per register, propagated in program order and joined
// at every goto target.  Its transfer function IS the emitters — the fixpoint
// is computed by dry-emitting the function body to a throwaway buffer until
// the label states stabilise (emitters read bounds via boundOf and report
// writes via assign), so emission and analysis can never disagree.
//
// Soundness leans on two frame facts: frames are zero-allocated (a register
// reads 0 until written — so the boot frame enters with all-zero bounds and a
// callee's locals enter at zero), and every store is width-checked (so a
// register never exceeds its declared width).  Joins use single-shot widening:
// the first visit records the incoming state verbatim, and any later increase
// jumps straight to the register's width bound — loop-varying registers settle
// at their declared width while loop-invariant ones stay tight, and the
// fixpoint terminates in a handful of passes.
type intervals struct {
	disabled bool
	// caps[i] is 2^width(i) - 1, the lattice top for register i.
	caps []*big.Int
	// entry is the state on function entry.
	entry []*big.Int
	// labels holds the joined state at every goto/skip target (nil = not yet
	// reached).
	labels map[pos][]*big.Int
	// cur is the state at the position currently being emitted (nil =
	// unreachable).
	cur []*big.Int
	// changed records whether any label state moved during the current pass.
	changed bool
}

// newIntervals seeds the analysis for a function: the boot frame enters with
// every register zero (CallStack.Boot allocates without copying), a callee
// enters with its inputs bounded by their declared widths (Enter checks them)
// and everything else zero.
func newIntervals(fn *wordFunction, isBoot, disabled bool) *intervals {
	regs := fn.Registers()
	iv := &intervals{
		disabled: disabled,
		caps:     make([]*big.Int, len(regs)),
		entry:    make([]*big.Int, len(regs)),
		labels:   map[pos][]*big.Int{},
	}

	zero := big.NewInt(0)

	for i, r := range regs {
		w := uint(128)
		if !r.IsNative() && r.Width() < 128 {
			w = r.Width()
		}

		iv.caps[i] = widthMax(w)
		iv.entry[i] = zero

		if !isBoot && uint(i) < fn.NumInputs() {
			iv.entry[i] = iv.caps[i]
		}
	}

	return iv
}

// beginPass resets the walk to function entry, keeping the label states
// accumulated by previous passes.
func (iv *intervals) beginPass() {
	iv.changed = false
	iv.cur = append([]*big.Int{}, iv.entry...)
}

// stable reports whether the last pass left every label state unchanged.
func (iv *intervals) stable() bool { return !iv.changed }

// boundOf returns the current upper bound of a register (its width cap when
// the analysis is disabled or the position is unreachable).
func (iv *intervals) boundOf(id register.Id) *big.Int {
	i := id.Unwrap()
	if iv.disabled || iv.cur == nil {
		return iv.caps[i]
	}

	return iv.cur[i]
}

// assign records a write with the given value bound (capped at the register's
// width — the store check guarantees the value fits).  The bound is copied:
// callers may keep mutating the value they pass (e.g. the running remainder of
// a multi-limb store).
func (iv *intervals) assign(id register.Id, bound *big.Int) {
	if iv.disabled || iv.cur == nil {
		return
	}

	i := id.Unwrap()
	iv.cur[i] = new(big.Int).Set(bigMin(bound, iv.caps[i]))
}

// edgeTo merges the current state into a goto/skip target's label state.
func (iv *intervals) edgeTo(p pos) {
	if iv.disabled || iv.cur == nil {
		return
	}

	iv.merge(p, iv.cur)
}

// atLabel is called when emission reaches a labelled position: fall-through
// state (if any) joins the label state, which becomes the current state.
func (iv *intervals) atLabel(p pos) {
	if iv.disabled {
		return
	}

	if iv.cur != nil {
		iv.merge(p, iv.cur)
	}

	if l := iv.labels[p]; l != nil {
		iv.cur = append([]*big.Int{}, l...)
	} else {
		// A label never reached on any pass so far: treat as unreachable.
		iv.cur = nil
	}
}

// endOfFlow marks the positions after an unconditional transfer (jump, skip,
// return, fail) unreachable until the next label.
func (iv *intervals) endOfFlow() {
	if !iv.disabled {
		iv.cur = nil
	}
}

// merge joins a state into a label with single-shot widening.
func (iv *intervals) merge(p pos, state []*big.Int) {
	l := iv.labels[p]
	if l == nil {
		iv.labels[p] = append([]*big.Int{}, state...)
		iv.changed = true

		return
	}

	for i, b := range state {
		if b.Cmp(l[i]) > 0 {
			if l[i].Cmp(iv.caps[i]) != 0 {
				l[i] = iv.caps[i]
				iv.changed = true
			}
		}
	}
}
