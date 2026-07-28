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
	"math/big"

	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// accessSite records one read-write memory access in a function: the line
// selector under which it fires (the $is_pc_k line selector for a multi-line
// caller, or $ret for an atomic one — at most one RAM access lands on any row,
// so the line selector alone identifies it) and the access bytecode itself.
type accessSite[W vm.Word[W]] struct {
	sel register.Id
	rw  *vm.BytecodeReadWrite[W]
}

// addMemoryLookups emits, for each read-write (RAM) memory a function accesses, a
// SINGLE lookup constraint mapping that function's "RAM port" columns onto the
// RAM module's rows.  The port is one column family per accessed memory
// (RAM_TRIGGER, []RAM_ADDRESS, []RAM_VALUE, []RAM_TIMESTAMP, RAM_IS_WRITE, per
// the spec) which, on each row, holds whichever access fires there (or nothing,
// TRIGGER=0).  Because the vectoriser guarantees at most one RAM access per row,
// a single lookup (source-gated on TRIGGER) sweeps every access.
//
// The port columns are filled during trace expansion (selector-weighted sum over
// the memory's access sites) and bound by matching constraints — the "connecting
// constraints" tying, on each access line, the accessed register to the port
// column (e.g. on `a = ram[x]`, `RAM_VALUE == a`).
func addMemoryLookups[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	fn *vm.Function[W], pcSelectors []register.Id, ret register.Id, infos []vm.Module[W],
	callerRegs []register.Register, fieldCfg field.Config) {
	//
	var (
		sites     = map[uint16][]accessSite[W]{}
		order     []uint16
		multiLine = len(pcSelectors) != 0
	)
	// Gather access sites per memory, preserving first-seen order.
	for pc, vec := range fn.Vectors() {
		for _, code := range vec.Bytecodes {
			rw, ok := code.(*vm.BytecodeReadWrite[W])
			if !ok {
				continue
			}
			//
			mem, ok := infos[rw.Id].(*vm.Memory[W])
			if !ok || !mem.IsReadWrite() || len(rw.Stamp) == 0 {
				continue
			}
			// Line selector: the per-line $is_pc_k for a multi-line caller, else the
			// atomic caller's $ret activity line.
			sel := ret
			if multiLine {
				sel = pcSelectors[pc]
			}
			//
			if _, seen := sites[rw.Id]; !seen {
				order = append(order, rw.Id)
			}
			//
			sites[rw.Id] = append(sites[rw.Id], accessSite[W]{sel, rw})
		}
	}
	//
	for _, memId := range order {
		emitMemoryLookup[W, F](mod, ctx, callerRegs, memId, infos[memId].(*vm.Memory[W]), sites[memId], fieldCfg)
	}
}

