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

// GlobalFunctions checks that every function marked with the #[global]
// annotation is declared as non-returning (i.e. "-> !").  A global function is
// placed "on the bus" so that it can be called from another shard.  Since
// caller and callee may then reside in different shards, there is no way to
// thread the callee's results back to the caller.  Hence, a global function
// which returns is rejected here.
func GlobalFunctions(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, d := range program.Components() {
		fn, ok := d.(*decl.ResolvedFunction)
		//
		if !ok || !slices.Contains(fn.Annotations(), "global") {
			continue
		}
		//
		if !fn.NoReturn {
			errors = append(errors, srcmaps.SyntaxErrors(fn, "global function must not return")...)
		}
	}
	//
	return errors
}
