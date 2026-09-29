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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Future represents the (future) result of tracing a given shard, which may (or
// may not) produce a value and can additionally produce one or more errors.
type Future[F field.Element[F]] func() (util.Option[Shard[F]], []error)

// Trace represents a complete (sharded) trace.  That is, an array of shards.
type Trace[F field.Element[F]] []Shard[F]

// LazyTrace represents a trace whose shards are loaded lazilty (i.e. on
// demand).  The purpose of a lazy trace is to make it possible to consume (i.e.
// process) the shards without holding all of them in memory at once.  The lazy
// trace can be viewed as an array of "promises" or "futures".
type LazyTrace[F field.Element[F]] struct {
	// indicates whether or not to use parallelism
	parallel bool
	// array of future (or "promised") results.
	futures []Future[F]
}

// NewLazyTrace constructs a lazy trace from zero or more futures.
func NewLazyTrace[F field.Element[F]](futures ...Future[F]) LazyTrace[F] {
	return LazyTrace[F]{true, futures}
}

// WithParallelism enables the use of parallelism for the various functions
// which operate on this trace (e.g. Apply).
func (p LazyTrace[F]) WithParallelism(enable bool) LazyTrace[F] {
	p.parallel = enable
	return p
}

// Len returns the number of remaining shards.
func (p LazyTrace[F]) Len() uint {
	return uint(len(p.futures))
}

// Apply the given handler to each shard in turn, where the handler's first
// argument is the shard index.  This uses a parallel map by default (i.e.
// unless parallelism is disabled) but computes each shard on-demand and then
// discards it once the handler has finished.  Thus, only the current shards
// being processed are held in memory at any given moment.
//
// There are two failure modes: recoverable and non-recoverable errors.  A
// recoverable error indicates the machine encountered a failure (e.g. it
// executed a fail instruction), but a shard was still constructed.  A
// non-recoverable error indicates some other kind of abrupt internal or I/O
// error arose, and no shard was produced.  Recoverable errors are instances of
// *vm.Failure.  If no errors arose, the handler is called (in a non-determinic
// order) on all shards. If only recoverable errors arose, then again the
// handler is called on all shards. Otherwise, the handler is called only on
// those shards which were actually produced.
//
// NOTE: at this time, early termination is not supported.  Thus, the handler is
// always called on all shards which are produced, regardless of whether some
// non-recoverable has already occurred.
func (p LazyTrace[F]) Apply(handler func(uint, Shard[F])) []error {
	var (
		mapper = func(id uint, f Future[F]) []error {
			var shard, errs = f()
			// Apply handler (if applicable)
			if shard.HasValue() {
				handler(id, shard.Unwrap())
			}
			// Return any errors
			return errs
		}
		//
		errors [][]error
	)
	//
	if p.parallel {
		errors = array.ParallelMap(p.futures, mapper)
	} else {
		errors = array.Map(p.futures, mapper)
	}
	// Flattern error(s) into a single array.
	return array.FlatMap(errors, func(es []error) []error { return es })
}

// Get computes the ith shard in this array, or fails with one or more errors.
// This is, in effect, a raw accessor which can be used instead of Apply() above
// when more flexibility is required.  The shard is always returned unless an
// unrecoverable error arises.  However, it is also possible that both
// shard.HasValue() and len(errors) > 0 hold at the same time.  This happens
// when  a recoverable error is encountered (see discussion of Apply() for more
// on this).
func (p LazyTrace[F]) Get(ith uint) (shard util.Option[Shard[F]], errors []error) {
	return p.futures[ith]()
}
