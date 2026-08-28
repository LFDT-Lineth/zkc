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
// annotation can actually be placed "on the bus" so that it can be called from
// another shard.  Specifically, a global function must not be:
//
// (1) returning (i.e. anything other than "-> !"), since caller and callee may
// then reside in different shards and there is no way to thread the callee's
// results back to the caller;
//
// (2) marked #[native], since a native function is backed by an external
// circuit and, hence, has no activity ($ret) line to serve as the selector of
// the bus's receive port.
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
		//
		if slices.Contains(fn.Annotations(), "native") {
			errors = append(errors, srcmaps.SyntaxErrors(fn, "global function must not be native")...)
		}
	}
	//
	return errors
}
