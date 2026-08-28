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

import (
	"fmt"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
)

// PerfStats provides a snapshot of memory allocation at a given point in time.
type PerfStats struct {
	// Starting time
	startTime time.Time
	// Starting total memory allocation
	startMem uint64
	// Starting number of gc events
	startGc uint32
}

// NewPerfStats creates a new snapshot of the current amount of memory allocated.
func NewPerfStats() *PerfStats {
	var m runtime.MemStats

	startTime := time.Now()

	runtime.ReadMemStats(&m)

	return &PerfStats{startTime, m.TotalAlloc, m.NumGC}
}

// Reset the performance counter
func (p *PerfStats) Reset() uint {
	var (
		m        runtime.MemStats
		exectime = time.Since(p.startTime).Seconds()
	)
	//
	p.startTime = time.Now()
	//
	runtime.ReadMemStats(&m)
	//
	p.startMem = m.TotalAlloc
	p.startGc = m.NumGC
	//
	return uint(exectime)
}

// Log logs the difference between the state now and as it was when the PerfStats object was created.
func (p *PerfStats) Log(prefix string) {
	log.Debugf("%s took %s", prefix, p.String())
	p.Reset()
}

// String provides a string representation of the usage thus far.
func (p *PerfStats) String() string {
	var m runtime.MemStats

	runtime.ReadMemStats(&m)
	alloc := (m.TotalAlloc - p.startMem) / 1024 / 1024 / 1024
	gcs := m.NumGC - p.startGc
	exectime := time.Since(p.startTime).Seconds()

	return fmt.Sprintf("%0.2fs using %v Gb (%v GC events) [%v Gb]", exectime, alloc, gcs, m.Alloc/1024/1024/1024)
}
