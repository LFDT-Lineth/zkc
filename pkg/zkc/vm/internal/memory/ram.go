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
package memory

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// RandomAccess is a memory implementation backed by a dynamically sizing []W,
// meaning that an out-of-bound read will return 0.  Reads are performed by
// delegating address decoding to a D (an AddressDecoder) which translates the
// incoming multi-word address tuple into a (start, end) index range, and then
// returning the corresponding sub-slice of the backing data.
//
// The type parameter W is the word type (e.g. a field element or big.Int), and
// D is the AddressDecoder strategy that encodes the layout of rows within the
// flat slice.
type RandomAccess[W util.Uinter64] struct {
	StaticArray[W]
}

// Read function handles out-of-bounds accesses.
func (p *RandomAccess[W]) Read(address uint64) (W, error) {
	var val W
	//
	if address < uint64(len(p.data)) {
		val = p.data[address]
	}
	//
	return val, nil
}

// NewRandomAccess constructs an empty random-access memory which employs a
// non-sparse implementation.  Thus, this is not suitable for very large
// memories.
func NewRandomAccess[W util.Uinter64](name string, registers []register.Register) Memory[W] {
	return &RandomAccess[W]{
		StaticArray: NewStaticArray[W](name, RANDOM_ACCESS_MEMORY, registers),
	}
}
