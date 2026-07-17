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
package bytecode

import (
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FieldCast converts between a native field register and a uint register vector.
type FieldCast[W word.Word[W]] struct {
	Target []RegisterId
	Source []RegisterId
}

// Uses returns the source registers.
func (p *FieldCast[W]) Uses() []RegisterId {
	return p.Source
}

// Definitions returns the target registers.
func (p *FieldCast[W]) Definitions() []RegisterId {
	return p.Target
}

// Validate implements Bytecode.
func (p *FieldCast[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *FieldCast[W]) String(env Environment[W]) string {
	var builder strings.Builder
	//
	builder.WriteString("cast ")
	builder.WriteString(RegistersToString(array.Reverse(p.Target), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(array.Reverse(p.Source), env, "::"))
	//
	return builder.String()
}
