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
	"fmt"
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	tracer "github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// ramLayout records the register ids and limb widths of every column of a
// read-write memory (RAM) module, in the fixed order established by
// translateReadWriteMemory: one row per memory access, in access order, with
// per-row constraints tying the timestamps together.  Cross-row consistency
// (permuted block, bus, finalizer) is added by follow-up PRs of #2160.  Both
// the module translation (which creates the columns) and the trace observer /
// caller->RAM lookup (which reference them by position) derive the layout from
// this helper so all sides stay in lock-step.
//
// Register order is [inputs, outputs, computed]:
//   - inputs   : []ADDRESS                 (declared address lines)
//   - outputs  : []VALUE_WRITTEN           (declared data lines)
//   - computed : EXEC, IS_WRITE, []VALUE_READ, []TIMESTAMP_WRITTEN,
//     []TIMESTAMP_READ, []TIMESTAMP_DELTA, []TS_CARRY, EXEC_WRITE, EXEC_READ,
//     []TEMPORAL_TS, []TEMPORAL_TS_CARRY
//
// All limb slices are most-significant-limb first (matching declaration /
// "big endian" order used by ApplyLimbsMap and the module register order).
type ramLayout struct {
	// address is the cell accessed by this row.
	address []register.Id
	// valueWritten is the value the cell holds immediately AFTER this row's
	// access: the value written (for a write) or, for a read, the value read
	// back (a read leaves the cell unchanged, so it "writes back" what it
	// found).  This is the column the caller's lookup pins for both kinds of
	// access.
	valueWritten []register.Id
	// exec is 1 on every real row (one per access, in access order), 0 on padding.
	exec register.Id
	// isWrite distinguishes a write access (1) from a read access (0).
	isWrite register.Id
	// valueRead is the value the cell held immediately BEFORE this row's
	// access, i.e. the value the last access to this address wrote.  For a
	// read, VALUE_READ == VALUE_WRITTEN (enforced here); that VALUE_READ is
	// genuinely the last write's value is the receive/send consistency
	// argument deferred with the bus.
	valueRead []register.Id
	// tsWritten is this access's timestamp: the caller's threaded stamp
	// (stamps count from one; timestamp zero is reserved for the initial state
	// of an untouched cell).
	tsWritten []register.Id
	// tsRead is the timestamp of the LAST access to this address (zero for a
	// first touch): the "when" of valueRead.
	tsRead []register.Id
	// tsDelta witnesses TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA
	// (range-checked >= 0), i.e. TIMESTAMP_READ < TIMESTAMP_WRITTEN.
	tsDelta []register.Id
	// tsCarry witnesses the per-boundary carries of that multi-limb addition:
	// one fewer entry than the timestamp has limbs, indexed by significance.
	tsCarry []register.Id
	// execWrite / execRead select the write (EXEC * IS_WRITE) and read
	// (EXEC * (1 - IS_WRITE)) rows of the execution phase.  They exist because
	// a lookup's target filter must be a single column: a caller's write-site
	// lookup targets the table filtered by execWrite, a read-site lookup by
	// execRead, which is how each access's read/write kind is pinned.
	execWrite register.Id
	execRead  register.Id
	// temporalTs is the shard's clock: TEMPORAL_TS = prev(TEMPORAL_TS) + 1 on
	// consecutive real rows.  A permutation against tsWritten is added by #2206.
	temporalTs []register.Id
	// temporalTsCarry witnesses the carries of that increment.
	temporalTsCarry []register.Id
	// Limb widths (most-significant first) of the data and timestamp register
	// families.
	dataWidths []uint
	tsWidths   []uint
}

