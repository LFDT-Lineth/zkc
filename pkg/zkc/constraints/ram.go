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
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/mirc"
	tracer "github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/trace"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// stampWidth is the bit-width of a read-write memory timestamp.  It MUST match
// the value used by the ThreadTimestamps transform
// (pkg/zkc/vm/internal/transform/thread_timestamps.go), so that the caller's
// (split) stamp limbs line up with this module's timestamp columns.
//
// TODO: source this per-memory once a stamp-width syntax exists at the ZkC
// source level (issue #2069) rather than sharing a global default across
// packages.
const stampWidth uint = 32

// ramLayout records the register ids and limb widths of every column of a
// read-write memory (RAM) module, in the fixed order established by
// translateReadWriteMemory.  This is the specification of how the RAM table
// works: one row per memory access (plus, in a follow-up, one row per touched
// cell for initialisation / finalisation), with per-row constraints tying the
// timestamps together and — once the offline memory-checking bus lands —
// receive/send pairs proving every read returns the last value written.  Both
// the module translation (which creates the columns) and the trace observer /
// caller->RAM lookup (which reference them by position) derive the layout from
// this helper so all sides stay in lock-step.
//
// Register order is [inputs, outputs, computed]:
//   - inputs   : []ADDRESS                 (declared address lines)
//   - outputs  : []VALUE_WRITTEN           (declared data lines)
//   - computed : EXEC, FINL, IS_WRITE, []VALUE_READ, []TIMESTAMP_WRITTEN,
//     []TIMESTAMP_READ, []TIMESTAMP_DELTA, []ADDRESS_DELTA,
//     []TS_CARRY, []ADDR_CARRY
//
// All limb slices are most-significant-limb first (matching declaration /
// "big endian" order used by ApplyLimbsMap and the module register order).
type ramLayout struct {
	// address is the cell accessed by this row: on an EXEC row, the address of
	// the guest program's access; on a (future) FINL row, the cell being
	// initialised / finalised.
	address []register.Id
	// valueWritten is the value the cell holds immediately AFTER this row's
	// access: the value written (for a write) or, for a read, the value read
	// back (a read leaves the cell unchanged, so it "writes back" what it
	// found).  This is the column the caller's lookup pins for both kinds of
	// access.
	valueWritten []register.Id
	// exec is a binary phase flag: 1 on the rows mirroring the guest program's
	// accesses (one row per access, in access order).
	exec register.Id
	// finl is a binary phase flag: 1 on the initialisation / finalisation rows
	// (one per touched cell) which a follow-up PR will emit; mutually
	// exclusive with exec.
	finl register.Id
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
	// tsDelta witnesses the strict timestamp orderings of both phases.  On EXEC
	// rows: TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA, with
	// TIMESTAMP_DELTA range-checked >= 0, so TIMESTAMP_READ <
	// TIMESTAMP_WRITTEN.  On FINL rows the direction REVERSES (written = the
	// cell's initial state, read = its final state): TIMESTAMP_READ =
	// TIMESTAMP_WRITTEN + 1 + TIMESTAMP_DELTA, so TIMESTAMP_WRITTEN <
	// TIMESTAMP_READ, pinning each finalized cell to one genuinely touched
	// during execution.  Sharing the column is sound since EXEC * FINL == 0.
	tsDelta []register.Id
	// addrDelta witnesses the strict ADDRESS ordering of the (future) FINL
	// phase: each touched cell must be initialised / finalised AT MOST once,
	// which the table proves by listing FINL rows in strictly increasing
	// address order — ADDRESS = prev(ADDRESS) + 1 + ADDRESS_DELTA, with
	// ADDRESS_DELTA range-checked >= 0.  Strict monotony alone delivers
	// uniqueness: the first FINL address is deliberately unconstrained (an
	// anchor at zero would force an init/finalize event for address 0 even
	// when that cell was never touched).  Unused (zero) on EXEC rows.
	addrDelta []register.Id
	// tsCarry / addrCarry witness the per-boundary carry of the multi-limb
	// timestamp / address additions above.  Each has one fewer entry than the
	// value it carries (the most significant limb produces no carry), indexed
	// by significance (index 0 == carry out of the least significant limb).
	tsCarry   []register.Id
	addrCarry []register.Id
	// Limb widths (most-significant first) of the address, data and timestamp
	// register families.
	addrWidths []uint
	dataWidths []uint
	tsWidths   []uint
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
		// Timestamp limb widths, most-significant first.  A stamp is a stampWidth
		// register, so it splits exactly like the caller's threaded stamp.
		tsWidths = array.Reverse(register.LimbWidths(field.RegisterWidth, stampWidth))
		nStamp   = uint(len(tsWidths))
		next     = nAddr + nData
	)
	//
	layout := ramLayout{
		addrWidths: widthsOf(addrRegs),
		dataWidths: widthsOf(dataRegs),
		tsWidths:   tsWidths,
	}
	// inputs / outputs occupy the leading positions
	layout.address = idRange(0, nAddr)
	layout.valueWritten = idRange(nAddr, nData)
	// computed columns follow, in declaration order
	layout.exec = register.NewId(next)
	layout.finl = register.NewId(next + 1)
	layout.isWrite = register.NewId(next + 2)
	next += 3
	//
	layout.valueRead = idRange(next, nData)
	next += nData
	layout.tsWritten = idRange(next, nStamp)
	next += nStamp
	layout.tsRead = idRange(next, nStamp)
	next += nStamp
	layout.tsDelta = idRange(next, nStamp)
	next += nStamp
	layout.addrDelta = idRange(next, nAddr)
	next += nAddr
	layout.tsCarry = idRange(next, nStamp-1)
	next += nStamp - 1
	layout.addrCarry = idRange(next, nAddr-1)
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

