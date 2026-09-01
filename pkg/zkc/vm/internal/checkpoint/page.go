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

// Page represents a variable-length chunk of data within a given memory.
type Page struct {
	// Address specifies the physical address in memory where this page begins.
	address uint64
	// Data holds the words stored in this page, beginning at address.
	data []byte
	// timestamps, when non-nil, holds the per-cell timestamp parallel to data
	// (used for read/write memories whose cells are timestamped); nil otherwise.
	timestamps []uint64
}

// NewPage constructs a single page of memory beginning at the given physical
// address and holding the given data (with no per-cell timestamps).
func NewPage(address uint64, data []byte) Page {
	return Page{address: address, data: data}
}

// NewStampedPage constructs a page carrying a per-cell timestamp alongside
// each data word; len(timestamps) must equal len(data).
func NewStampedPage(address uint64, data []byte, timestamps []uint64) Page {
	return Page{address: address, data: data, timestamps: timestamps}
}

// Bytes returns the raw data stored in this page
func (p Page) Bytes() []byte {
	return p.data
}

// Stamps returns the timestamp associated with each distinct cell.
func (p Page) Stamps() []uint64 {
	return p.timestamps
}

// Address returns the address of the first cell in this page.
func (p Page) Address() uint64 {
	return p.address
}
