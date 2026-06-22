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
package util

import "math"

// Counter is a simple countdown which fires (returns true from Tick) once every
// `initial` ticks.  It begins at its initial value and, on each Tick,
// decrements the current count; upon reaching zero it resets to the initial
// value and reports true.  This is useful, for example, for triggering an
// action periodically (e.g. taking a checkpoint every N instructions).
type Counter struct {
	// count is the number of ticks remaining until the counter next fires.
	count uint64
	// initial is the value to which count is reset whenever the counter fires.
	initial uint64
}

// NewCounter constructs a counter which fires every n ticks.
func NewCounter(n uint64) Counter {
	return Counter{n, n}
}

// NewCounterOnce constructs a counter which fires once after n ticks.
func NewCounterOnce(initial uint64) Counter {
	return Counter{initial, math.MaxUint64}
}

// Tick decrements the counter, returning true if it has reached zero.  When it
// fires, the counter is automatically reset to its initial value.
func (p *Counter) Tick() bool {
	p.count--
	//
	if p.count == 0 {
		p.count = p.initial
		return true
	}
	//
	return false
}
