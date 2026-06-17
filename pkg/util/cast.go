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

import "fmt"

// Cast provides a safe cast from a value of one type to a value of another
// (typically smaller) type.  In particular, if the value does not fit in the
// given type then this will panic.
func Cast[S uint8 | uint16 | uint32, T uint8 | uint16 | uint32 | uint](val T) S {
	var bound = uint64(S(0) - 1)
	//
	if uint64(val) > bound {
		var bits = 0
		//
		for bound > 0 {
			bound = bound >> 1
			bits++
		}
		//
		panic(fmt.Sprintf("cast overflow (%d not u%d)", val, bits))
	}
	//
	return S(val)
}
