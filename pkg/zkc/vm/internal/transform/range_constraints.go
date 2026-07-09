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
	"math/bits"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Register names used within generated range modules.  The "value" column is
// the lookup recipient in both static tables and recursive modules, so callers
// wiring the (deferred) lookups can rely on a single, uniform target name.
const (
	rangeValueName = "value"
	rangeLoName    = "lo"
	rangeHiName    = "hi"
)

// AddRangeConstraints adds, for every distinct register width occurring in the
// program, a "range" module which acts as the recipient of a range-proof lookup
// for registers of that width.  Two flavours of range module are generated:
//
//   - For a width n <= maxStaticWidth, range_un is a static table enumerating every valid
//     value 0 .. 2^n - 1 in a single value column.
//
//   - For a width n > maxStaticWidth, range_un destructures the value into two halves
//     (lo of n/2 bits, hi of n-n/2 bits) via the constraint value = hi::lo,
//     and (later) range-checks each half by lookups into range_u{lo} and
//     range_u{hi}.  This recursion bottoms out at the static tables above.
//
// Native (field-element) registers are not ranged-checked.
//
// NOTE: this transform must run after register splitting.
func AddRangeConstraints[W word.Word[W]](cfg field.Config, program descriptor.Program[W],
	maxStaticDepth uint) descriptor.Program[W] {
	var (
		modules = program.Modules()
		// maxStaticWidth is the largest X for which 2^X <= maxStaticDepth (the
		// max static table size), i.e. floor(log2(maxStaticDepth)).
		// It represents the maximum register width for which a static table can be use to range-check it.
		// Wider registers require recursive range modules.
		maxStaticWidth = uint(bits.Len(maxStaticDepth) - 1)
	)

	// First step, generate the range modules for every width which occurs on some register.
	var extra = generateRangeModules(modules, maxStaticWidth)

	// Second step, call range_uX for every registers.
	modules = addRangeCalls(modules, extra, maxStaticWidth)

	// Reassemble the program with the original modules plus the range modules.
	return descriptor.NewProgram(program.Field(), append(slices.Clone(modules), extra...)...)
}

func generateRangeModules[W word.Word[W]](modules []descriptor.Module[W],
	maxStaticWidth uint) []descriptor.Module[W] {
	var (
		// Every width requiring a range module, mapped to its decomposition.
		splits = neededRangeWidths(modules, maxStaticWidth)
		// Freshly generated range modules.
		extra []descriptor.Module[W]
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
		if w <= maxStaticWidth {
			extra = append(extra, newStaticRangeTable[W](name, w))
		} else {
			extra = append(extra, newRecursiveRangeModule[W](name, w, splits[w], moduleOf, maxStaticWidth))
		}
	}

	return extra
}

// rangeSplit records how a width > maxStaticWidth is destructured into a low half and a high
// half, where lo + hi == width.  For leaf widths (<= maxStaticWidth) both fields are zero.
type rangeSplit struct {
	lo, hi uint
}

