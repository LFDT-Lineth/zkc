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
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// BitWidthOf determines the bitwidth of the given type in the given
// environment.  NOTE: should a typing cycle exist involving the given type,
// then this will enter an infinite loop.
//
// NOTE: this function assumes that t1 and t2 are well-formed under the given
// environment.
func BitWidthOf[S symbol.Symbol[S]](t Type[S], env Environment[S]) (bitwidth uint, ok bool) {
	switch t := t.(type) {
	case *UnsignedInt[S]:
		return t.bitwidth, true
	case *Alias[S]:
		return BitWidthOf(t.Resolve(env), env)
	case *FixedArray[S]:
		return BitWidthOf(t.Resolve(env), env)
	case *Tuple[S]:
		var bitwidth uint
		//
		for _, f := range t.elements {
			bw, ok := BitWidthOf(f, env)
			//
			if !ok {
				return math.MaxUint, false
			}
			//
			bitwidth += bw
		}
		//
		return bitwidth, true
	case *FieldElement[S]:
		panic(fmt.Sprintf("field element type has no fixed bitwidth: %s", t.String(env)))
	}
	//
	return math.MaxUint, false
}
