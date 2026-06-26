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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

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
// Native (field-element) registers are not ranged-checked.
//
// This pass must run after SplitRegisters.
func AddRangeConstraints[W word.Word[W]](cfg field.Config, m *machine.Word[W]) *machine.Word[W] {
	var (
		modules = m.Modules()
	)

	// First step, generate the range modules for every width which occurs on some register.
	var extra = generateRangeModules[W](cfg, modules)

	// Second step, call range_uX for every registers.
	modules = addRangeCalls[W](modules, extra)

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
		var name = rangeModuleName(w)
		// Note, this could be optimized:
		// https://github.com/LFDT-Lineth/zkc/issues/1910
		// and https://github.com/LFDT-Lineth/zkc/issues/1911
		if w <= util.MAX_STATIC_RANGE_WIDTH {
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
// into, while leaf widths (<= 16) map to the zero split.
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
	// Seed from every register of every module.
	for _, mod := range modules {
		for _, r := range mod.Registers() {
			add(registerWidthOrZero(r))
		}
	}
	// Destructure each width wider than the static limit, pulling in its halves.
	for len(queue) > 0 {
		var n = queue[0]

		queue = queue[1:]
		//
		if n <= util.MAX_STATIC_RANGE_WIDTH {
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

// registerWidthOrZero returns the bit-width to range-check a register against, or 0 if
// it needs no check (for native field)
func registerWidthOrZero(r register.Register) uint {
	if r.IsNative() {
		return 0
	}
	//
	return r.Width()
}

// newStaticRangeTable constructs a static table enumerating every valid value
// 0 .. 2^width - 1.  The table has an index (address) column and a value (data)
// column; the value column is the lookup recipient.
// TODO: some mini perf, see https://github.com/LFDT-Lineth/zkc/issues/1907
func newStaticRangeTable[W word.Word[W]](name string, width uint) Module {
	var (
		padding big.Int
		// Number of valid values representable in this width.
		rows     = 1 << width
		contents = make([]W, rows)
		regs     = []register.Register{
			// TODO: why an index is needed see https://github.com/LFDT-Lineth/zkc/issues/1906
			register.NewInput(rangeIndexName, width, padding),
			register.NewOutput(rangeValueName, width, padding),
		}
	)
	// Enumerate 0 .. 2^width - 1.
	for i := range contents {
		var w W

		contents[i] = w.SetUint64(uint64(i))
	}
	//
	return memory.NewStatic(name, false, memory.NewGeometry[W](regs), contents...)
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
		// (little-endian targets, so lo receives the low bits).  The subsequent
		// checks read lo/hi via in-vector forwarding, so they see the destructured
		// values.
		codes = []WordInstruction{instruction.UintDestruct[W](register.NewVector(loID, hiID), valID)}
	)
	// Range-check each half: a static table (<= 16) is read via MemRead, a
	// recursive range function (> 16) is invoked via Call.
	codes = appendRangeCheck(codes, loID, s.lo, moduleOf)
	codes = appendRangeCheck(codes, hiID, s.hi, moduleOf)
	codes = append(codes, instruction.NewReturn())
	//
	return function.New(name, false, regs, []VectorInstruction{instruction.NewVector(codes...)})
}

// appendRangeCheck appends to codes a range-check of register r (of width w)
// against range_u{w} (see rangeCheck).
func appendRangeCheck(codes []WordInstruction, r register.Id, w uint, moduleOf map[uint]uint) []WordInstruction {
	return append(codes, rangeCheck(moduleOf[w], r, w))
}

// rangeModuleName returns the canonical module name for the range module of a
// given width.
func rangeModuleName(w uint) string {
	return fmt.Sprintf("$range_u%d", w)
}

// rangeCheck builds a single range-check of register r (of width w) against the
// range module whose id is `id`: a (data-less) MemRead from the static ROM when
// w <= 16, or a Call into the recursive range function otherwise.
func rangeCheck(id uint, r register.Id, w uint) WordInstruction {
	if w <= util.MAX_STATIC_RANGE_WIDTH {
		return instruction.NewMemRead(id, []register.Id{r}, nil)
	}

	return instruction.NewUnconditionalCall(id, []register.Id{r}, nil)
}

// addRangeCalls range-checks every register of every function
// module: a block of range-checks is inserted before each row-terminating
// instruction (Return or Jump — a Fail row is rejected so needs no check), so
// that every row of every register column is checked.  Non-function modules
// carry no instructions to host the checks and are left unchanged.
func addRangeCalls[W word.Word[W]](modules []Module, rangeModules []Module) []Module {
	// Resolve range_u{w}'s module id by name.  Range modules are appended after
	// the original modules (in ascending-width order), so the k-th range module
	// has id len(modules)+k.  Only range-module names are ever looked up.
	var idOf = make(map[string]uint, len(rangeModules))
	//
	for k, m := range rangeModules {
		idOf[m.Name()] = uint(len(modules)) + uint(k)
	}
	//
	var out = make([]Module, len(modules))
	//
	for i, mod := range modules {
		out[i] = addRangeChecks(mod, idOf)
	}
	//
	return out
}

// addRangeChecks inserts a range-check of each register before
// every Return / Jump terminator in each vector of the given function.  Vector
// .Map remaps skip offsets, so a skip which targeted the terminator instead
// lands on the first check and falls through to it.
func addRangeChecks(mod Module, idOf map[string]uint) Module {
	fn, ok := mod.(*WordFunction)
	if !ok || fn.IsNative() {
		return mod
	}
	// Build the check block: one range-check per register.
	var checks []WordInstruction
	//
	for j, r := range fn.Registers() {
		// For runtime, we only need to do the call for registers wider than MAX_STATIC_RANGE_WIDTH
		// to populate the range module.
		// For registers of width <= MAX_STATIC_RANGE_WIDTH, the range check is done via a static table,
		// and the lookup is added when generating constraints.
		if w := registerWidthOrZero(r); w > util.MAX_STATIC_RANGE_WIDTH {
			checks = append(checks, rangeCheck(idOf[rangeModuleName(w)], register.NewId(uint(j)), w))
		}
	}
	//
	if len(checks) == 0 {
		return mod
	}
	// Insert the check block before every Return / Jump in each vector.
	var (
		code  = fn.Code()
		ncode = make([]VectorInstruction, len(code))
	)
	//
	for i, vec := range code {
		ncode[i] = vec.Map(func(_ uint, ith WordInstruction) []WordInstruction {
			switch ith.(type) {
			case *instruction.Return, *instruction.Jump:
				return append(slices.Clone(checks), ith)
			default:
				return []WordInstruction{ith}
			}
		})
	}
	//
	return function.New(fn.Name(), fn.IsNative(), fn.Registers(), ncode)
}
