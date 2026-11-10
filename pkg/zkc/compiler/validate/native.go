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
package validate

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
)

// NativeFunctions checks that no function marked with the #[native]
// annotation calls a function (including itself).  A native function is
// backed by an external circuit which exposes only its interface (input /
// output registers): its body is never traced or constrained.  Hence, a call
// made from within that body would have no row to connect its call lookup to.
//
// Note: the linker already rejects such calls as it resolves them; this check
// guards the (flattened) program against any later transformation introducing
// them.
func NativeFunctions(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for i, d := range program.Components() {
		fn, ok := d.(*decl.ResolvedFunction)
		//
		if !ok || !slices.Contains(fn.Annotations(), "native") {
			continue
		}
		//
		for _, s := range fn.Code {
			for _, use := range externUsesOfStmt(s) {
				switch {
				case use.IsUnknown() || !use.IsFunction():
					continue
				case use.Index == uint(i):
					errors = append(errors, srcmaps.SyntaxErrors(s, "native function cannot be recursive")...)
				default:
					errors = append(errors, srcmaps.SyntaxErrors(s, "native function cannot call a function")...)
				}
			}
		}
	}
	//
	return errors
}
