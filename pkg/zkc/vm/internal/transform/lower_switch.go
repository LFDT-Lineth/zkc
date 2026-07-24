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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform/split"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerSwitch rewrites Switch bytecodes:
// switch x {
// case 1: {do_1}
// case 2: {do_2}
// default: {do_default}}
//
// into:
//
// if x == 1 {b_1 = 1} else {b_1 = 0}
// if x == 2 {b_2 = 1} else {b_2 = 0}
// b_default = 1 - (b_1 + b_2)
//
// dispatch [b_1: {do_1}, b_2: {do_2}] b_default
//
// This enables to have each do_* gated by a single degree 1 guard (b_i != 0),
// independent of the number of cases, even for the default case.
//
// Note: we could transform switch under condition (don't transform for switch on u1, etc),
// but as it may never happen when writing zkc programm, we choose to lower inconditionnaly.
func LowerSwitch[W word.Word[W]](program descriptor.Program[W]) descriptor.Program[W] {
	out := slices.Clone(program.Modules())

	for i, mod := range out {
		if fn, ok := mod.(*descriptor.Function[W]); ok {
			out[i] = lowerSwitchFunction(fn)
		}
	}

	return descriptor.NewProgram(program.Field(), out...)
}

func lowerSwitchFunction[W word.Word[W]](fn *descriptor.Function[W]) *descriptor.Function[W] {
	var (
		vectors = fn.Vectors()
		nvecs   = make([]BytecodeVector[W], len(vectors))
		alloc   = split.NewAllocator(fn)
		// Number of switchCounter lowered so far within this function, used to give
		// each switch's bit registers a distinct, recognisable name.
		switchCounter uint
	)

	for i, vec := range vectors {
		nvecs[i] = lowerSwitchVector(vec, alloc, &switchCounter)
	}

	return descriptor.NewFunction(fn.Name(), alloc.Registers(), fn.Kind(), nvecs)
}

// lowerSwitchVector expands every Switch bytecode within a vector, recalculating
// the skip offsets of all bytecodes (including the other skips in the vector)
// against the new layout.  Vector.Map cannot be used here: a switch case whose
// skip is smaller than the size of its own replacement packet would be
// misclassified as an internal skip and left unremapped.
func lowerSwitchVector[W word.Word[W]](vec BytecodeVector[W], registers split.Allocator[W],
	switchCounter *uint) BytecodeVector[W] {
	var (
		codes = vec.Bytecodes
		// mapping[i] holds the new offset of old bytecode i, with an end
		// sentinel supporting skips which target the end of the vector.
		mapping = make([]uint, len(codes)+1)
		offset  uint
	)
	// First pass: determine the new offset of every bytecode.
	for i, c := range codes {
		mapping[i] = offset
		//
		if sw, ok := c.(*bytecode.Switch[W]); ok {
			offset += switchPacketSize(sw)
		} else {
			offset++
		}
	}
	//
	mapping[len(codes)] = offset
	// Second pass: emit lowered bytecodes with recalculated skips.
	ncodes := make([]Bytecode[W], 0, offset)
	//
	for i, c := range codes {
		switch c := c.(type) {
		case *bytecode.Switch[W]:
			*switchCounter++
			ncodes = append(ncodes, lowerSwitchCode(uint(i), c, mapping, registers, *switchCounter)...)
		case *bytecode.Skip[W]:
			ncodes = append(ncodes, &bytecode.Skip[W]{Skip: relocateSkip(uint(i), c.Skip, mapping)})
		case *bytecode.SkipIf[W]:
			ncodes = append(ncodes, &bytecode.SkipIf[W]{
				Skip: relocateSkip(uint(i), c.Skip, mapping), Left: c.Left, Right: c.Right, Op: c.Op})
		default:
			ncodes = append(ncodes, c)
		}
	}
	//
	return bytecode.NewVector(ncodes...)
}

// switchPacketSize returns the number of bytecodes in the replacement packet
// for a given Switch: five codes per case for its bit diamond (constant load,
// comparison skip, both bit assignments and the internal skip), three codes
// computing the default bit (one-register load, bit sum, subtraction), and the
// final one-hot dispatch.
func switchPacketSize[W word.Word[W]](sw *bytecode.Switch[W]) uint {
	if len(sw.Cases) == 0 {
		return 0
	}
	//
	return 5*uint(len(sw.Cases)) + 4
}

