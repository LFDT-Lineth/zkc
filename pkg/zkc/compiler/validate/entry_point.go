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
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
)

// EntryPoint checks that the program's entry point (the function named "main",
// if any) is well-formed.  The machine boots "main" with a fresh stack frame
// whose registers are all zero — there is no caller to supply arguments — so
// any declared parameter would silently read as zero in every execution mode
// (reference interpreter, bytecode and generated Go alike).  Program inputs
// must instead flow through declared input memories, which are initialised
// from the supplied inputs at boot time.  Hence, an entry point declaring
// parameters is almost certainly a mistake, and is rejected here.
func EntryPoint(program ast.Program, srcmaps source.Maps[any]) []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for _, d := range program.Components() {
		if fn, ok := d.(*decl.ResolvedFunction); ok && fn.Name() == "main" && fn.NumInputs > 0 {
			errors = append(errors,
				srcmaps.SyntaxErrors(fn, "entry point cannot declare parameters (use an input memory)")...)
		}
	}
	//
	return errors
}
