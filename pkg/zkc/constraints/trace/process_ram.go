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
package trace

import (
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// ramTraceLayout records the column offsets of a RAM trace row.  The order MUST
// match constraints.computeRamLayout (the schema this trace is checked against):
// [ADDRESS, VALUE_WRITTEN, EXEC, FINL, IS_WRITE, VALUE_READ, TIMESTAMP_WRITTEN,
//
//	TIMESTAMP_READ, TIMESTAMP_DELTA, ADDRESS_DELTA, TS_CARRY, ADDR_CARRY,
//	EXEC_WRITE, EXEC_READ].
type ramTraceLayout struct {
	nAddr, nData, nStamp                         int
	valueWritten, exec, finl, isWrite, valueRead int
	tsWritten, tsRead, tsDelta                   int
	addrDelta, tsCarry, addrCarry                int
	execWrite, execRead                          int
	width                                        int
}

func newRamTraceLayout(nAddr, nData, nStamp int) ramTraceLayout {
	var l ramTraceLayout
	//
	l.nAddr, l.nData, l.nStamp = nAddr, nData, nStamp
	// address occupies [0, nAddr); value-written [nAddr, nAddr+nData).
	l.valueWritten = nAddr
	base := nAddr + nData
	l.exec = base
	l.finl = base + 1
	l.isWrite = base + 2
	l.valueRead = base + 3
	l.tsWritten = l.valueRead + nData
	l.tsRead = l.tsWritten + nStamp
	l.tsDelta = l.tsRead + nStamp
	l.addrDelta = l.tsDelta + nStamp
	l.tsCarry = l.addrDelta + nAddr
	l.addrCarry = l.tsCarry + (nStamp - 1)
	l.execWrite = l.addrCarry + (nAddr - 1)
	l.execRead = l.execWrite + 1
	l.width = l.execRead + 1
	//
	return l
}

// ramAccess is one logical read-write memory access reconstructed from the
// per-lane access log: a shared timestamp / write-flag, the physical start
// address of its lanes, and the value on each lane.
type ramAccess[W Word[W]] struct {
	timestamp uint64
	isWrite   bool
	physStart uint64
	values    []W
}

// InitReadWriteMemory initialises a trace module for a RandomAccessMemory.
func initReadWriteMemory[W Word[W], F Element[F], M ModuleBuilder[F, M]](cfg field.Config, m vm.Memory[W]) (module M) {
	var (
		regs = array.Map(m.Registers(), toRtraceRegister)
		// Timestamp limb widths, most-significant first: the memory's declared
		// timestamp width splits exactly as on the constraint side
		// (constraints.computeRamLayout).
		tsWidths = array.Reverse(register.LimbWidths(cfg.RegisterWidth, m.TimestampWidth()))
		u1       = util.Some[uint](1)
	)
	// EXEC, FINL, IS_WRITE.
	regs = append(regs,
		rtrace.NewColumnDescriptor(RAM_EXEC_NAME, u1),
		rtrace.NewColumnDescriptor(RAM_FINL_NAME, u1),
		rtrace.NewColumnDescriptor(RAM_IS_WRITE_NAME, u1),
	)
	// VALUE_READ (same widths as the data lanes, native included).
	for j, r := range m.DataRegisters() {
		regs = append(regs, rtrace.NewColumnDescriptor(RamLimbName(RAM_VALUE_READ_PREFIX, uint(j)), r.Bitwidth()))
	}
	// TIMESTAMP_WRITTEN / READ / DELTA.
	for _, prefix := range []string{RAM_TS_WRITTEN_PREFIX, RAM_TS_READ_PREFIX, RAM_TS_DELTA_PREFIX} {
		for k, w := range tsWidths {
			regs = append(regs, rtrace.NewColumnDescriptor(RamLimbName(prefix, uint(k)), util.Some(w)))
		}
	}
	// ADDRESS_DELTA (same widths as the address lanes).
	for j, r := range m.AddressRegisters() {
		regs = append(regs, rtrace.NewColumnDescriptor(RamLimbName(RAM_ADDR_DELTA_PREFIX, uint(j)), r.Bitwidth()))
	}
	// TS_CARRY / ADDR_CARRY (one fewer than their value's limbs; 1-bit).
	for k := uint(1); k < uint(len(tsWidths)); k++ {
		regs = append(regs, rtrace.NewColumnDescriptor(RamLimbName(RAM_TS_CARRY_PREFIX, k-1), u1))
	}
	//
	for k := uint(1); k < m.NumInputs(); k++ {
		regs = append(regs, rtrace.NewColumnDescriptor(RamLimbName(RAM_ADDR_CARRY_PREFIX, k-1), u1))
	}
	// EXEC_WRITE / EXEC_READ (the per-kind lookup selectors).
	regs = append(regs,
		rtrace.NewColumnDescriptor(RAM_EXEC_WRITE_NAME, u1),
		rtrace.NewColumnDescriptor(RAM_EXEC_READ_NAME, u1),
	)
	//Done
	return module.Initialise(m.Name(), regs)
}

// traceReadWriteMemory materialises the trace of a read-write (RAM) memory: one
// row per logical access (grouped from the per-lane access log), in access order,
// preceded by a padding row.  It fills the execution-phase columns declared by
// constraints.translateReadWriteMemory; the finalization phase (FINL) is left
// empty (the rcv/snd consistency bus and finalization rows are a follow-up), so
// FINL-guarded columns (ADDRESS_DELTA, ADDR_CARRY) stay zero.
func traceReadWriteMemory[W Word[W], F Element[F]](m vm.RuntimeMemory[W], module Module[F], cfg field.Config,
	scratch []F) {
	//
	var (
		geometry = m.Descriptor()
		addrRegs = geometry.AddressRegisters()
		nAddr    = int(geometry.NumInputs())
		nData    = int(geometry.NumOutputs())
		// Timestamp limb widths, most-significant first (matches computeRamLayout).
		tsWidths = array.Reverse(register.LimbWidths(cfg.RegisterWidth, geometry.TimestampWidth()))
		layout   = newRamTraceLayout(nAddr, nData, len(tsWidths))
		accesses = groupRamAccesses[W](m.AccessLog(), nData)
		// Per-physical-cell read-side state, tracked by replaying the log forward;
		// every cell starts at value 0, timestamp 0.
		cellValue = map[uint64]W{}
		cellStamp = map[uint64]uint64{}
		//
		width = module.Width()
	)
	// Row 0 is a padding row (EXEC = FINL = 0).
	module.Append(paddingRow(scratch[:width])...)
	//
	for _, acc := range accesses {
		var (
			row     = scratch[:width]
			logical = acc.physStart / uint64(nData)
			// Read-side: the row's cells share one last-write timestamp (accesses
			// are row-atomic), so read it from the first lane.
			tsRead = cellStamp[acc.physStart]
			// The interpreter clock ticks before each access and the threaded
			// stamps count from one, so the recorded timestamp IS the caller's
			// stamp — no re-basing needed.  Timestamp zero is reserved for the
			// initial state of an untouched cell (the zero value of cellStamp).
			tsWr    = acc.timestamp
			tsDelta = tsWr - tsRead - 1
		)
		// EXEC = 1, IS_WRITE from the access; FINL = 0 (zero value).  The
		// per-kind lookup selectors follow: EXEC_WRITE = EXEC * IS_WRITE,
		// EXEC_READ = EXEC * (1 - IS_WRITE).
		row[layout.exec] = field.Uint64[F](1)
		row[layout.isWrite] = field.Uint1[F](acc.isWrite)
		row[layout.execWrite] = field.Uint1[F](acc.isWrite)
		row[layout.execRead] = field.Uint1[F](!acc.isWrite)
		// ADDRESS (logical) split across the address lanes (offset 0).
		copyAddressLines(logical, addrRegs, row[0:nAddr])
		// VALUE_WRITTEN / VALUE_READ per lane.
		for j := range nData {
			row[layout.valueWritten+j] = wordToField[W, F](acc.values[j])
			row[layout.valueRead+j] = wordToField[W, F](cellValue[acc.physStart+uint64(j)])
		}
		// TIMESTAMP_WRITTEN / READ / DELTA, split into limbs.
		fillLimbs(row[layout.tsWritten:], tsWr, tsWidths)
		fillLimbs(row[layout.tsRead:], tsRead, tsWidths)
		fillLimbs(row[layout.tsDelta:], tsDelta, tsWidths)
		// TS_CARRY: carries of the limb addition TIMESTAMP_READ + 1 + TIMESTAMP_DELTA.
		for s, c := range timestampCarries(tsRead, tsDelta, tsWidths) {
			row[layout.tsCarry+s] = field.Uint64[F](c)
		}
		// Update the read-side state for every touched cell.
		for j := range nData {
			cellValue[acc.physStart+uint64(j)] = acc.values[j]
			cellStamp[acc.physStart+uint64(j)] = tsWr
		}
		//
		module.Append(row...)
	}
}

// groupRamAccesses groups the per-lane access log into logical accesses.  Every
// lane of one logical access shares a single (Tick) timestamp and its lanes are
// logged consecutively at ascending physical addresses, so a run of equal
// timestamps is exactly one logical access.
func groupRamAccesses[W Word[W]](log []vm.AccessData[W], nData int) []ramAccess[W] {
	var out []ramAccess[W]
	//
	for i := 0; i < len(log); {
		var (
			ts  = log[i].TimestampWritten()
			acc = ramAccess[W]{timestamp: ts, isWrite: log[i].IsWrite(), physStart: log[i].Address()}
		)
		//
		for i < len(log) && log[i].TimestampWritten() == ts {
			acc.values = append(acc.values, log[i].ValueWritten())
			i++
		}
		//
		out = append(out, acc)
	}
	//
	return out
}

// fillLimbs writes value into the limb columns at the head of row, most-
// significant limb first, matching copyAddressLines / the schema's limb order.
func fillLimbs[F Element[F]](row []F, value uint64, widths []uint) {
	// Most-significant limb comes first, so fill from the least significant end.
	for i := len(widths); i > 0; i-- {
		var (
			f    F
			w    = widths[i-1]
			mask = (uint64(1) << w) - 1
		)
		//
		row[i-1] = f.SetUint64(value & mask)
		value >>= w
	}
}

// timestampCarries returns the per-boundary carry of the multi-limb addition
// TIMESTAMP_READ + 1 + TIMESTAMP_DELTA (= TIMESTAMP_WRITTEN), indexed by
// significance (index 0 = carry out of the least significant limb).  Each carry
// is 0 or 1.  widths are most-significant-limb first.
func timestampCarries(tsRead, tsDelta uint64, widths []uint) []uint64 {
	var (
		L       = len(widths)
		read    = splitLimbs(tsRead, widths)
		delta   = splitLimbs(tsDelta, widths)
		carries = make([]uint64, 0, max(0, L-1))
		carry   = uint64(0)
	)
	// Add from the least significant limb (index L-1) up to the most significant.
	for s := 0; s < L; s++ {
		var (
			i     = L - 1 - s
			total = read[i] + delta[i] + carry
		)
		//
		if s == 0 {
			total++
		}
		//
		carry = total >> widths[i]
		//
		if s < L-1 {
			carries = append(carries, carry)
		}
	}
	//
	return carries
}

// splitLimbs decomposes value into limbs of the given (most-significant-first)
// widths.
func splitLimbs(value uint64, widths []uint) []uint64 {
	limbs := make([]uint64, len(widths))
	//
	for i := len(widths); i > 0; i-- {
		var w = widths[i-1]

		limbs[i-1] = value & ((uint64(1) << w) - 1)
		value >>= w
	}
	//
	return limbs
}