// neededRangeWidths computes the decomposition of every register width for which
// a range module must be generated.  This is the set of widths occurring
// directly on some register, closed under the destructuring of wide widths
// (> maxStaticWidth); each wide width is mapped to the (lo, hi) halves it is destructured
// into, while leaf widths (<= maxStaticWidth) map to the zero split.
func neededRangeWidths[W word.Word[W]](modules []descriptor.Module[W],
	maxStaticWidth uint) map[uint]rangeSplit {
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
		// Note: width == 0 comes from native registers, which are not range-checked.
		// width == 1 are not range proven with a call to a static table, but with a constraint r (1 - r) == 0
		if w != 0 && w != 1 && !seen[w] {
			seen[w] = true
			queue = append(queue, w)
		}
	}

	for _, mod := range modules {
		// Seed from the PC register, which is added later while lowering to mir
		// (see translateFunction). It exists only for non-atomic, non-native
		// functions, and the PC bit width must match the one chosen there.
		if fn, ok := mod.(*descriptor.Function[W]); ok && !fn.IsNative() && !fn.IsOneLine() {
			add(fn.PcWidth())
		}
		// Seed from every register of every module.
		for _, r := range mod.Registers() {
			add(registerWidthOrZero(r))
		}
	}
	// Destructure each width wider than the static limit, pulling in its halves.
	for len(queue) > 0 {
		var n = queue[0]

		queue = queue[1:]
		//
		if n <= maxStaticWidth {
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
func registerWidthOrZero[W word.Word[W]](r descriptor.Register[W]) uint {
	if r.IsNative() {
		return 0
	}
	//
	return r.Bitwidth().Unwrap()
}

// newStaticRangeTable constructs a static table enumerating every valid value
// 0 .. 2^width - 1.  The table has an index (address) column and a value (data)
// column; the value column is the lookup recipient.
// TODO: some mini perf, see https://github.com/LFDT-Lineth/zkc/issues/1907
func newStaticRangeTable[W word.Word[W]](name string, width uint) descriptor.Module[W] {
	var (
		padding W
		// Number of valid values representable in this width.
		rows     = 1 << width
		contents = make([]W, rows)
		regs     = []descriptor.Register[W]{
			descriptor.NewRegister(register.OUTPUT_REGISTER, rangeValueName, util.Some(width), padding),
		}
	)
	// Enumerate 0 .. 2^width - 1.
	for i := range contents {
		var w W

		contents[i] = w.SetUint64(uint64(i))
	}
	//
	return descriptor.NewMemory(name, regs, memory.PRIVATE_STATIC_MEMORY, contents)
}

// newRecursiveRangeModule constructs the range module for a width > maxStaticWidth.  It is a
// callable function "fn range_uw(value)" which range-checks its input by
// destructuring it into a low half (lo) and a high half (hi), value = hi::lo,
// and then range-checking each half via an unconditional call into the
// corresponding range module (range_u{lo} / range_u{hi}).  A call whose callee
// is a static enumeration table (width <= maxStaticWidth) is flagged accordingly.
//
// moduleOf maps a width to the index of its range module, so the emitted calls
// can reference their sub-modules by id.
func newRecursiveRangeModule[W word.Word[W]](name string, width uint, s rangeSplit,
	moduleOf map[uint]uint, maxStaticWidth uint) descriptor.Module[W] {
	//
	var (
		padding W
		regs    = []descriptor.Register[W]{
			descriptor.NewRegister(register.INPUT_REGISTER, rangeValueName, util.Some(width), padding),
			descriptor.NewRegister(register.COMPUTED_REGISTER, rangeLoName, util.Some(s.lo), padding),
			descriptor.NewRegister(register.COMPUTED_REGISTER, rangeHiName, util.Some(s.hi), padding),
		}
		// Register ids follow declaration order: value=0, lo=1, hi=2.
		valID = bytecode.RegisterId(0)
		loID  = bytecode.RegisterId(1)
		hiID  = bytecode.RegisterId(2)
		// Destructure value into its low (lo) and high (hi) halves: value = hi::lo
		// (little-endian targets, so lo receives the low bits).  The subsequent
		// checks read lo/hi via in-vector forwarding, so they see the destructured
		// values.
		codes = []Bytecode[W]{bytecode.AddVec[W]([]bytecode.RegisterId{loID, hiID}, []bytecode.RegisterId{valID})}
	)
	// Range-check each half: a static table (<= 16) is read via MemRead, a
	// recursive range function (> 16) is invoked via Call.
	codes = appendRangeCheck(codes, loID, s.lo, moduleOf, maxStaticWidth)
	codes = appendRangeCheck(codes, hiID, s.hi, moduleOf, maxStaticWidth)
	codes = append(codes, bytecode.NewRet())
	//
	return descriptor.NewFunction(name, regs, false, []BytecodeVector[W]{bytecode.NewVector(codes...)})
}

// appendRangeCheck appends to codes a range-check of register r (of width w)
// against range_u{w} (see rangeCheck).
func appendRangeCheck[W word.Word[W]](codes []Bytecode[W], r bytecode.RegisterId, w uint,
	moduleOf map[uint]uint, maxStaticWidth uint) []Bytecode[W] {
	return append(codes, rangeCheck[W](moduleOf[w], r, w, maxStaticWidth))
}

// rangeModuleName returns the canonical module name for the range module of a
// given width.
func rangeModuleName(w uint) string {
	return fmt.Sprintf("$range_u%d", w)
}

// rangeCheck builds a single range-check of register r (of width w) against the
// range module whose id is `id`: a (data-less) MemRead from the static ROM when
// w <= 16, or a Call into the recursive range function otherwise.
func rangeCheck[W word.Word[W]](id uint, r bytecode.RegisterId, w uint,
	maxStaticWidth uint) Bytecode[W] {
	if w <= maxStaticWidth {
		return bytecode.NewMemRead(uint16(id), []bytecode.RegisterId{r}, nil)
	}
	//
	return bytecode.CallFun(uint16(id), bytecode.CallFlags{Unconditional: true}, []bytecode.RegisterId{r}, nil)
}

// addRangeCalls range-checks every register of every function module: a block of
// range-checks is inserted before each row-terminating bytecode (Ret or Jmp — a
// Fail row is rejected so needs no check), so that every row of every register
// column is checked.  Non-function modules carry no bytecodes to host the checks
// and are left unchanged.
func addRangeCalls[W word.Word[W]](modules []descriptor.Module[W],
	rangeModules []descriptor.Module[W],
	maxStaticWidth uint) []descriptor.Module[W] {
	// Resolve range_u{w}'s module id by name.  Range modules are appended after
	// the original modules (in ascending-width order), so the k-th range module
	// has id len(modules)+k.  Only range-module names are ever looked up.
	var idOf = make(map[string]uint, len(rangeModules))
	//
	for k, m := range rangeModules {
		idOf[m.Name()] = uint(len(modules)) + uint(k)
	}
	//
	var out = make([]descriptor.Module[W], len(modules))
	//
	for i, mod := range modules {
		out[i] = addRangeChecks(mod, idOf, maxStaticWidth)
	}
	//
	return out
}

// addRangeChecks inserts a range-check of each register before every Ret / Jmp
// terminator in each vector of the given function.  Vector.Map remaps skip
// offsets, so a skip which targeted the terminator instead lands on the first
// check and falls through to it.
func addRangeChecks[W word.Word[W]](mod descriptor.Module[W], idOf map[string]uint,
	maxStaticWidth uint) descriptor.Module[W] {
	fn, ok := mod.(*descriptor.Function[W])
	if !ok || fn.IsNative() {
		return mod
	}
	// Build the check block: one range-check per register.
	var checks []Bytecode[W]
	//
	for j, r := range fn.Registers() {
		// For runtime, we only need to do the call for registers wider than maxStaticWidth
		// to populate the range module.
		// For registers of width <= maxStaticWidth, the range check is done via a static table,
		// and the lookup is added when generating constraints.
		if w := registerWidthOrZero(r); w > maxStaticWidth {
			checks = append(checks, rangeCheck[W](idOf[rangeModuleName(w)], bytecode.RegisterId(j), w,
				maxStaticWidth))
		}
	}
	//
	if len(checks) == 0 {
		return mod
	}
	// Insert the check block before every Ret / Jmp in each vector.
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
	)
	//
	for i, vec := range vectors {
		nvecs[i] = vec.Map(func(_ uint, ith Bytecode[W]) []Bytecode[W] {
			switch ith.(type) {
			case *bytecode.Ret, *bytecode.Jmp:
				return append(slices.Clone(checks), ith)
			default:
				return []Bytecode[W]{ith}
			}
		})
	}
	//
	return descriptor.NewFunction(fn.Name(), fn.Registers(), fn.IsNative(), nvecs)
}
