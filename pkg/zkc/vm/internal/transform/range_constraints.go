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
	"fmt"
	"math/big"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// MAX_STATIC_RANGE_WIDTH is the largest register width for which a range proof
// is provided directly by a fully-enumerated static table (range_u1 ..
// range_u16).  Registers wider than this are range-checked by recursively
// destructuring them into two roughly-equal halves, each of which is itself
// range-checked.
const MAX_STATIC_RANGE_WIDTH = 16

// Register names used within generated range modules.  The "value" column is
// the lookup recipient in both static tables and recursive modules, so callers
// wiring the (deferred) lookups can rely on a single, uniform target name.
const (
	rangeValueName = "value"
	rangeIndexName = "index"
	rangeLoName    = "lo"
	rangeHiName    = "hi"
)

// AddRangeConstraints adds, for every distinct register width occurring in the
// machine, a "range" module which acts as the recipient of a range-proof
// lookup for registers of that width.  Two flavours of range module are
// generated:
//
//   - For a width n <= 16, range_un is a static table enumerating every valid
//     value 0 .. 2^n - 1 in a single value column.
//
//   - For a width n > 16, range_un destructures the value into two halves
//     (lo of n/2 bits, hi of n-n/2 bits) via the constraint value = hi::lo,
//     and (later) range-checks each half by lookups into range_u{lo} and
//     range_u{hi}.  This recursion bottoms out at the static tables above.
//
// Native (field-element) registers have no fixed bitwidth and are range-checked
// against the field bandwidth.
//
// The lookups themselves (both the external register -> value lookups and the
// internal lo/hi -> range_u{lo}/range_u{hi} lookups) are intentionally not
// emitted here yet; this pass only materialises the module structures and the
// destructuring constraints.
//
// This pass must run after SplitRegisters.
func AddRangeConstraints[W word.Word[W]](cfg field.Config, m *machine.Word[W]) *machine.Word[W] {
	var (
		modules = m.Modules()
	)

	// First generate the range modules for every width which occurs on some register.
	var extra = generateRangeModules[W](cfg, modules)

	// Second step, call range_uX for every registers.
	modules = addRangeCalls[W](cfg, modules, extra)

	// Reassemble the machine with the original modules plus the range modules.
	return machine.NewWord[W](cfg, append(slices.Clone(modules), extra...)...)
}

func generateRangeModules[W word.Word[W]](cfg field.Config, modules []Module) []Module {
	var (
		// Every width requiring a range module, mapped to its decomposition.
		splits = neededRangeWidths[W](cfg, modules)
		// Freshly generated range modules.
		extra []Module
		// Widths in ascending order, for deterministic output.
		widths = make([]uint, 0, len(splits))
	)
	// First step, generate the range_uX modules for every width X which occurs on some register.
	for w := range splits {
		widths = append(widths, w)
	}
	//
	slices.Sort(widths)
	// Assign each range module its final index in the module list.  Range modules
	// are appended after the original modules in ascending width order, so a
	// width's module id is fixed once the ordering is known.  This lets the
	// recursive range modules emit calls referencing their (lo/hi) sub-modules.
	var (
		base     = uint(len(modules))
		moduleOf = make(map[uint]uint, len(widths))
	)
	//
	for k, w := range widths {
		moduleOf[w] = base + uint(k)
	}
	//
	for _, w := range widths {
		var name = fmt.Sprintf("$range_u%d", w)
		//
		if w <= MAX_STATIC_RANGE_WIDTH {
			extra = append(extra, newStaticRangeTable[W](name, w))
		} else {
			extra = append(extra, newRecursiveRangeModule[W](name, w, splits[w], moduleOf))
		}
	}

	return extra
}

// rangeSplit records how a width > 16 is destructured into a low half and a high
// half, where lo + hi == width.  For leaf widths (<= 16) both fields are zero.
type rangeSplit struct {
	lo, hi uint
}

// neededRangeWidths computes the decomposition of every register width for which
// a range module must be generated.  This is the set of widths occurring
// directly on some register, closed under the destructuring of wide widths
// (> 16); each wide width is mapped to the (lo, hi) halves it is destructured
// into, while leaf widths (<= 16) map to the zero split.  Constant registers are
// pinned to a fixed value and require no range proof, so are ignored.
func neededRangeWidths[W word.Word[W]](cfg field.Config, modules []Module) map[uint]rangeSplit {
	var (
		// Final decompositions, keyed by width (one entry per dequeued width).
		splits = make(map[uint]rangeSplit)
		// Every width discovered so far, whether still queued or already decided.
		seen = make(map[uint]bool)
		// Widths discovered but not yet processed.
		queue []uint
	)
	//
	add := func(w uint) {
		if w != 0 && !seen[w] {
			seen[w] = true
			queue = append(queue, w)
		}
	}
	// Seed from every (non-constant) register of every module.
	for _, mod := range modules {
		for _, r := range mod.Registers() {
			if !r.IsConst() {
				add(registerWidth(cfg, r))
			}
		}
	}
	// Destructure each width wider than the static limit, pulling in its halves.
	for len(queue) > 0 {
		var n = queue[0]

		queue = queue[1:]
		//
		if n <= MAX_STATIC_RANGE_WIDTH {
			// Leaf width: handled by a static table, no decomposition.
			splits[n] = rangeSplit{}
		} else {
			lo, hi := decompose(n, seen)
			splits[n] = rangeSplit{lo: lo, hi: hi}
			add(lo)
			add(hi)
		}
	}
	//
	return splits
}

