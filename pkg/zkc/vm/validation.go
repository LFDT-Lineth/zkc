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
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

func validateBytecodeProgram[W word.Word[W]](program Program[W]) error {
	var (
		errs    []error
		modules = program.Modules()
		names   = make(map[string]uint)
	)
	// A module identifier is a uint16, so 2^16 distinct modules are addressable.
	if uint64(len(modules)) > uint64(math.MaxUint16)+1 {
		errs = append(errs, fmt.Errorf("program has too many modules (%d)", len(modules)))
	}
	// Validate module-level invariants before constructing any environments.
	for mid, module := range modules {
		if isNilInterface(module) {
			errs = append(errs, fmt.Errorf("module %d is nil", mid))
			continue
		}

		if previous, ok := names[module.Name()]; ok {
			errs = append(errs, fmt.Errorf("module %d (%s): duplicate name (first used by module %d)",
				mid, module.Name(), previous))
		} else {
			names[module.Name()] = uint(mid)
		}

		if uint64(module.Width()) > uint64(math.MaxUint16)+1 {
			errs = append(errs, fmt.Errorf("module %d (%s): too many registers (%d)",
				mid, module.Name(), module.Width()))
		}
	}
	// An environment stores its enclosing module as a uint16.  If the module
	// count is already invalid, do not truncate module indices while validating.
	if uint64(len(modules)) > uint64(math.MaxUint16)+1 {
		return errors.Join(errs...)
	}

	for mid, module := range modules {
		if isNilInterface(module) {
			continue
		}

		fn, ok := module.(*descriptor.Function[W])
		if !ok || fn.IsNative() {
			continue
		}

		env := program.EnvironmentOf(uint16(mid))

		for pc, vector := range fn.Vectors() {
			location := fmt.Sprintf("function %s, vector %d", fn.Name(), pc)
			for _, err := range vector.Validate(program.Field(), env) {
				errs = append(errs, fmt.Errorf("%s: %w", location, err))
			}
		}
	}

	return errors.Join(errs...)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)

	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
