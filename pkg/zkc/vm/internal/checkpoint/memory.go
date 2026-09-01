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
package checkpoint

// Memory captures a snapshot of the contents of a single (mutable) memory
// module.  To support sparse memories, the contents are described as a sequence
// of pages, each covering a contiguous region; regions not covered by any page
// are implicitly zero.
type Memory struct {
	// Module identifier for this memory.
	moduleId uint16
	// Pages determines the contents of the given memory in this snapshot.
	pages []Page
	// clock is the memory's access clock at snapshot time (the timestamp of its
	// most recent access); zero for memories which do not track timestamps.
	// Restored so that accesses after a resume stay monotonic.
	clock uint64
}

// NewMemory constructs a snapshot of a single memory module, identified by its
// module identifier, described by the given sequence of pages, and with the
// given access clock (zero for memories which do not track timestamps).
func NewMemory(moduleId uint16, clock uint64, pages ...Page) Memory {
	return Memory{moduleId, pages, clock}
}

// ModuleId returns the module identifier of the memory captured by this
// snapshot.
func (p Memory) ModuleId() uint16 {
	return p.moduleId
}

// Pages returns the pages describing the captured contents of this memory.
func (p Memory) Pages() []Page {
	return p.pages
}

// Clock returns the memory's access clock at snapshot time.
func (p Memory) Clock() uint64 {
	return p.clock
}