// emitMemoryLookup creates the RAM-port columns for one accessed memory, fills
// and binds them, and emits the single caller->RAM lookup.
func emitMemoryLookup[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	callerRegs []register.Register, memId uint16, mem *vm.Memory[W], sites []accessSite[W], fieldCfg field.Config) {
	//
	var (
		layout  = computeRamLayout(mem, fieldCfg)
		memName = mem.Name()
		padding big.Int
		// Port columns.
		triggerId = register.NewId(mod.Width())
	)
	mod.AddRegisters(register.NewComputed(io.RamPortName(memName, "trigger"), 1, padding))
	//
	var (
		addressIds = addPortLimbs(mod, memName, "address", layout.addrWidths, padding)
		valueIds   = addPortLimbs(mod, memName, "value", layout.dataWidths, padding)
		tsIds      = addPortLimbs(mod, memName, "ts", layout.tsWidths, padding)
		isWriteId  = register.NewId(mod.Width())
	)
	mod.AddRegisters(register.NewComputed(io.RamPortName(memName, "is_write"), 1, padding))
	// Fill + bind TRIGGER = Σ_k sel_k  (0/1: at most one site fires per row).
	fillBindPort[W, F](mod, ctx, fmt.Sprintf("ram_%d_trigger", memId), triggerId, 1,
		selectorAddends(sites, false))
	// Fill + bind IS_WRITE = Σ_{write sites} sel_k.
	fillBindPort[W, F](mod, ctx, fmt.Sprintf("ram_%d_is_write", memId), isWriteId, 1,
		selectorAddends(sites, true))
	// Fill + bind each address / value / timestamp limb: the "connecting"
	// constraint RAM_<fam>[j] == Σ_k sel_k · <fam>_k[j].
	bindLimbs[W, F](mod, ctx, memId, "address", addressIds, layout.addrWidths, callerRegs, sites,
		func(rw *vm.BytecodeReadWrite[W]) []vm.RegisterId { return rw.Address })
	bindLimbs[W, F](mod, ctx, memId, "value", valueIds, layout.dataWidths, callerRegs, sites,
		func(rw *vm.BytecodeReadWrite[W]) []vm.RegisterId { return rw.Data })
	bindLimbs[W, F](mod, ctx, memId, "ts", tsIds, layout.tsWidths, callerRegs, sites,
		func(rw *vm.BytecodeReadWrite[W]) []vm.RegisterId { return rw.Stamp })
	// Emit the single caller->RAM lookup.  Source (caller port) gated on TRIGGER;
	// target (RAM rows) gated on EXEC; tuples [ADDRESS, VALUE, TIMESTAMP, IS_WRITE].
	var (
		handle = fmt.Sprintf("ram_%d_%d", ctx, memId)
		source = lookup.FilteredVector(ctx, filterAccess[F](triggerId),
			portTuple[F](addressIds, layout.addrWidths, valueIds, layout.dataWidths, tsIds, layout.tsWidths, isWriteId)...)
		target = lookup.FilteredVector(schema.ModuleId(memId), filterAccess[F](layout.exec),
			portTuple[F](layout.address, layout.addrWidths, layout.valueWritten, layout.dataWidths,
				layout.tsWritten, layout.tsWidths, layout.isWrite)...)
	)
	//
	mod.AddConstraints(mir.NewLookupConstraint(handle, []mir.LookupVector[F]{target}, []mir.LookupVector[F]{source}))
}

// addend is one term of a selector-weighted sum: sel (a 1-bit line selector) and,
// optionally, a register whose value it selects.
type addend struct {
	sel    register.Id
	reg    register.Id
	width  uint
	hasReg bool
}

// selectorAddends returns the bare-selector addends for TRIGGER (writeOnly=false,
// all sites) or IS_WRITE (writeOnly=true, only write sites).
func selectorAddends[W vm.Word[W]](sites []accessSite[W], writeOnly bool) []addend {
	var out []addend
	//
	for _, s := range sites {
		if writeOnly && !s.rw.Write {
			continue
		}
		//
		out = append(out, addend{sel: s.sel})
	}
	//
	return out
}

// bindLimbs fills and binds one RAM-port limb family (address / value / ts): for
// each limb j, RAM_<fam>[j] == Σ_k sel_k · <fam>_k[j].
func bindLimbs[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	memId uint16, family string, portIds []register.Id, widths []uint, callerRegs []register.Register,
	sites []accessSite[W], pick func(*vm.BytecodeReadWrite[W]) []vm.RegisterId) {
	//
	for j := range portIds {
		addends := make([]addend, len(sites))
		//
		for i, s := range sites {
			id := toRegisterIds(pick(s.rw))[j]
			addends[i] = addend{sel: s.sel, reg: id, width: callerRegs[id.Unwrap()].WidthOrNative(), hasReg: true}
		}
		//
		fillBindPort[W, F](mod, ctx, fmt.Sprintf("ram_%d_%s_%d", memId, family, j), portIds[j], widths[j], addends)
	}
}

// fillBindPort fills a RAM-port column during trace expansion with the
// selector-weighted sum of its addends, and adds the matching binding constraint.
func fillBindPort[W vm.Word[W], F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	handle string, targetId register.Id, targetWidth uint, addends []addend) {
	//
	mod.AddAssignments(assignment.NewComputedRegister[F](sumComputation(addends), true, ctx, targetId))
	mod.AddConstraints(sumConstraint[F](handle, ctx, targetId, targetWidth, addends))
}

