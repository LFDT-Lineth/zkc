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
package bus_test

import (
	"testing"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
)

// F is the field used throughout these tests (any field would do).
type F = gf251.Element

// Compile-time check that a bus constraint satisfies the constraint interface.
var _ sc.Constraint[F] = bus.Constraint[F]{}

// These tests exercise the multiset mechanics of the bus constraint
// in isolation, using hand-built traces.  End-to-end coverage (source
// declarations, lowering, padding, field iteration) comes from the
// Test_Valid_Bus_* / Test_Invalid_Bus_* fixtures under testdata/corset/.

// Every module in these tests has three columns (SEL, A, B), where SEL is the
// selector and (A, B) the message.  onePort describes such a module's port.
func onePort(module sc.ModuleId) bus.Port {
	return bus.NewPort(module, columnId(0), columnId(1), columnId(2))
}

// xfer constructs the standard bus used by most tests below: module 0 sends
// and module 1 receives.
func xfer() bus.Constraint[F] {
	return bus.NewConstraint[F]("xfer", []bus.Port{onePort(0)}, []bus.Port{onePort(1)})
}

func Test_Bus_Balanced(t *testing.T) {
	tr := newTrace(
		newModule("alpha", row(1, 5, 7), row(0, 0, 0), row(1, 9, 2)),
		newModule("beta", row(1, 9, 2), row(1, 5, 7)))
	//
	if failure := xfer().AcceptsGroup(tr); failure != nil {
		t.Errorf("balanced bus rejected: %s", failure.Message())
	}
}

func Test_Bus_EmptyIsBalanced(t *testing.T) {
	// A bus on which nothing is sent or received balances trivially, even
	// when the participating modules have (unselected) rows.
	tr := newTrace(
		newModule("alpha", row(0, 1, 2)),
		newModule("beta"))
	//
	if failure := xfer().AcceptsGroup(tr); failure != nil {
		t.Errorf("empty bus rejected: %s", failure.Message())
	}
}

func Test_Bus_Duplicates(t *testing.T) {
	// Two identical sends require two identical receives: one is not enough.
	// This is exactly what distinguishes a bus from a (de-duplicating)
	// lookup.
	unbalanced := newTrace(
		newModule("alpha", row(1, 5, 7), row(1, 5, 7)),
		newModule("beta", row(1, 5, 7)))
	balanced := newTrace(
		newModule("alpha", row(1, 5, 7), row(1, 5, 7)),
		newModule("beta", row(1, 5, 7), row(1, 5, 7)))
	//
	failure := xfer().AcceptsGroup(unbalanced)
	if failure == nil {
		t.Errorf("bus with 2 sends vs 1 receive accepted")
	} else if f := failure.(*bus.Failure[F]); f.Sent != 2 || f.Received != 1 {
		t.Errorf("wrong counts reported: sent %d, received %d", f.Sent, f.Received)
	}
	//
	if failure := xfer().AcceptsGroup(balanced); failure != nil {
		t.Errorf("bus with 2 sends vs 2 receives rejected: %s", failure.Message())
	}
}

func Test_Bus_SelectorZeroInert(t *testing.T) {
	// Junk payloads on rows whose selector is zero contribute nothing.  This
	// is what makes (all-zero) padding rows harmless.
	tr := newTrace(
		newModule("alpha", row(1, 5, 7), row(0, 123, 250)),
		newModule("beta", row(0, 88, 99), row(1, 5, 7)))
	//
	if failure := xfer().AcceptsGroup(tr); failure != nil {
		t.Errorf("junk on unselected rows unbalanced the bus: %s", failure.Message())
	}
}

func Test_Bus_ReceiveNeverSent(t *testing.T) {
	tr := newTrace(
		newModule("alpha", row(1, 5, 7)),
		newModule("beta", row(1, 5, 7), row(1, 8, 1)))
	//
	failure := xfer().AcceptsGroup(tr)
	if failure == nil {
		t.Errorf("bus with an unsent message accepted")
	} else if f := failure.(*bus.Failure[F]); f.Sent != 0 || f.Received != 1 {
		t.Errorf("wrong counts reported: sent %d, received %d", f.Sent, f.Received)
	}
}

