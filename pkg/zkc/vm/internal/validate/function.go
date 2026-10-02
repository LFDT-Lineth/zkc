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
package validate

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Environment provides a convenient alias
type Environment[W word.Word[W]] = bytecode.Environment[W]

// RegisterId provides a convenient alias
type RegisterId = bytecode.RegisterId

// Vector provides a covenient alias
type Vector[W word.Word[W]] = bytecode.Vector[W]

// Bytecode provides a covenient alias
type Bytecode[W word.Word[W]] = bytecode.Bytecode[W]

// ProgramPoint provides a convenient alias
type ProgramPoint = descriptor.ProgramPoint

// Word provides a convenient alias
type Word[W any] = word.Word[W]

// Function validates the body of a function against a given environment,
// returning errors if it is no well-formed.
func Function[W Word[W]](field field.Config, env Environment[W], f *descriptor.Function[W]) (errs []error) {
	var (
		n    = uint(len(f.Vectors()))
		safe = true
	)
	// Perform internal validations first
	for _, vec := range f.Vectors() {
		var es, s = validateVector(field, env, vec, n)
		//
		errs = append(errs, es...)
		safe = safe && s
	}
	// valididate register bitwidths (if it is safe to do so).
	if safe {
		errs = append(errs, validateRegisterBitwidth(f, env)...)
	}
	//
	return errs
}
