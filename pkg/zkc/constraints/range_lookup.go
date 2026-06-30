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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
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
// n <= MAX_STATIC_RANGE_WIDTH; wider registers are range-checked recursively by
// a call (lowered via addCallLookups), so only the static tables are collected
// here.
func indexRangeTables[F field.Element[F]](modules []vm.Module, maxStaticWidth uint) map[uint]rangeTable {
	tables := make(map[uint]rangeTable)
	//
	for id, m := range modules {
		// Only the fully-enumerated static tables serve as direct lookup targets;
		mem, ok := m.(vm.Memory[F])
		if !ok || !mem.IsStatic() || !strings.HasPrefix(m.Name(), rangeModulePrefix) {
			continue
		}
		// The "value" column holds the enumerated values and is the lookup target.
		valId, ok := m.HasRegister(rangeValueName)
		if !ok {
			continue
		}
		//
		if w := m.Register(valId).Width(); w <= maxStaticWidth {
			tables[w] = rangeTable{schema.ModuleId(id), valId}
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
// Registers wider than MAX_STATIC_RANGE_WIDTH have no static table; they are
// range-checked at runtime by a recursive call which addCallLookups lowers into
// a lookup.  Native (field-element) and zero-width registers are not
// range-checked at all.
func addRangeProofConstraints[F field.Element[F]](mod *schema.Table[F, mir.Constraint[F]], ctx schema.ModuleId,
	regs []register.Register, tables map[uint]rangeTable, maxStaticWidth uint) {
	// TODO: lots of perf possible here, see
	// https://github.com/LFDT-Lineth/zkc/issues/1910
	// https://github.com/LFDT-Lineth/zkc/issues/1907
	// https://github.com/LFDT-Lineth/zkc/issues/1911
	for i, reg := range regs {
		// Native registers are not range-checked.
		if reg.IsNative() || reg.Width() == 0 {
			continue
		}
		//
		table, ok := tables[reg.Width()]
		if !ok {
			// a width <= MAX_STATIC_RANGE_WIDTH must always have a static table.
			if reg.Width() <= maxStaticWidth {
				panic(fmt.Sprintf("missing static range table for width %d", reg.Width()))
			}
			// Wider registers are range-checked recursively via a call lookup.
			continue
		}
		//
		var (
			regId  = register.NewId(uint(i))
			handle = fmt.Sprintf("range_%d_%d", ctx, regId.Unwrap())
			// Source: the register's value on the current row.
			source = lookup.UnfilteredVector(ctx,
				term.RawRegisterAccess[F, mir.Term[F]](regId, reg.Width(), 0))
			// Target: the enumerated value column of the matching $range_un table.
			target = lookup.UnfilteredVector(table.module,
				term.RawRegisterAccess[F, mir.Term[F]](table.value, reg.Width(), 0))
		)
		//
		mod.AddConstraints(mir.NewLookupConstraint(handle,
			[]mir.LookupVector[F]{target}, []mir.LookupVector[F]{source}))
	}
}
