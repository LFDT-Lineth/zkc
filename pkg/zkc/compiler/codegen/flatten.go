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
package codegen

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// flatten this type into a set of one or more registers, using a given
// prefix.  For example, a variable "x [u8;2]" is flattened into "x$0 u8"
// and "x$1 u8", etc.
//
// The constructor callback is invoked once per leaf register produced by the
// flattening.  Its arguments are:
//
//   - name: the fully-qualified register name, formed from the supplied prefix
//     plus a "$<index>" suffix for each tuple element traversed.
//   - bitwidth: the register's bitwidth.  For an UnsignedInt leaf this is the
//     declared bit-width (e.g. 8 for a u8).  For a FieldElement leaf this is
//     math.MaxUint, signalling that the register should be allocated as a
//     "native" register (i.e. one backed by a field element with no fixed
//     bit-width — see register.NewNative / register.IsNative).
//
// NOTE: should a typing cycle exist involving the given type, then this will
// enter an infinite loop.
func flatten[S symbol.Symbol[S]](t data.Type[S], prefix string, env data.Environment[S],
	constructor func(name string, bitwidth uint)) {
	//
	switch t := t.(type) {
	case *data.UnsignedInt[S]:
		constructor(prefix, t.BitWidth())
	case *data.Alias[S]:
		flatten(t.Resolve(env), prefix, env, constructor)
	case *data.Tuple[S]:
		for i := uint(0); i < t.Width(); i++ {
			ith := fmt.Sprintf("%s$%d", prefix, i)
			flatten(t.Ith(i), ith, env, constructor)
		}
	case *data.FieldElement[S]:
		constructor(prefix, math.MaxUint)
	default:
		//
		panic(fmt.Sprintf("unknown type encountered: %s", t.String(env)))
	}
}
