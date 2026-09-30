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
package dfa

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
)

// Reaches identifies, on entry to a given micro-code, the set of micro-codes
// from which it is reachable.  That is, the offsets of those micro-codes lying
// on some path to it (excluding itself).  For example, consider the following
// sequence:
//
// skip_if ... 1; y = 0; ret; y = 1; ret
//
// Here, the final return is reachable from the skip_if and the assignment
// "y = 1", but not from the assignment "y = 0" (since that lies on a mutually
// exclusive path).
type Reaches struct {
	offsets bit.Set
}

// Visit constructs a reaches state representing this state after passing
// through the micro-code at the given offset.
func (p Reaches) Visit(offset uint) Reaches {
	var nst = Reaches{p.offsets.Clone()}
	//
	nst.offsets.Insert(offset)
	//
	return nst
}

// Join combines two reaches states together.
func (p Reaches) Join(q Reaches) Reaches {
	var nst = Reaches{p.offsets.Clone()}
	//
	nst.offsets.Union(q.offsets)
	//
	return nst
}

// ReachableFrom determines whether or not the micro-code at the given offset
// lies on some path to this point.
func (p Reaches) ReachableFrom(offset uint) bool {
	return p.offsets.Contains(offset)
}

// String implementation for State interface.  Observe that the register
// mapping is ignored, since this state holds micro-code offsets rather than
// registers.
func (p Reaches) String(_ func(RegisterId) string) string {
	return p.offsets.String()
}
