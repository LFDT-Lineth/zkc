// Copyright Consensys Incorporated
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
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// rangeModulePrefix and rangeValueName mirror the naming used by the VM-side
// range-module generator (pkg/zkc/vm/internal/transform).  That package is
// internal to pkg/zkc/vm and so cannot be imported here, hence the conventions
// are duplicated.
const (
	rangeModulePrefix = "$range_u"
	rangeValueName    = "value"
)

// rangeTable locates the static range-check table for a given register width:
// the id of the module enumerating every value 0 .. 2^n-1, together with the id
// of its "value" column (the lookup recipient).
type rangeTable struct {
	module schema.ModuleId
	value  register.Id
}

// indexRangeTables indexes, by register width, the static range-check tables
// present in the machine.  A static $range_un table fully enumerates the values
// of an n-bit register and is generated for exactly the widths
// n <= maxStaticWidth; wider registers are range-checked by a recursive range
// module (see addRangeProofConstraints), so only the static tables are
// collected here.
func indexRangeTables[W vm.Word[W], F field.Element[F]](program vm.Program[W], maxStaticWidth uint,
) map[uint]rangeTable {
	//
	var tables = make(map[uint]rangeTable)
	//
	for id, m := range program.Modules() {
		// Only the fully-enumerated static tables serve as direct lookup targets;
		mem, ok := m.(*vm.Memory[W])
		if !ok || !mem.IsStatic() || !strings.HasPrefix(m.Name(), rangeModulePrefix) {
			continue
		}
		// The "value" column holds the enumerated values and is the lookup target.
		valId := m.HasRegister(rangeValueName)
		if valId.IsEmpty() {
			continue
		}
		// The value column is a word (non-native) register, so it has a bitwidth.
		if w := m.Register(valId.Unwrap()).Bitwidth().Unwrap(); w <= maxStaticWidth {
			tables[w] = rangeTable{schema.ModuleId(id), register.NewId(uint(valId.Unwrap()))}
		}
	}
	//
	return tables
}

// addRangeProofConstraints emits, for every register of the given module whose
// width is covered by a static range table, an unfiltered lookup asserting the
// register's value lies in that table — i.e. it fits within its declared bit
// width.  Both sides are unfiltered: the source ranges over every row (each row
// holds a value which must be in range) and the target over the whole enumeration.
//
// Registers wider than maxStaticWidth have no static table, and are
// range-checked by the recursive range module of their width instead.  For a
// function, AddRangeChecks has already inserted a call into that module, which
// addLookups lowers into a lookup.  A memory has no bytecode to host such a
// call, so its wide registers are looked up here (see addRecursiveRangeLookup).
// Native (field-element) registers are not range-checked at all.
//
// Zero-width registers (e.g. the shared zero register used to pad lookups
// against wider memories / callees) are constrained to be exactly zero.  This
// is essential for soundness: such registers appear as ordinary columns in
// call and memory lookups, where they stand in for limbs that must be zero.
// Without this constraint, a prover could assign them arbitrary values and
// thereby bypass the implied range check on the narrower side of the lookup.
func (p *constraintTranslator[W, F]) addRangeProofConstraints(mod *schema.Table[F], ctx schema.ModuleId,
	regs ...vm.Register[W]) {
	// A schema module is translated from the program module of the same
	// identifier, which tells us whether this is a memory.
	_, isMemory := p.program.Module(util.Cast[uint16](ctx)).(*vm.Memory[W])
	// TODO: lots of perf possible here, see
	// https://github.com/LFDT-Lineth/zkc/issues/1907
	for i, reg := range mod.Registers() {
		if reg.IsNative() {
			// Native registers are obviously not range-checked.
			continue
		} else if i < len(regs) && !regs[i].IsRangeChecked() {
			// We have len(regs) <= len(mod.Registers()), where the diff is corset-added registers
			// ($ret, IS_PC, etc). We may skip only the first ones, but will always range-checked the later.
			continue
		}
		// Registers of width <= MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK are not
		// range-checked with a lookup to a static table, but with a vanishing
		// constraint (see smallRangeCheck).
		if reg.Width() <= vm.MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK {
			regId := register.NewId(uint(i))
			handle := fmt.Sprintf("range_u%d_%d", reg.Width(), regId.Unwrap())
			r := mirc.Variable[F](regId, reg.Width(), 0)
			mod.AddConstraints(vanishing.NewConstraint(handle, ctx, util.None[int](),
				smallRangeCheck(r, reg.Width()).AsLogical()))

			continue
		}
		// Registers wider than maxStaticWidth have no static table.
		if reg.Width() > p.maxStaticWidth {
			// Those of a function are range-checked recursively via a call
			// lookup that has been inserted in the vm previously.  Those of a
			// memory have no such call, so need a dedicated lookup.
			if isMemory {
				p.addRecursiveRangeLookup(mod, ctx, register.NewId(uint(i)), reg.Width())
			}
			//
			continue
		}
		// a width <= maxStaticWidth must always have a static table.
		table, ok := p.rangeTables[reg.Width()]
		util.Assert(ok, "missing static range table for width %d", reg.Width())
		//
		var (
			regId  = register.NewId(uint(i))
			handle = fmt.Sprintf("range_u%d_%d", reg.Width(), regId.Unwrap())
			// Source: the register's value on the current row.
			source = lookup.UnfilteredVector(ctx, regId)
			// Target: the enumerated value column of the matching $range_un table.
			target = lookup.UnfilteredVector(table.module, table.value)
		)
		//
		mod.AddConstraints(lookup.NewConstraint[F](handle,
			[]mir.LookupVector{target}, []mir.LookupVector{source}))
	}
}

