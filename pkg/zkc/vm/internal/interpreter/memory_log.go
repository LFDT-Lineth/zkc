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
package interpreter

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Log records the reads and writes performed against a read/write memory,
// in chronological order.  It is an interface so the recording cost is paid only
// when needed: fast / checkpoint mode installs a no-op log (CheckpointingMemoryLog),
// whilst trace generation installs a recording log (TraceableMemoryLog).
//
// Only the write-side of each access is recorded: the address, and the value and
// timestamp the cell holds *after* the access (VALUE_WRITTEN / TIMESTAMP_WRITTEN
// in ram.md).  The write-side is the only genuinely new information -- on a write
// the value is program data that exists nowhere else, and the cell may never be
// touched again.  The read-side (VALUE_READ / TIMESTAMP_READ) is reconstructed at
// trace time by an observer that replays the log forward from the restored
// initial cells, tracking each cell's current (value, timestamp): the read-side
// of any access is simply that tracked state immediately before the access (i.e.
// the previous access's write-side at that address, or the initial 0).
type Log[W word.Word[W]] interface {
	// Read records a read of the cell at address, which afterwards holds
	// valueWritten (unchanged, equal to the value read) at timestampWritten.
	Read(address uint64, valueWritten W, timestampWritten uint64)
	// Write records a write to the cell at address, which afterwards holds
	// valueWritten at timestampWritten.
	Write(address uint64, valueWritten W, timestampWritten uint64)
	// Reset clears the log so recording starts fresh (called by Initialise).
	Reset()
	// Accesses returns the recorded accesses in chronological order; nil for a
	// non-recording log.
	Accesses() []AccessData[W]
	// Records reports whether this log retains accesses.  A memory caches this
	// when the log is installed, so the hot read/write path can skip the log
	// call without paying for an interface dispatch.
	Records() bool
}

// AccessData records a single memory access: the address, the value and
// timestamp the cell holds AFTER the access, and whether the access was
// a write.
type AccessData[W word.Word[W]] struct {
	address   uint64
	value     W
	timestamp uint64
	isWrite   bool
}

// Address returns the address touched by this access.
func (a AccessData[W]) Address() uint64 { return a.address }

// ValueWritten returns the value the cell holds after this access.
func (a AccessData[W]) ValueWritten() W { return a.value }

// TimestampWritten returns the timestamp the cell holds after this access.
func (a AccessData[W]) TimestampWritten() uint64 { return a.timestamp }

// IsWrite reports whether this access was a write (true) or a read (false).
func (a AccessData[W]) IsWrite() bool { return a.isWrite }

// CheckpointingMemoryLog is the no-op Log used in fast / checkpoint mode:
// it records nothing.  Checkpointing captures the timestamped cells directly
// (see RandomAccess.Cells), so no access log is required.
type CheckpointingMemoryLog[W word.Word[W]] struct{}

// Read implementation for Log (no-op).
func (l *CheckpointingMemoryLog[W]) Read(address uint64, valueWritten W, timestampWritten uint64) {}

// Write implementation for Log (no-op).
func (l *CheckpointingMemoryLog[W]) Write(address uint64, valueWritten W, timestampWritten uint64) {}

// Reset implementation for Log (no-op).
func (l *CheckpointingMemoryLog[W]) Reset() {}

// Accesses implementation for Log (always nil).
func (l *CheckpointingMemoryLog[W]) Accesses() []AccessData[W] { return nil }

// Records implementation for Log (never records).
func (l *CheckpointingMemoryLog[W]) Records() bool { return false }

// TraceableMemoryLog is the recording Log used during trace generation: it
// retains every access in chronological order for the trace observer.
type TraceableMemoryLog[W word.Word[W]] struct {
	accesses []AccessData[W]
}

// Read implementation for Log.
// valueWritten will coincide with the read value.
func (l *TraceableMemoryLog[W]) Read(address uint64, valueWritten W, timestampWritten uint64) {
	l.accesses = append(l.accesses, AccessData[W]{address, valueWritten, timestampWritten, false})
}

// Write implementation for Log.
func (l *TraceableMemoryLog[W]) Write(address uint64, valueWritten W, timestampWritten uint64) {
	l.accesses = append(l.accesses, AccessData[W]{address, valueWritten, timestampWritten, true})
}

// Reset implementation for Log.
func (l *TraceableMemoryLog[W]) Reset() { l.accesses = nil }

// Accesses implementation for Log.
func (l *TraceableMemoryLog[W]) Accesses() []AccessData[W] { return l.accesses }

// Records implementation for Log (always records).
func (l *TraceableMemoryLog[W]) Records() bool { return true }
