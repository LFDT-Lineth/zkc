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
	"fmt"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/bus"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
)

// These benchmarks measure the balance check itself: one pass of AcceptsGroup
// over a synthetic one-sender / one-receiver bus.  They exist to put numbers on
// the cost of the tally, whose every row does one hash, one map lookup and one
// map update, plus an allocation for the message.
//
// The dimensions that matter:
//
//   - rows: how many selected rows the check must walk.
//   - distinct: how many distinct messages those rows carry.  With few distinct
//     messages the tally stays small and most rows hit an existing key, which is
//     where a reusable message buffer would pay off.
//   - balanced: a balanced bus is the worst case for the walk, since every row
//     is scanned and nothing can short-circuit.  An unbalanced one additionally
//     pays the failure path, which rescans every port twice via count().
//   - field: koalabear hashes an element with one multiply, bls12_377 with four,
//     so comparing the two isolates how much of the time is hashing.
func Benchmark_Bus_Koalabear(b *testing.B) {
	benchmarkBusMatrix[koalabear.Element](b)
}

func Benchmark_Bus_Bls12_377(b *testing.B) {
	benchmarkBusMatrix[bls12_377.Element](b)
}

// benchmarkBusMatrix runs the balance check over a small matrix of shapes.
func benchmarkBusMatrix[F field.Element[F]](b *testing.B) {
	for _, rows := range []uint64{1_000, 100_000} {
		// Either every row carries its own message, or they are drawn from a
		// small pool so most rows hit a key already in the tally.
		for _, messages := range []struct {
			name     string
			distinct uint64
		}{
			{"alldistinct", rows},
			{"duplicated", 16},
		} {
			for _, balanced := range []bool{true, false} {
				name := fmt.Sprintf("rows=%d/%s/%s", rows, messages.name, verdict(balanced))
				//
				b.Run(name, func(b *testing.B) {
					benchmarkBus[F](b, rows, messages.distinct, balanced)
				})
			}
		}
	}
}

// benchmarkBus times AcceptsGroup over a single synthetic trace.  The trace is
// built once, outside the timed region, since only the check is of interest.
func benchmarkBus[F field.Element[F]](b *testing.B, rows, distinct uint64, balanced bool) {
	var (
		constraint = bus.NewConstraint[F]("bench", []bus.Port{onePort(0)}, []bus.Port{onePort(1)})
		tr         = benchTrace[F](rows, distinct, balanced)
	)
	//
	b.ReportAllocs()
	b.ResetTimer()
	//
	for range b.N {
		// Guard against measuring a check that silently stopped doing its job.
		if failure := constraint.AcceptsGroup(tr); (failure == nil) != balanced {
			b.Fatalf("unexpected verdict (balanced=%t)", balanced)
		}
	}
}

// benchTrace builds a one-sender / one-receiver trace of the given number of
// rows, carrying messages drawn from a pool of the given size.  The receiver
// sees them in reverse order, so the check cannot benefit from the two sides
// being similarly ordered.  When balanced is false one receive is perturbed,
// leaving exactly one message over-sent and one over-received.
func benchTrace[F field.Element[F]](rows, distinct uint64, balanced bool) trace.Trace[F] {
	var (
		sends    = make([][2]uint64, rows)
		receives = make([][2]uint64, rows)
	)
	//
	for i := range rows {
		var message = [2]uint64{(i % distinct) + 1, (i % distinct) + 2}
		//
		sends[i] = message
		receives[rows-1-i] = message
	}
	//
	if !balanced {
		receives[rows-1][1] += distinct + 1
	}
	//
	return trace.NewArray([]*trace.CompactModule[F]{
		benchModule[F]("sndr", sends),
		benchModule[F]("rcvr", receives),
	})
}

// benchModule builds a module with columns (SEL, A, B) holding one selected row
// per given message.
func benchModule[F field.Element[F]](name string, messages [][2]uint64) *trace.CompactModule[F] {
	var columns []trace.ColumnDescriptor
	//
	for _, col := range []string{"SEL", "A", "B"} {
		columns = append(columns, trace.NewColumnDescriptor(col, util.None[uint]()))
	}
	//
	mod := trace.InitCompactModule[F](trace.NewModuleDescriptor(name, columns))
	//
	for _, message := range messages {
		mod.Append(field.Uint64[F](1), field.Uint64[F](message[0]), field.Uint64[F](message[1]))
	}
	//
	return mod
}

// verdict names the expected outcome, for use in benchmark names.
func verdict(balanced bool) string {
	if balanced {
		return "balanced"
	}
	//
	return "unbalanced"
}
