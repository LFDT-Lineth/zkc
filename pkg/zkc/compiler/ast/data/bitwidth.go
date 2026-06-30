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
package data

import (
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// BitWidthOf determines the bitwidth of the given type in the given
// environment.  NOTE: should a typing cycle exist involving the given type,
// then this will enter an infinite loop.
//
// NOTE: this function assumes that t1 and t2 are well-formed under the given
// environment.
func BitWidthOf[S symbol.Symbol[S]](t Type[S], env Environment[S]) (bitwidth util.Option[uint]) {
	switch t := t.(type) {
	case *UnsignedInt[S]:
		return util.Some(t.bitwidth)
	case *Alias[S]:
		return BitWidthOf(t.Resolve(env), env)
	case *FixedArray[S]:
		return BitWidthOf(t.Resolve(env), env)
	case *Tuple[S]:
		var bitwidth uint
		//
		for _, f := range t.elements {
			bw := BitWidthOf(f, env)
			//
			if !bw.HasValue() {
				return bw
			}
			//
			bitwidth += bw.Unwrap()
		}
		//
		return util.Some(bitwidth)
	default:
		return util.None[uint]()
	}
}