// lowerSwitchCode expands a single Switch bytecode, located at (old) offset pc
// within its enclosing vector, into a one-hot dispatch.  First, every case's
// match is computed unconditionally into a fresh 1-bit register by a diamond,
// and the default bit is derived from the case bits (`c_j`, `b_j`, `one`,
// `sum` and `b_default` are fresh):
//
//	c_0 = v_0
//	skip_if r == c_0 2
//	b_0 = 0
//	skip 1
//	b_0 = 1
//	... (one diamond per case)
//	one = 1
//	sum = b_0 + ... + b_{n-1}
//	b_default = one - sum
//
// after which a single Dispatch bytecode transfers control on the bits:
//
//	dispatch [b_0:<case 0 target>, ..., b_{n-1}:<case n-1 target>] b_default
//
// This satisfies the Dispatch contract (see its declaration): case values are
// pairwise distinct (see Switch.Validate), so at most one case bit is set,
// each bit is constrained on both arms of its diamond, and sum — being a
// 1-bit register — enforces the one-hot invariant via its range constraint,
// with b_default = 1 exactly when no case matched.  In return, each case
// edge's branch condition is a single degree-1 atom (its bit being set),
// independent of the number of cases; only the fall-through (default) edge
// pays a conjunction over all bits, and the join after the dispatch
// simplifies away entirely.
//
// Each case needs a fresh constant register since every load executes on all
// paths.  The register is sized to the dispatch register, which the case's
// value cannot overflow (see Switch.Validate).
//
// TODO: compare against constants directly, rather than loading them into
// registers first (cf https://github.com/LFDT-Lineth/zkc/issues/1879).
func lowerSwitchCode[W word.Word[W]](pc uint, sw *bytecode.Switch[W], mapping []uint,
	registers split.Allocator[W], switchIndex uint) []Bytecode[W] {
	//
	var (
		n     = uint(len(sw.Cases))
		codes = make([]Bytecode[W], 0, switchPacketSize(sw))
		bits  = make([]descriptor.RegisterId, n)
		width = registers.Register(sw.Source).Bitwidth()
		zero  = word.Const64[W](0)
		one   = word.Const64[W](1)
	)
	//
	if n == 0 {
		return nil
	}
	// Materialise every case's match into a fresh bit register, unconditionally.
	for j, cse := range sw.Cases {
		creg := registers.Allocate("", width)
		bits[j] = registers.AllocateNamed(fmt.Sprintf("$b_switch_%d_case_%d", switchIndex, j+1), util.Some[uint](1))
		//
		codes = append(codes,
			// c_j = v_j
			bytecode.LoadConst(creg, cse.Value),
			// skip_if r == c_j 2  => match, jump to "b_j = 1"
			bytecode.NewSkipIf[W](bytecode.CONDITION_EQ, 2, sw.Source, creg),
			// b_j = 0  (no match)
			bytecode.LoadConst(bits[j], zero),
			// skip 1  => jump over "b_j = 1"
			bytecode.NewSkip[W](1),
			// b_j = 1  (match)
			bytecode.LoadConst(bits[j], one))
	}
	// Derive the default bit: b_default = 1 - (b_0 + ... + b_{n-1}), which is 1
	// exactly when no case matched.  Enforcing b_default to be a u1
	// enforces that at most 1 case_* is 1.
	//
	// TODO: we could compute b_no_default = sum instead to save one subtraction
	// We don't do it now for simplicity.
	var (
		onereg = registers.Allocate("", util.Some[uint](1))
		sumreg = registers.Allocate("", util.Some[uint](1))
		bdef   = registers.AllocateNamed(fmt.Sprintf("$b_switch_%d_case_default", switchIndex), util.Some[uint](1))
	)
	//
	codes = append(codes,
		// one = 1
		bytecode.LoadConst(onereg, one),
		// sum = b_0 + ... + b_{n-1}
		bytecode.AddConst(sumreg, bits, zero),
		// b_default = one - sum
		bytecode.NewArith(bytecode.OP_SUB, []descriptor.RegisterId{bdef},
			[]descriptor.RegisterId{onereg, sumreg}, zero),
		// Explicit range proof for b_default:
		bytecode.NewCheckCast[W](bdef, 1))
	// Dispatch on the bits, in case order.
	var (
		// New position of the dispatch bytecode itself.
		position = mapping[pc] + 5*n + 5
		dcases   = make([]bytecode.DispatchCase, n)
	)
	//
	for j, cse := range sw.Cases {
		// New position of this case's dispatch target.
		target := mapping[pc+uint(cse.Skip)+1]
		//
		dcases[j] = bytecode.DispatchCase{Bit: bits[j], Skip: util.Cast[uint16](target - position - 1)}
	}
	//
	return append(codes, bytecode.NewDispatch[W](dcases, bdef))
}

// relocateSkip recalculates the skip offset of a (conditional or unconditional)
// skip located at old offset pc, such that it continues to identify the same
// target under the new layout described by mapping.
func relocateSkip(pc uint, skip uint16, mapping []uint) uint16 {
	var (
		// New position of the skip's target.
		target = mapping[pc+uint(skip)+1]
		// New position of the skip itself.
		position = mapping[pc]
	)
	//
	return util.Cast[uint16](target - position - 1)
}
