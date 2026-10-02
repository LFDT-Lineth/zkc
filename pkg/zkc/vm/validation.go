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
package vm

import (
	"errors"
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/validate"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

func validateBytecodeProgram[W word.Word[W]](program Program[W]) error {
	var (
		errs    []error
		modules = program.Modules()
	)
	// A module identifier is a uint16, so 2^16 distinct modules are addressable.
	if uint64(len(modules)) > uint64(math.MaxUint16)+1 {
		errs = append(errs, fmt.Errorf("program has too many modules (%d)", len(modules)))
	}
	// Simple validations
	errs = append(errs, validateModuleNames(program)...)
	errs = append(errs, validateModuleWidths(program)...)
	errs = append(errs, validateModuleZeroRegisters(program)...)
	// Validate all bytecode vectors
	errs = append(errs, validateFunctionBytecode(program)...)
	// Join all errors together
	return errors.Join(errs...)
}

// Ensure all module names are unique
func validateModuleNames[W word.Word[W]](program Program[W]) (errs []error) {
	var names = make(map[string]uint)
	//
	for mid, module := range program.Modules() {
		if previous, ok := names[module.Name()]; ok {
			errs = append(errs, fmt.Errorf("module %d (%s): duplicate name (first used by module %d)",
				mid, module.Name(), previous))
		} else {
			names[module.Name()] = uint(mid)
		}
	}
	//
	return errs
}

// Ensure no module is wider than permitted
func validateModuleWidths[W word.Word[W]](program Program[W]) (errs []error) {
	for mid, module := range program.Modules() {
		// Note: DISCARD cannot be a valid register identifier.
		if uint64(module.Width()) > uint64(math.MaxUint16) {
			errs = append(errs, fmt.Errorf("module %d (%s): too many registers (%d)",
				mid, module.Name(), module.Width()))
		}
	}
	//
	return errs
}

// Ensure no module has more than one zero register
func validateModuleZeroRegisters[W word.Word[W]](program Program[W]) (errs []error) {
	for mid, module := range program.Modules() {
		// At most one zero register should exist per module.
		var zeros []string

		for _, r := range module.Registers() {
			if r.IsZeroWidth() {
				zeros = append(zeros, r.Name())
			}
		}

		if len(zeros) > 1 {
			errs = append(errs, fmt.Errorf("module %d (%s): multiple zero registers (%v)",
				mid, module.Name(), zeros))
		}
	}
	//
	return errs
}

// Ensure function bytecode is well-formed
func validateFunctionBytecode[W word.Word[W]](program Program[W]) (errs []error) {
	for mid, module := range program.Modules() {
		if fn, ok := module.(*descriptor.Function[W]); ok && !fn.IsNative() {
			var env = program.EnvironmentOf(uint16(mid))
			// Validate the bytecode
			errs = append(errs, validate.Function(program.Field(), env, fn)...)
		}
	}
	//
	return errs
}