// addRecursiveRangeLookup emits a lookup asserting that a memory register too
// wide for a static range table fits within its declared bit width.  The target
// is the "value" column of the recursive range module of that width, filtered
// on its $ret line (which is 1 exactly on the rows where the module's own
// constraints destructure and range-check the value).  This is the lookup which
// a call "$range_uN(r)" is lowered into (see emitCallLookup).
//
// The source is unfiltered, so the padding rows of the memory are checked as
// well.  Every column of a memory is zero on its padding rows, so this only
// requires that zero is present in the range module.  The range module is
// still called directly for function registers and casts; since a memory has
// no bytecode to host such a call, the values held by its registers (along
// with zero) are instead added when the trace of the memory is built.
func (p *constraintTranslator[W, F]) addRecursiveRangeLookup(mod *schema.Table[F], ctx schema.ModuleId,
	regId register.Id, width uint) {
	//
	fn, ok := p.rangeFunctions[width]
	// Sanity check a recursive range module was generated for this width.
	util.Assert(ok, "missing recursive range module for width %d", width)
	//
	var (
		callee = p.program.Module(fn.Module).(*vm.Function[W])
		handle = fmt.Sprintf("range_u%d_%d", width, regId.Unwrap())
		// $ret is allocated immediately after the callee's registers (see
		// translateFunction).
		retId = register.NewId(uint(len(callee.Registers())))
		// Source: the register's value on the current row.
		source = lookup.UnfilteredVector(ctx, regId)
		// Target: the values checked by the matching $range_un function.
		target = lookup.FilteredVector(uint(fn.Module), retId, register.NewId(uint(fn.Value)))
	)
	//
	mod.AddConstraints(lookup.NewConstraint[F](handle,
		[]mir.LookupVector{target}, []mir.LookupVector{source}))
}