// translateReadWriteMemory builds the MIR module for a read-write (RAM) memory:
// the declared address / data columns plus the synthetic columns, together with
// the per-row constraints.
func (p *constraintTranslator[W, F]) translateReadWriteMemory(ctx schema.ModuleId, m *vm.Memory[W]) mir.Module[F] {
	//
	var (
		mod    *schema.Table[F, mir.Constraint[F]]
		regs   = toRegisters(m.Registers())
		layout = computeRamLayout(m, p.program.Field())
	)
	// Initialise module.  Note a leading padding row exists (EXEC == 0 there),
	// emitted by the tracer (see traceReadWriteMemory).  A read-write memory is
	// internal state: neither a public input nor a public output, never native.
	mod = mod.Init(m.Name(), false, false, false, false, false)
	mod.AddRegisters(regs...)
	// Append the synthetic columns, in the order fixed by computeRamLayout.
	mod.AddRegisters(
		register.NewComputed(tracer.RAM_EXEC_NAME, 1),
		register.NewComputed(tracer.RAM_IS_WRITE_NAME, 1),
	)
	addLimbRegisters(mod, tracer.RAM_VALUE_READ_PREFIX, layout.dataWidths)
	addLimbRegisters(mod, tracer.RAM_TS_WRITTEN_PREFIX, layout.tsWidths)
	addLimbRegisters(mod, tracer.RAM_TS_READ_PREFIX, layout.tsWidths)
	addLimbRegisters(mod, tracer.RAM_TS_DELTA_PREFIX, layout.tsWidths)
	addCarryRegisters(mod, tracer.RAM_TS_CARRY_PREFIX, len(layout.tsCarry))
	mod.AddRegisters(
		register.NewComputed(tracer.RAM_EXEC_WRITE_NAME, 1),
		register.NewComputed(tracer.RAM_EXEC_READ_NAME, 1),
	)
	addLimbRegisters(mod, tracer.RAM_TEMPORAL_TS_PREFIX, layout.tsWidths)
	addCarryRegisters(mod, tracer.RAM_TEMPORAL_TS_CARRY_PREFIX, len(layout.temporalTsCarry))
	// Per-row constraints.
	mod.AddConstraints(ramGeneralConstraints[F](ctx, layout)...)
	mod.AddConstraints(ramExecConstraints[F](ctx, layout)...)
	mod.AddConstraints(ramChronologyConstraints[F](ctx, layout)...)
	// Range-prove every column.  This covers the internally-witnessed columns
	// (value-read, timestamp-read, deltas, carries) which — unlike the address /
	// value / timestamp-written columns pinned by the caller lookup — are not
	// otherwise constrained.  1-bit columns (phase bits, carries) get an r*r==r
	// constraint; wider columns a range-table lookup.
	p.addRangeProofConstraints(mod, ctx, mod.Registers())
	//
	return mod
}

// computeRamLayout determines the full column layout of a RAM module for the
// given field, without creating any registers.  Register ids are assigned in
// the fixed order documented on ramLayout.
func computeRamLayout[W vm.Word[W]](m *vm.Memory[W], field field.Config) ramLayout {
	var (
		addrRegs = m.AddressRegisters()
		dataRegs = m.DataRegisters()
		nAddr    = uint(len(addrRegs))
		nData    = uint(len(dataRegs))
		// Timestamp limb widths, most-significant first.  A stamp is a register
		// of the memory's declared timestamp width (see "memory name[uN](...)"),
		// carried by the descriptor, so it splits exactly like the caller's
		// threaded stamp.
		tsWidths = array.Reverse(register.LimbWidths(field.RegisterWidth, m.TimestampWidth().Unwrap()))
		nStamp   = uint(len(tsWidths))
		next     = nAddr + nData
	)
	//
	layout := ramLayout{
		dataWidths: widthsOf(dataRegs),
		tsWidths:   tsWidths,
	}
	// inputs / outputs occupy the leading positions
	layout.address = idRange(0, nAddr)
	layout.valueWritten = idRange(nAddr, nData)
	// computed columns follow, in declaration order
	layout.exec = register.NewId(next)
	layout.isWrite = register.NewId(next + 1)
	next += 2
	//
	layout.valueRead = idRange(next, nData)
	next += nData
	layout.tsWritten = idRange(next, nStamp)
	next += nStamp
	layout.tsRead = idRange(next, nStamp)
	next += nStamp
	layout.tsDelta = idRange(next, nStamp)
	next += nStamp
	layout.tsCarry = idRange(next, nStamp-1)
	next += nStamp - 1
	layout.execWrite = register.NewId(next)
	layout.execRead = register.NewId(next + 1)
	next += 2
	// Appended last so the ids above (used by the caller lookup) are unchanged.
	layout.temporalTs = idRange(next, nStamp)
	next += nStamp
	layout.temporalTsCarry = idRange(next, nStamp-1)
	//
	return layout
}

// idRange returns the contiguous register ids [start, start+n).
func idRange(start, n uint) []register.Id {
	ids := make([]register.Id, n)
	//
	for i := range ids {
		ids[i] = register.NewId(start + uint(i))
	}
	//
	return ids
}