// decompose chooses how to destructure a width n > 16 into two halves summing to
// n.  It first tries to reuse an existing pair of required widths (n1, n2) with
// n1 + n2 == n, which avoids introducing fresh range tables; the smallest such
// n1 (hence the largest reusable high half) is chosen for determinism.  Failing
// that, it peels off the highest power of two below n, leaving the (smaller)
// remainder as the low half.  A width counts as reusable if it has already been
// discovered (whether still queued or already decided).
func decompose(n uint, seen map[uint]bool) (lo, hi uint) {
	// Try to reuse an existing pair of widths.
	for lo := uint(1); lo+lo <= n; lo++ {
		if seen[lo] && seen[n-lo] {
			return lo, n - lo
		}
	}
	// Otherwise peel off the highest power of two below n.
	hi = highestPowerOfTwoBelow(n)
	lo = n - hi
	//
	return lo, hi
}

// highestPowerOfTwoBelow returns the largest power of two strictly less than n
// (assuming n > 1).
func highestPowerOfTwoBelow(n uint) uint {
	var p uint = 1
	//
	for p*2 < n {
		p *= 2
	}
	//
	return p
}

// registerWidth returns the bitwidth to range-check a register against.  Native
// registers have no fixed width, so the field bandwidth is used.
func registerWidth(cfg field.Config, r register.Register) uint {
	if r.IsNative() {
		return cfg.BandWidth
	}
	//
	return r.Width()
}

// newStaticRangeTable constructs a static table enumerating every valid value
// 0 .. 2^width - 1.  The table has an index (address) column and a value (data)
// column; the value column is the lookup recipient.
func newStaticRangeTable[W word.Word[W]](name string, width uint) Module {
	var (
		padding big.Int
		// Number of valid values representable in this width.
		rows     = uint64(1) << width
		contents = make([]W, rows)
		regs     = []register.Register{
			// TODO: why an index is needed ??
			register.NewInput(rangeIndexName, width, padding),
			register.NewOutput(rangeValueName, width, padding),
		}
	)
	// Enumerate 0 .. 2^width - 1.
	for i := range rows {
		var w W

		contents[i] = w.SetUint64(i)
	}
	//
	return memory.NewStatic(name, false, regs, contents...)
}

// newRecursiveRangeModule constructs the range module for a width > 16.  It is a
// callable function "fn range_uw(value)" which range-checks its input by
// destructuring it into a low half (lo) and a high half (hi), value = hi::lo,
// and then range-checking each half via an unconditional call into the
// corresponding range module (range_u{lo} / range_u{hi}).  A call whose callee
// is a static enumeration table (width <= 16) is flagged accordingly.
//
// moduleOf maps a width to the index of its range module, so the emitted calls
// can reference their sub-modules by id.
func newRecursiveRangeModule[W word.Word[W]](name string, width uint, s rangeSplit, moduleOf map[uint]uint) Module {
	println("RECURSIVE_RANGE_MODULE:", name, "lo=", s.lo, "hi=", s.hi)

	var (
		padding big.Int
		regs    = []register.Register{
			register.NewInput(rangeValueName, width, padding),
			register.NewComputed(rangeLoName, s.lo, padding),
			register.NewComputed(rangeHiName, s.hi, padding),
		}
		// Register ids follow declaration order: value=0, lo=1, hi=2.
		valID = register.NewId(0)
		loID  = register.NewId(1)
		hiID  = register.NewId(2)
		// Destructure value into its low (lo) and high (hi) halves: value = hi::lo
		// (little-endian targets, so lo receives the low bits).
		destruct = instruction.UintDestruct[W](register.NewVector(loID, hiID), valID)
		// Range-check each half via an unconditional call into its range module.
		loCall = instruction.NewUnconditionalCall(moduleOf[s.lo], loID, s.lo <= MAX_STATIC_RANGE_WIDTH)
		hiCall = instruction.NewUnconditionalCall(moduleOf[s.hi], hiID, s.hi <= MAX_STATIC_RANGE_WIDTH)
		ret    = instruction.NewReturn()
		code   = []VectorInstruction{instruction.NewVector[WordInstruction](destruct, loCall, hiCall, ret)}
	)
	//
	return function.New(name, false, regs, code)
}

func addRangeCalls[W word.Word[W]](cfg field.Config, modules []Module, rangeModules []Module) []Module {
	for _, mod := range modules {
		for _, r := range mod.Registers() {
			if !r.IsConst() {
				// TODO : add a call to the corresponding range module for this register.
				// Issue:
				// - for register > u16 we need a call that populates the range_uX modules. How to do it ?
				// we can't do it before the last return instruction (it might not being hit), it's ugly
				// to do it just before ach return instruction ...
				// - same, we have to deal with multi line instruction ...
				// so in the end, isn't it better to have uncontional call (API already implemented),
				// and when lowering the unconditional call at the --air level it keeps in mind that it has to
				// populate the callee module during a sort of trace expansion, like in zkasm ...
				// does this step still exist ?
				// Another option is to leave it for later ... as with KOALABEAR we'll have at most range_u16 modules,
				// so only static modules, so no issue with this ...
				// WDYT @DAve ?
				return modules // rm me, just to make it compile
			}
		}
	}

	return modules
}