// emitCastCheck lowers a cast "check r:uN" into a range check of r against N
// bits, which applies only on the rows where the cast executes (i.e. those
// where the given source selector is on).  Observe that N is strictly below the
// declared width of r, so the (unconditional) range proof of r does not imply
// this.  Three cases arise:
//
//   - N <= MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK: the vanishing constraint
//     "sel != 0 ==> poly_N(r) == 0" (see smallRangeCheck).
//   - N <= maxStaticWidth: a lookup of r (filtered by sel) into the static
//     $range_uN table.
//   - N > maxStaticWidth: nothing to do here, as AddRangeChecks has already
//     inserted a call "$range_uN(r)" after the cast (on the same path), which is
//     lowered into a call lookup gated likewise.
//
// In all cases, r is read on the cast's row, where its column holds the value
// cast.  This is because r is never written at or after the cast on that row:
// where it would be (e.g. by the call in "x = f(x as u8)"), FlattenLookupAccess
// has already replaced it with a snapshot.  This applies equally to the call
// "$range_uN(r)" following a wide cast, which reads the same column.
func (p *constraintTranslator[W, F]) emitCastCheck(mod *schema.Table[F], ctx schema.ModuleId, pc, cc uint,
	vec vm.BytecodeVector[W], cast *vm.BytecodeCheckCast[W], srcSelector register.Id) {
	var (
		width  = uint(cast.Bitwidth)
		regId  = register.NewId(uint(cast.Target))
		handle = fmt.Sprintf("cast_u%d_%d_%d_%d", width, ctx, pc, cc)
	)
	// Sanity check the column of r holds the value cast (see above).
	util.Assert(!isWrittenFrom(vec, cc, cast.Target),
		"cast target %d written at or after the cast on the same row", cast.Target)
	//
	switch {
	case width > p.maxStaticWidth:
		// Range-checked via the call to $range_uN following the cast.
		return
	case width <= vm.MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK:
		var (
			sel = mirc.Variable[F](srcSelector, 1, 0)
			r   = mirc.Variable[F](regId, mod.Registers()[regId.Unwrap()].Width(), 0)
		)
		//
		mod.AddConstraints(vanishing.NewConstraint(handle, ctx, util.None[int](),
			mirc.If(sel.NotEquals(mirc.Number[F](0)), smallRangeCheck(r, width)).AsLogical()))
	default:
		table, ok := p.rangeTables[width]
		// Sanity check a static table was generated for this width.
		util.Assert(ok, "missing static range table for cast to u%d", width)
		//
		var (
			// Source: the cast's target on the rows executing the cast.
			source = lookup.FilteredVector(ctx, srcSelector, regId)
			// Target: the enumerated value column of the matching $range_un table.
			target = lookup.UnfilteredVector(table.module, table.value)
		)
		//
		mod.AddConstraints(lookup.NewConstraint[F](handle,
			[]mir.LookupVector{target}, []mir.LookupVector{source}))
	}
}

// isWrittenFrom determines whether the given register may be written by the
// bytecode at offset "from" in the given vector, or by any bytecode on some path
// through it.
func isWrittenFrom[W vm.Word[W]](vec vm.BytecodeVector[W], from uint, reg vm.RegisterId) bool {
	var reaches = vec.ReachMap()
	//
	for j := from; j < uint(len(vec.Bytecodes)); j++ {
		var reachable = j == from || reaches.StateOf(j).ReachableFrom(from)
		//
		if reachable && slices.Contains(vec.Bytecodes[j].Definitions(), reg) {
			return true
		}
	}
	//
	return false
}

// smallRangeCheck constructs the vanishing constraint "r * (r - 1) * ... * (r -
// (2^width - 1)) == 0" asserting r fits within width bits, for a width of at most
// MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK (wider registers being range-checked by
// lookups instead).  For width 0 this is simply r == 0.
func smallRangeCheck[F field.Element[F]](r Expr[F], width uint) Expr[F] {
	util.Assert(width <= vm.MAX_BITWIDTH_POLYNOMIAL_RANGE_CHECK, "invalid width for small range check")
	//
	var factors = []Expr[F]{r}
	//
	for i := uint(1); i < 1<<width; i++ {
		factors = append(factors, r.Subtract(mirc.Number[F](i)))
	}
	//
	return mirc.Product(factors...).Equals(mirc.Number[F](0))
}