// sumComputation builds the trace-expansion computation Σ_k sel_k [· reg_k] over
// word.BigEndian.
func sumComputation(addends []addend) term.Computation[word.BigEndian] {
	type T = mir.Term[word.BigEndian]
	//
	if len(addends) == 0 {
		return term.NewComputation[word.BigEndian, mir.LogicalTerm[word.BigEndian], T](
			term.Const[word.BigEndian, T](field.Zero[word.BigEndian]()))
	}
	//
	terms := make([]T, len(addends))
	//
	for i, a := range addends {
		var sel T = term.RawRegisterAccess[word.BigEndian, T](a.sel, 1, 0)
		//
		if a.hasReg {
			var reg T = term.RawRegisterAccess[word.BigEndian, T](a.reg, a.width, 0)

			terms[i] = term.Product[word.BigEndian, T](sel, reg)
		} else {
			terms[i] = sel
		}
	}
	//
	return term.NewComputation[word.BigEndian, mir.LogicalTerm[word.BigEndian], T](term.Sum[word.BigEndian, T](terms...))
}

// sumConstraint builds the binding "target == Σ_k sel_k [· reg_k]".
func sumConstraint[F field.Element[F]](handle string, ctx schema.ModuleId, targetId register.Id, targetWidth uint,
	addends []addend) mir.Constraint[F] {
	//
	var rhs Expr[F]
	//
	if len(addends) == 0 {
		rhs = mirc.Number[register.Id, Expr[F]](0)
	} else {
		terms := make([]Expr[F], len(addends))
		//
		for i, a := range addends {
			sel := mirc.Variable[register.Id, Expr[F]](a.sel, 1, 0)
			//
			if a.hasReg {
				terms[i] = sel.Multiply(mirc.Variable[register.Id, Expr[F]](a.reg, a.width, 0))
			} else {
				terms[i] = sel
			}
		}
		//
		rhs = mirc.Sum(terms)
	}
	//
	target := mirc.Variable[register.Id, Expr[F]](targetId, targetWidth, 0)
	//
	return mir.NewVanishingConstraint(handle, ctx, util.None[int](), target.Equals(rhs).AsLogical())
}

// addPortLimbs appends one computed RAM-port column per limb width and returns
// their ids.
func addPortLimbs[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], mem, family string,
	widths []uint, padding big.Int) []register.Id {
	//
	ids := make([]register.Id, len(widths))
	//
	for k, w := range widths {
		ids[k] = register.NewId(mod.Width())
		mod.AddRegisters(register.NewComputed(io.RamPortName(mem, fmt.Sprintf("%s_%d", family, k)), w, padding))
	}
	//
	return ids
}

// portTuple builds the lookup tuple [addresses, values, timestamps, is_write] as
// register accesses (row offset 0).
func portTuple[F field.Element[F]](addressIds []register.Id, addrWidths []uint, valueIds []register.Id,
	dataWidths []uint, tsIds []register.Id, tsWidths []uint, isWriteId register.Id) []*mir.RegisterAccess[F] {
	//
	return concatAccesses(
		ramAccesses[F](addressIds, addrWidths),
		ramAccesses[F](valueIds, dataWidths),
		ramAccesses[F](tsIds, tsWidths),
		[]*mir.RegisterAccess[F]{filterAccess[F](isWriteId)},
	)
}

// filterAccess builds a 1-bit register access (row offset 0), used as a lookup
// selector.
func filterAccess[F field.Element[F]](id register.Id) *mir.RegisterAccess[F] {
	return term.RawRegisterAccess[F, mir.Term[F]](id, 1, 0)
}

// ramAccesses builds MIR register accesses (row offset 0) for the given register
// ids and matching limb widths.
func ramAccesses[F field.Element[F]](ids []register.Id, widths []uint) []*mir.RegisterAccess[F] {
	terms := make([]*mir.RegisterAccess[F], len(ids))
	//
	for i, id := range ids {
		terms[i] = term.RawRegisterAccess[F, mir.Term[F]](id, widths[i], 0)
	}
	//
	return terms
}

// concatAccesses concatenates register-access slices.
func concatAccesses[F field.Element[F]](accs ...[]*mir.RegisterAccess[F]) []*mir.RegisterAccess[F] {
	var out []*mir.RegisterAccess[F]
	//
	for _, a := range accs {
		out = append(out, a...)
	}
	//
	return out
}
