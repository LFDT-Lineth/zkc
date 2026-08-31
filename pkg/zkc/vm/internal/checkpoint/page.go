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

import "slices"

// Page represents a variable-length chunk of data within a given memory.
type Page[W any] struct {
	// Address specifies the physical address in memory where this page begins.
	address uint64
	// Data holds the words stored in this page, beginning at address.
	data []W
	// timestamps, when non-nil, holds the per-cell timestamp parallel to data
	// (used for read/write memories whose cells are timestamped); nil otherwise.
	timestamps []uint64
}

// NewPage constructs a single page of memory beginning at the given physical
// address and holding the given data (with no per-cell timestamps).
func NewPage[W any](address uint64, data []W) Page[W] {
	return Page[W]{address: address, data: data}
}

// NewTimestampedPage constructs a page carrying a per-cell timestamp alongside
// each data word; len(timestamps) must equal len(data).
func NewTimestampedPage[W any](address uint64, data []W, timestamps []uint64) Page[W] {
	return Page[W]{address: address, data: data, timestamps: timestamps}
}

// Clone returns a deep copy of this page, with its own copy of the underlying
// data.  This is useful when the page references storage (e.g. live memory)
// which may subsequently be mutated.
func (p Page[W]) Clone() Page[W] {
	return Page[W]{p.address, slices.Clone(p.data), slices.Clone(p.timestamps)}
}

// Address returns the physical address in memory where this page begins.
func (p Page[W]) Address() uint64 {
	return p.address
}

// Data returns the words stored in this page, beginning at Address.
func (p Page[W]) Data() []W {
	return p.data
}

// Timestamps returns the per-cell timestamps parallel to Data, or nil if this
// page carries no timestamps.
func (p Page[W]) Timestamps() []uint64 {
	return p.timestamps
}