// widthsOf extracts the bit-widths of the given registers, preserving order.  A
// native (field-element) lane — e.g. a felt-valued RAM's data line — has no fixed
// width and is reported as math.MaxUint (the native sentinel), so the value
// columns built from it become native columns too.  Address / timestamp lanes are
// never native.
func widthsOf[W vm.Word[W]](regs []vm.Register[W]) []uint {
	widths := make([]uint, len(regs))
	//
	for i, r := range regs {
		if r.IsNative() {
			widths[i] = math.MaxUint
		} else {
			widths[i] = r.Bitwidth().Unwrap()
		}
	}
	//
	return widths
}

// addLimbRegisters appends one computed register per given limb width, named
// "<prefix><k>".
func addLimbRegisters[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	prefix string, widths []uint) {
	//
	for k, w := range widths {
		mod.AddRegisters(register.NewComputed(tracer.RamLimbName(prefix, uint(k)), w))
	}
}

// addCarryRegisters appends n single-bit computed carry registers named
// "<prefix><k>".  A carry out of a two-operand limb addition is always in {0,1},
// so one bit suffices.
func addCarryRegisters[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	prefix string, n int) {
	//
	for k := 0; k < n; k++ {
		mod.AddRegisters(register.NewComputed(tracer.RamLimbName(prefix, uint(k)), 1))
	}
}

// ramGeneralConstraints builds the row-shape constraints: binarity of EXEC and
// IS_WRITE, a leading padding row, EXEC nondecreasing (so the layout is
// [padding..][EXEC..]), and the definitions of the lookup selectors.
func ramGeneralConstraints[F field.Element[F]](ctx schema.ModuleId, l ramLayout) []mir.Constraint[F] {
	var (
		zero      = mirc.Number[register.Id, Expr[F]](0)
		one       = mirc.Number[register.Id, Expr[F]](1)
		exec      = mirc.Variable[register.Id, Expr[F]](l.exec, 1, 0)
		isWrite   = mirc.Variable[register.Id, Expr[F]](l.isWrite, 1, 0)
		prevExec  = mirc.Variable[register.Id, Expr[F]](l.exec, 1, -1)
		execWrite = mirc.Variable[register.Id, Expr[F]](l.execWrite, 1, 0)
		execRead  = mirc.Variable[register.Id, Expr[F]](l.execRead, 1, 0)
	)
	//
	return []mir.Constraint[F]{
		// binary columns
		binaryConstraint[F]("exec_is_binary", ctx, exec),
		binaryConstraint[F]("is_write_is_binary", ctx, isWrite),
		// leading padding row: EXEC[0] == 0.
		mir.NewVanishingConstraint("exec_vanishes_in_padding", ctx, util.Some(0),
			exec.Equals(zero).AsLogical()),
		// EXEC nondecreasing: EXEC[i-1] == 1 => EXEC[i] == 1.
		mir.NewVanishingConstraint("exec_monotony", ctx, util.None[int](),
			mirc.If(prevExec.Equals(one), exec.Equals(one)).AsLogical()),
		// The per-kind lookup selectors are fully determined:
		// EXEC_WRITE == EXEC * IS_WRITE, and EXEC_READ == EXEC * (1 - IS_WRITE)
		// expressed subtraction-free as EXEC_WRITE + EXEC_READ == EXEC.
		mir.NewVanishingConstraint("exec_write_def", ctx, util.None[int](),
			execWrite.Equals(exec.Multiply(isWrite)).AsLogical()),
		mir.NewVanishingConstraint("exec_read_def", ctx, util.None[int](),
			execWrite.Add(execRead).Equals(exec).AsLogical()),
	}
}

// ramExecConstraints builds the execution-phase constraints (guarded by EXEC):
// the timestamp ordering TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA
// (which entails TIMESTAMP_READ < TIMESTAMP_WRITTEN), and — for reads — the
// equality []VALUE_READ == []VALUE_WRITTEN.
func ramExecConstraints[F field.Element[F]](ctx schema.ModuleId, l ramLayout) []mir.Constraint[F] {
	var (
		zero    = mirc.Number[register.Id, Expr[F]](0)
		exec    = mirc.Variable[register.Id, Expr[F]](l.exec, 1, 0)
		isWrite = mirc.Variable[register.Id, Expr[F]](l.isWrite, 1, 0)
		execOn  = exec.NotEquals(zero)
		cs      []mir.Constraint[F]
	)
	// TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA
	cs = append(cs, multiLimbIncrement[F](ctx, "ts", l.tsWritten, l.tsRead, l.tsDelta,
		l.tsCarry, l.tsWidths, 0, execOn)...)
	// read (IS_WRITE == 0) => []VALUE_READ == []VALUE_WRITTEN
	readOn := execOn.And(isWrite.Equals(zero))
	//
	for k := range l.valueRead {
		var (
			vr = mirc.Variable[register.Id, Expr[F]](l.valueRead[k], l.dataWidths[k], 0)
			vw = mirc.Variable[register.Id, Expr[F]](l.valueWritten[k], l.dataWidths[k], 0)
		)

		cs = append(cs, mir.NewVanishingConstraint(fmt.Sprintf("read_value_%d", k), ctx, util.None[int](),
			mirc.If(readOn, vr.Equals(vw)).AsLogical()))
	}
	//
	return cs
}

