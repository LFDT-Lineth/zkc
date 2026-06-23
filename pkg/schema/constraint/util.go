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
package constraint

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
)

// CheckConsistent performs a simple consistency check for terms in a given
// module.  Specifically, to check that: (1) the module exists; (2) all used
// registers existing with then given module.
func CheckConsistent[F any, E term.Contextual](module uint, schema schema.AnySchema[F], terms ...E) []error {
	var errs []error
	// Sanity check module
	if module >= schema.Width() {
		errs = append(errs, fmt.Errorf("invalid module (%d >= %d)", module, schema.Width()))
	} else {
		for _, term := range terms {
			var (
				regs = term.RequiredRegisters()
				mod  = schema.Module(module)
			)
			// Sanity check referenced registers
			for iter := regs.Iter(); iter.HasNext(); {
				reg := iter.Next()
				if reg >= mod.Width() {
					errs = append(errs, fmt.Errorf("invalid register access in %s (%d >= %d)", mod.Name(), reg, mod.Width()))
				}
			}
		}
	}
	//
	return errs
}

// DetermineHandle is a very simple helper which determines a suitable qualified
// name for the given constraint handle.
func DetermineHandle[F any](handle string, ctx schema.ModuleId, tr trace.Trace[F]) string {
	modName := tr.Module(ctx).Name()
	//
	return trace.QualifiedColumnName(modName, handle)
}