func Test_Bus_FanInFanOut(t *testing.T) {
	// Four modules on one bus: two senders (modules 0, 1), two receivers
	// (modules 2, 3).  Messages from different senders are answered by
	// different receivers, and one message is duplicated across senders.
	manyPorts := bus.NewConstraint[F]("many",
		[]bus.Port{onePort(0), onePort(1)},
		[]bus.Port{onePort(2), onePort(3)})
	//
	balanced := newTrace(
		newModule("s1", row(1, 5, 7), row(1, 4, 4)),
		newModule("s2", row(1, 9, 2), row(1, 4, 4)),
		newModule("r1", row(1, 4, 4), row(1, 9, 2)),
		newModule("r2", row(1, 4, 4), row(1, 5, 7)))
	// As above, except one copy of the duplicated message (4,4) is missing.
	unbalanced := newTrace(
		newModule("s1", row(1, 5, 7), row(1, 4, 4)),
		newModule("s2", row(1, 9, 2), row(1, 4, 4)),
		newModule("r1", row(1, 4, 4), row(1, 9, 2)),
		newModule("r2", row(0, 4, 4), row(1, 5, 7)))
	//
	if failure := manyPorts.AcceptsGroup(balanced); failure != nil {
		t.Errorf("balanced fan-in/fan-out bus rejected: %s", failure.Message())
	}
	//
	if manyPorts.AcceptsGroup(unbalanced) == nil {
		t.Errorf("fan-in/fan-out bus missing one duplicate accepted")
	}
}

func Test_Bus_Group(t *testing.T) {
	// Shard 1 only sends; shard 2 only receives.  Together they balance,
	// alone they do not.
	shard1 := newTrace(
		newModule("alpha", row(1, 5, 7), row(1, 9, 2)),
		newModule("beta", row(0, 0, 0)))
	shard2 := newTrace(
		newModule("alpha", row(0, 0, 0)),
		newModule("beta", row(1, 9, 2), row(1, 5, 7)))
	//
	if failure := xfer().AcceptsGroup(shard1, shard2); failure != nil {
		t.Errorf("balanced group rejected: %s", failure.Message())
	}
	//
	if xfer().AcceptsGroup(shard1) == nil {
		t.Errorf("unbalanced shard accepted on its own")
	}
	// Accepts must agree with a group of one.
	if xfer().Accepts(shard1, nil, nil) == nil {
		t.Errorf("Accepts disagrees with AcceptsGroup on a single trace")
	}
}

func Test_Bus_GroupDuplicates(t *testing.T) {
	// A message sent twice in shard 1 is balanced by one receive in shard 1
	// and one in shard 2 — counts pool across the whole group.
	shard1 := newTrace(
		newModule("alpha", row(1, 5, 7), row(1, 5, 7)),
		newModule("beta", row(1, 5, 7)))
	shard2 := newTrace(
		newModule("alpha"),
		newModule("beta", row(1, 5, 7)))
	//
	if failure := xfer().AcceptsGroup(shard1, shard2); failure != nil {
		t.Errorf("group balancing duplicates across shards rejected: %s", failure.Message())
	}
	// A third copy of the receive tips the balance over.
	shard3 := newTrace(
		newModule("alpha"),
		newModule("beta", row(1, 5, 7)))
	//
	if xfer().AcceptsGroup(shard1, shard2, shard3) == nil {
		t.Errorf("group with 2 sends vs 3 receives accepted")
	}
}

// ============================================================================
// Test helpers
// ============================================================================

// columnId is shorthand for constructing a column (i.e. register) identifier.
func columnId(index uint) register.Id {
	return register.NewId(index)
}

// row is shorthand for one row of column values.
func row(values ...uint64) []uint64 {
	return values
}

// newTrace constructs a trace from the given modules; each module's position
// determines its module identifier.
func newTrace(modules ...*trace.CompactModule[F]) trace.Trace[F] {
	return trace.NewArray(modules)
}

// newModule constructs a module with columns (SEL, A, B) holding the given
// rows, where SEL acts as the selector.
func newModule(name string, rows ...[]uint64) *trace.CompactModule[F] {
	var columns []trace.ColumnDescriptor
	//
	for _, col := range []string{"SEL", "A", "B"} {
		columns = append(columns, trace.NewColumnDescriptor(col, util.None[uint]()))
	}
	//
	mod := trace.InitCompactModule[F](trace.NewModuleDescriptor(name, columns))
	//
	for _, values := range rows {
		var elements []F
		//
		for _, v := range values {
			elements = append(elements, field.Uint64[F](v))
		}
		//
		mod.Append(elements...)
	}
	//
	return mod
}