// translateReadWriteMemory builds the MIR module for a read-write (RAM) memory:
// the declared address / data columns plus the synthetic phase, value-read,
// timestamp and delta columns, together with the local (per-row) consistency
// constraints of the RAM spec.  The offline memory-checking bus (rcv/snd) and
// the finalization rows are deferred to a follow-up PR; the finalization-phase
// constraints below are therefore written but vacuous (no FINL rows are emitted
// yet).
func translateReadWriteMemory[W vm.Word[W], F field.Element[F]](
	ctx schema.ModuleId, m *vm.Memory[W], field field.Config,
	rangeTables map[uint]rangeTable, maxStaticWidth uint) mir.Module[F] {
	//
	var (
		mod     *schema.Table[F, mir.Constraint[F]]
		name    = trace.ModuleName{Name: m.Name(), Multiplier: 1}
		regs    = toRegisters(m.Registers())
		layout  = computeRamLayout(m, field)
		padding big.Int
	)
	// Initialise module.  AllowPadding is true so a leading padding row exists
	// (EXEC == FINL == 0 there).  A read-write memory is internal state: it is
	// neither a public input nor a public output, and never native.
	mod = mod.Init(name, true, false, false, false, false, false, 0)
	mod.AddRegisters(regs...)
	// Append the synthetic columns, in the order fixed by computeRamLayout.
	mod.AddRegisters(
		register.NewComputed(tracer.RAM_EXEC_NAME, 1, padding),
		register.NewComputed(tracer.RAM_FINL_NAME, 1, padding),
		register.NewComputed(tracer.RAM_IS_WRITE_NAME, 1, padding),
	)
	addLimbRegisters(mod, tracer.RAM_VALUE_READ_PREFIX, layout.dataWidths, padding)
	addLimbRegisters(mod, tracer.RAM_TS_WRITTEN_PREFIX, layout.tsWidths, padding)
	addLimbRegisters(mod, tracer.RAM_TS_READ_PREFIX, layout.tsWidths, padding)
	addLimbRegisters(mod, tracer.RAM_TS_DELTA_PREFIX, layout.tsWidths, padding)
	addLimbRegisters(mod, tracer.RAM_ADDR_DELTA_PREFIX, layout.addrWidths, padding)
	addCarryRegisters(mod, tracer.RAM_TS_CARRY_PREFIX, len(layout.tsCarry), padding)
	addCarryRegisters(mod, tracer.RAM_ADDR_CARRY_PREFIX, len(layout.addrCarry), padding)
	// Local (per-row) consistency constraints.
	mod.AddConstraints(ramGeneralConstraints[F](ctx, layout)...)
	mod.AddConstraints(ramExecConstraints[F](ctx, layout)...)
	mod.AddConstraints(ramFinlConstraints[F](ctx, layout)...)
	// Range-prove every column.  This covers the internally-witnessed columns
	// (value-read, timestamp-read, deltas, carries) which — unlike the address /
	// value / timestamp-written columns pinned by the caller lookup — are not
	// otherwise constrained.  1-bit columns (phase bits, carries) get an r*r==r
	// constraint; wider columns a range-table lookup.
	addRangeProofConstraints(mod, ctx, mod.Registers(), rangeTables, maxStaticWidth)
	//
	return mod
}