// ramChronologyConstraints builds the clock constraint: on consecutive EXEC
// rows, TEMPORAL_TS = prev(TEMPORAL_TS) + 1.  A shard's first EXEC row is
// unconstrained.
func ramChronologyConstraints[F field.Element[F]](ctx schema.ModuleId, l ramLayout) []mir.Constraint[F] {
	var (
		one      = mirc.Number[register.Id, Expr[F]](1)
		exec     = mirc.Variable[register.Id, Expr[F]](l.exec, 1, 0)
		prevExec = mirc.Variable[register.Id, Expr[F]](l.exec, 1, -1)
		bothExec = prevExec.Equals(one).And(exec.Equals(one))
	)
	//
	return multiLimbIncrement[F](ctx, "temporal_ts_increment", l.temporalTs, l.temporalTs, nil,
		l.temporalTsCarry, l.tsWidths, -1, bothExec)
}

// multiLimbIncrement emits the constraints proving the multi-limb relation
//
//	out = base + 1 + delta
//
// over limb slices given most-significant-limb first.  `base` limbs are read at
// row offset `baseShift` (0 for the same row, -1 for the previous row); `out`,
// `delta` and `carry` on the current row.  A nil `delta` means a plain
// increment (delta = 0).  `carry` (length len(out)-1, indexed by significance)
// witnesses the carry out of each limb; the most significant limb must produce
// no carry.  Every constraint is guarded by `guard`.
func multiLimbIncrement[F field.Element[F]](ctx schema.ModuleId, prefix string,
	out, base, delta, carry []register.Id, widths []uint, baseShift int, guard Expr[F],
) []mir.Constraint[F] {
	var (
		one = mirc.Number[register.Id, Expr[F]](1)
		L   = len(out)
		cs  = make([]mir.Constraint[F], 0, L)
	)
	// Iterate by significance s (0 == least significant).  In the MSB-first
	// arrays, significance s lives at index i = L-1-s.
	for s := 0; s < L; s++ {
		var (
			i      = L - 1 - s
			w      = widths[i]
			outVar = mirc.Variable[register.Id, Expr[F]](out[i], w, 0)
			// left-hand side: base (+ delta) (+ carry-in) (+ 1 at the least
			// significant limb).
			lhs = mirc.Variable[register.Id, Expr[F]](base[i], w, baseShift)
			// right-hand side accumulates the output limb and the outgoing carry.
			rhs = outVar
		)
		//
		if delta != nil {
			lhs = lhs.Add(mirc.Variable[register.Id, Expr[F]](delta[i], w, 0))
		}
		// carry into this limb (from the less significant boundary)
		if s > 0 {
			lhs = lhs.Add(mirc.Variable[register.Id, Expr[F]](carry[s-1], 1, 0))
		}
		// the +1 lands on the least significant limb
		if s == 0 {
			lhs = lhs.Add(one)
		}
		// carry out of this limb (none for the most significant limb)
		if s < L-1 {
			shift := new(big.Int).Lsh(big.NewInt(1), w)
			rhs = rhs.Add(mirc.Variable[register.Id, Expr[F]](carry[s], 1, 0).
				Multiply(mirc.BigNumber[register.Id, Expr[F]](shift)))
		}
		//
		cs = append(cs, mir.NewVanishingConstraint(fmt.Sprintf("%s_add_limb_%d", prefix, s), ctx, util.None[int](),
			mirc.If(guard, lhs.Equals(rhs)).AsLogical()))
	}
	//
	return cs
}

// binaryConstraint builds "e * e == e", asserting e is 0 or 1.
func binaryConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, e Expr[F]) mir.Constraint[F] {
	return mir.NewVanishingConstraint(handle, ctx, util.None[int](),
		e.Multiply(e).Equals(e).AsLogical())
}
