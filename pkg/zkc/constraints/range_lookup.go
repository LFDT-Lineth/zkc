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

	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
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
// n <= maxStaticWidth; wider registers are range-checked recursively by
// a call (lowered via addCallLookups), so only the static tables are collected
// here.
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

// addRangeProofConstraints emits, for every register of the given function whose
// width is covered by a static range table, an unfiltered lookup asserting the
// register's value lies in that table — i.e. it fits within its declared bit
// width.  Both sides are unfiltered: the source ranges over every row (each row
// holds a value which must be in range) and the target over the whole enumeration.
//
// Registers wider than maxStaticWidth have no static table; they are
// range-checked at runtime by a recursive call which addCallLookups lowers into
// a lookup.  Native (field-element) registers are not range-checked at all.
//
// Zero-width registers (e.g. the shared zero register used to pad lookups
// against wider memories / callees) are constrained to be exactly zero.  This
// is essential for soundness: such registers appear as ordinary columns in
// call and memory lookups, where they stand in for limbs that must be zero.
// Without this constraint, a prover could assign them arbitrary values and
// thereby bypass the implied range check on the narrower side of the lookup.
func (p *constraintTranslator[W, F]) addRangeProofConstraints(mod *schema.Table[F], ctx schema.ModuleId,
	regs ...vm.Register[W]) {
	// TODO: lots of perf possible here, see
	// https://github.com/LFDT-Lineth/zkc/issues/1907
	// https://github.com/LFDT-Lineth/zkc/issues/1911
	for i, reg := range mod.Registers() {
		if reg.IsNative() {
			// Native registers are not range-checked.
			continue
		} else if i < len(regs) && !regs[i].IsRangeChecked() {
			// The condition "if i < len(regs)" ensures that ALL registers (like PC, etc)
			// added are range-checked. Note that len(mod.Registers()) >= len(regs)
			continue
		}
		// Registers of width <= 2 are not range-checked with a lookup to a
		// static table, but with a vanishing constraint (see smallRangeCheck).
		if reg.Width() <= 2 {
			regId := register.NewId(uint(i))
			handle := fmt.Sprintf("range_u%d_%d", reg.Width(), regId.Unwrap())
			r := mirc.Variable[F](regId, reg.Width(), 0)
			mod.AddConstraints(vanishing.NewConstraint(handle, ctx, util.None[int](),
				smallRangeCheck(r, reg.Width()).AsLogical()))

			continue
		}
		//
		table, ok := p.rangeTables[reg.Width()]
		if !ok {
			// a width <= maxStaticWidth must always have a static table.
			if reg.Width() <= p.maxStaticWidth {
				panic(fmt.Sprintf("missing static range table for width %d", reg.Width()))
			}
			// Wider registers are range-checked recursively via a call lookup.
			continue
		}
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

// emitCastCheck lowers a cast "check r:uN" into a range check of r against N
// bits, which applies only on the rows where the cast executes (i.e. those
// where the given source selector is on).  Observe that N is strictly below the
// declared width of r, so the (unconditional) range proof of r does not imply
// this.  Three cases arise:
//
//   - N <= 2: the vanishing constraint "sel != 0 ==> poly_N(r) == 0" (see
//     smallRangeCheck).
//   - N <= maxStaticWidth: a lookup of r (filtered by sel) into the static
//     $range_uN table.
//   - N > maxStaticWidth: nothing to do here, as AddRangeChecks has already
//     inserted a call "$range_uN(r)" after the cast (on the same path), which is
//     lowered into a call lookup gated likewise.
//
// The value checked is that held by the target at the cast.  Usually, the
// target's column holds this value on the cast's row.  However, when the target
// may be written later on the same row (e.g. "x = f(x as u8)", where the call
// returns into x), that column holds the later value instead.  In such case,
// the value checked is read from the previous row (see castShift) and, for a
// lookup (which cannot read across rows), materialised in a fresh column bound
// to it.  This snapshot is taken here, rather than in the bytecode, since a
// cast also narrows the bitwidth of its target for subsequent bytecodes (e.g.
// a call passing it to a narrower argument), which a snapshot would lose.
func (p *constraintTranslator[W, F]) emitCastCheck(mod *schema.Table[F], ctx schema.ModuleId, pc, cc uint,
	vec vm.BytecodeVector[W], writes dfa.Result[dfa.Writes], cast *vm.BytecodeCheckCast[W],
	srcSelector register.Id) {
	var (
		width  = uint(cast.Bitwidth)
		regId  = register.NewId(uint(cast.Target))
		handle = fmt.Sprintf("cast_u%d_%d_%d_%d", width, ctx, pc, cc)
	)
	// Range-checked via the call to $range_uN following the cast.
	if width > p.maxStaticWidth {
		return
	}
	//
	var (
		regWidth = mod.Registers()[regId.Unwrap()].Width()
		shift    = castShift(vec, writes, cc, cast.Target)
	)
	//
	if width <= 2 {
		var (
			sel = mirc.Variable[F](srcSelector, 1, 0)
			r   = mirc.Variable[F](regId, regWidth, shift)
		)
		//
		mod.AddConstraints(vanishing.NewConstraint(handle, ctx, util.None[int](),
			mirc.If(sel.NotEquals(mirc.Number[F](0)), smallRangeCheck(r, width)).AsLogical()))
		//
		return
	}
	//
	table, ok := p.rangeTables[width]
	// Sanity check a static table was generated for this width.
	util.Assert(ok, "missing static range table for cast to u%d", width)
	// Snapshot the value checked into a fresh column, if necessary.
	if shift != 0 {
		regId = newCastSnapshot(mod, ctx, regId, regWidth, shift, srcSelector)
	}
	//
	var (
		// Source: the value checked on the rows executing the cast.
		source = lookup.FilteredVector(ctx, srcSelector, regId)
		// Target: the enumerated value column of the matching $range_un table.
		target = lookup.UnfilteredVector(table.module, table.value)
	)
	//
	mod.AddConstraints(lookup.NewConstraint[F](handle,
		[]mir.LookupVector{target}, []mir.LookupVector{source}))
}

// castShift determines the row, relative to that of the cast at offset cc,
// whose target column holds the value checked by the cast.  This is the cast's
// own row (0), unless the target may be written at or after the cast on that
// row.  In such case, the target cannot have been written before the cast on
// this row (as vectorisation starts a new row on reassignment) and, hence, holds
// the value checked on the previous row (-1).
func castShift[W vm.Word[W]](vec vm.BytecodeVector[W], writes dfa.Result[dfa.Writes], cc uint,
	target vm.RegisterId) int {
	//
	var reaches = vec.ReachMap()
	//
	for j := cc; j < uint(len(vec.Bytecodes)); j++ {
		var reachable = j == cc || reaches.StateOf(j).ReachableFrom(cc)
		//
		if reachable && slices.Contains(vec.Bytecodes[j].Definitions(), target) {
			// Sanity check target not also written before the cast.
			util.Assert(!writes.StateOf(cc).MaybeAssigned(target),
				"cast target %d written both before and after cast on same row", target)
			//
			return -1
		}
	}
	//
	return 0
}

// newCastSnapshot allocates a fresh column holding the value of the given
// register on the given (relative) row, and returns its id.  The column is
// filled during trace expansion and, on the rows where the given selector is
// on, constrained to equal that value.  The binding is gated since it cannot
// hold on padding rows (e.g. on the first padding row, the register's previous
// value is that of the last non-padding row, whilst the column holds padding).
func newCastSnapshot[F field.Element[F]](mod *schema.Table[F], ctx schema.ModuleId, regId register.Id,
	width uint, shift int, selector register.Id) register.Id {
	var (
		// Allocate the snapshot column.
		snapId = register.NewId(mod.Width())
		// Default padding
		padding F
		sel     = mirc.Variable[F](selector, 1, 0)
	)
	//
	mod.AddRegisters(register.NewComputed[F](fmt.Sprintf("$cast_%d", snapId.Unwrap()), width, padding))
	// Fill it during trace expansion with the register's (shifted) value.
	mod.AddAssignments(assignment.NewComputedRegister(snapId,
		term.NewRegisterAccess[F, mir.Term[F]](regId, width, shift), ctx))
	// Bind it for soundness: sel != 0 ==> $cast == register on the given row.
	mod.AddConstraints(vanishing.NewConstraint(fmt.Sprintf("cast_%d", snapId.Unwrap()), ctx, util.None[int](),
		mirc.If(sel.NotEquals(mirc.Number[F](0)),
			mirc.Variable[F](snapId, width, 0).Equals(mirc.Variable[F](regId, width, shift))).AsLogical()))
	//
	return snapId
}

// smallRangeCheck constructs the vanishing constraint "r * (r - 1) * ... * (r -
// (2^width - 1)) == 0" asserting r fits within width bits, for a width of at most
// 2 (wider registers being range-checked by lookups instead).  For width 0 this
// is simply r == 0.
func smallRangeCheck[F field.Element[F]](r Expr[F], width uint) Expr[F] {
	util.Assert(width <= 2, "invalid width for small range check")
	//
	var factors = []Expr[F]{r}
	//
	for i := uint(1); i < 1<<width; i++ {
		factors = append(factors, r.Subtract(mirc.Number[F](i)))
	}
	//
	return mirc.Product(factors...).Equals(mirc.Number[F](0))
}