// addLimbRegisters appends one computed register per given limb width, named
// "<prefix><k>".
func addLimbRegisters[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	prefix string, widths []uint, padding big.Int) {
	//
	for k, w := range widths {
		mod.AddRegisters(register.NewComputed(tracer.RamLimbName(prefix, uint(k)), w, padding))
	}
}

// addCarryRegisters appends n single-bit computed carry registers named
// "<prefix><k>".  A carry out of a two-operand limb addition is always in {0,1},
// so one bit suffices.
func addCarryRegisters[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]],
	prefix string, n int, padding big.Int) {
	//
	for k := 0; k < n; k++ {
		mod.AddRegisters(register.NewComputed(tracer.RamLimbName(prefix, uint(k)), 1, padding))
	}
}

// ramGeneralConstraints builds the phase-bit constraints shared by both phases:
// binarity of EXEC, FINL, IS_WRITE and (EXEC+FINL); a leading padding row; and
// the "activity" monotonicities (FINL and EXEC+FINL nondecreasing).  Together
// these force the row layout [padding..][EXEC..][FINL..].
func ramGeneralConstraints[F field.Element[F]](ctx schema.ModuleId, l ramLayout) []mir.Constraint[F] {
	var (
		zero      = mirc.Number[register.Id, Expr[F]](0)
		one       = mirc.Number[register.Id, Expr[F]](1)
		exec      = mirc.Variable[register.Id, Expr[F]](l.exec, 1, 0)
		finl      = mirc.Variable[register.Id, Expr[F]](l.finl, 1, 0)
		isWrite   = mirc.Variable[register.Id, Expr[F]](l.isWrite, 1, 0)
		active    = exec.Add(finl)
		prevFinl  = mirc.Variable[register.Id, Expr[F]](l.finl, 1, -1)
		prevExec  = mirc.Variable[register.Id, Expr[F]](l.exec, 1, -1)
		prevActiv = prevExec.Add(prevFinl)
	)
	//
	return []mir.Constraint[F]{
		// binary columns
		binaryConstraint[F]("exec_is_binary", ctx, exec),
		binaryConstraint[F]("finl_is_binary", ctx, finl),
		binaryConstraint[F]("is_write_is_binary", ctx, isWrite),
		// EXEC and FINL (each already binary) are mutually exclusive: EXEC*FINL == 0
		// (equivalently EXEC+FINL is binary).
		mir.NewVanishingConstraint("exec_and_finl_are_binary_exclusive", ctx, util.None[int](),
			exec.Multiply(finl).Equals(zero).AsLogical()),
		// leading padding row: EXEC[0] == FINL[0] == 0.
		mir.NewVanishingConstraint("exec_vanishes_in_padding", ctx, util.Some(0),
			active.Equals(zero).AsLogical()),
		// FINL nondecreasing: FINL[i-1] == 1 => FINL[i] == 1.
		mir.NewVanishingConstraint("finl_monotony", ctx, util.None[int](),
			mirc.If(prevFinl.Equals(one), finl.Equals(one)).AsLogical()),
		// (EXEC+FINL) nondecreasing: active[i-1] == 1 => active[i] == 1.
		mir.NewVanishingConstraint("active_monotony", ctx, util.None[int](),
			mirc.If(prevActiv.Equals(one), active.Equals(one)).AsLogical()),
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

// ramFinlConstraints builds the finalization-phase constraints (guarded by
// FINL): addresses strictly increase from one FINL row to the next (via
// []ADDRESS_DELTA) — which alone guarantees at most one init/finalize event per
// cell, so the first FINL address is deliberately unconstrained; the strict
// timestamp ordering TIMESTAMP_READ > TIMESTAMP_WRITTEN pins each finalized cell
// to one genuinely touched during execution (an untouched cell's final timestamp
// is zero); and every FINL row carries the cell's initial state on the written
// side ([]VALUE_WRITTEN == 0, []TIMESTAMP_WRITTEN == 0) — the "init" half of the
// init/finalize pair (the finalize half reads the final state into []VALUE_READ /
// TIMESTAMP_READ, pinned by the deferred rcv/snd bus).  These are vacuous until
// finalization rows are emitted (a follow-up PR) but are written now so
// []ADDRESS_DELTA has a defined role and the columns are not dangling.
func ramFinlConstraints[F field.Element[F]](ctx schema.ModuleId, l ramLayout) []mir.Constraint[F] {
	var (
		zero      = mirc.Number[register.Id, Expr[F]](0)
		one       = mirc.Number[register.Id, Expr[F]](1)
		finl      = mirc.Variable[register.Id, Expr[F]](l.finl, 1, 0)
		prevFinl  = mirc.Variable[register.Id, Expr[F]](l.finl, 1, -1)
		laterFinl = finl.Equals(one).And(prevFinl.Equals(one))
		cs        []mir.Constraint[F]
	)
	// later FINL rows: []ADDRESS = prev([]ADDRESS) + 1 + []ADDRESS_DELTA (strictly
	// increasing).  The base operand is read on the previous row (shift -1).
	cs = append(cs, multiLimbIncrement[F](ctx, "finl_addr", l.address, l.address, l.addrDelta,
		l.addrCarry, l.addrWidths, -1, laterFinl)...)
	// every FINL row initialises its cell: the init half sends (value 0, time 0),
	// so the written side vanishes — []VALUE_WRITTEN == 0 and []TIMESTAMP_WRITTEN == 0.
	finlOn := finl.Equals(one)
	// FINL rows: []TIMESTAMP_READ = []TIMESTAMP_WRITTEN + 1 + []TIMESTAMP_DELTA,
	// i.e. TIMESTAMP_READ > TIMESTAMP_WRITTEN.  NOTE the direction is deliberately
	// the OPPOSITE of the EXEC-phase ordering: on a FINL row the WRITTEN side is
	// the cell's INITIAL state and the READ side its FINAL state, so the read
	// timestamp is the later one.  Strictness pins a FINL row to a cell genuinely
	// touched during execution (stamps count from one; timestamp zero is the
	// untouched-cell initial state), which is what allows the first FINL address
	// to be unconstrained.  TIMESTAMP_DELTA / TS_CARRY are reused as witnesses:
	// EXEC * FINL == 0, so they are idle on FINL rows.
	cs = append(cs, multiLimbIncrement[F](ctx, "finl_ts", l.tsRead, l.tsWritten, l.tsDelta,
		l.tsCarry, l.tsWidths, 0, finlOn)...)
	//
	for k := range l.valueWritten {
		vw := mirc.Variable[register.Id, Expr[F]](l.valueWritten[k], l.dataWidths[k], 0)
		cs = append(cs, mir.NewVanishingConstraint(fmt.Sprintf("finl_value_written_zero_%d", k), ctx, util.None[int](),
			mirc.If(finlOn, vw.Equals(zero)).AsLogical()))
	}
	//
	for k := range l.tsWritten {
		tw := mirc.Variable[register.Id, Expr[F]](l.tsWritten[k], l.tsWidths[k], 0)
		cs = append(cs, mir.NewVanishingConstraint(fmt.Sprintf("finl_ts_written_zero_%d", k), ctx, util.None[int](),
			mirc.If(finlOn, tw.Equals(zero)).AsLogical()))
	}
	//
	return cs
}

// multiLimbIncrement emits the constraints proving the multi-limb relation
//
//	out = base + 1 + delta
//
// over limb slices given most-significant-limb first.  `base` limbs are read at
// row offset `baseShift` (0 for the same row, -1 for the previous row); `out`,
// `delta` and `carry` on the current row.  `carry` (length len(out)-1, indexed
// by significance) witnesses the carry out of each limb; the most significant
// limb must produce no carry.  Every constraint is guarded by `guard`.
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
			// left-hand side: base + delta (+ carry-in) (+ 1 at the least
			// significant limb).
			lhs = mirc.Variable[register.Id, Expr[F]](base[i], w, baseShift).
				Add(mirc.Variable[register.Id, Expr[F]](delta[i], w, 0))
			// right-hand side accumulates the output limb and the outgoing carry.
			rhs = outVar
		)
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
