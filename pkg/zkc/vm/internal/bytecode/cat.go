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
)

// Cat concatenates source register bits and stores the result across targets.
type Cat struct {
	// Targets receive the concatenated value, least-significant limb first.
	Targets []RegisterId
	// Sources are concatenated with Sources[0] in the least-significant bits.
	Sources []RegisterId
}

// Uses implementation for Bytecode interface.
func (p *Cat) Uses() []RegisterId {
	return p.Sources
}

// Definitions implementation for Bytecode interface.
func (p *Cat) Definitions() []RegisterId {
	return p.Targets
}

// Validate implementation for Bytecode interface.
func (p *Cat) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *Cat) String(env Environment) string {
	var builder strings.Builder
	//
	builder.WriteString("cat ")
	builder.WriteString(RegistersToString(array.Reverse(p.Targets), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(array.Reverse(p.Sources), env, "::"))
	//
	return builder.String()
}
