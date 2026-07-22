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
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// NewArith constructs a new arithmetic instruction computing
// "targets = sources[0] op sources[1] op ... op constant".
func NewArith[W word.Word[W]](op Operation, targets []RegisterId, sources []RegisterId, constant W) *Arith[W] {
	util.Assert(len(targets) > 0, "missing target register(s)")
	util.Assert(len(sources) > 0 || op == OP_ADD, "missing source register(s)")
	//
	return &Arith[W]{op, constant, sources, targets}
}

// Arith (arithmetic) instruction encodes a wide range of related arithmetic
// operations (e.g. +,-,*) including various bitwise operations.
type Arith[W word.Word[W]] struct {
	Op       Operation
	Constant W
	Source   []RegisterId
	Target   []RegisterId
}

// Uses implementation for Bytecode interface.
func (p *Arith[W]) Uses() []RegisterId {
	return p.Source
}

// Definitions implementation for Bytecode interface.
func (p *Arith[W]) Definitions() []RegisterId {
	return p.Target
}

// Validate implementation for Bytecode interface.
func (p *Arith[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	return validateOperands(env, p.Source, p.Target)
}

func (p *Arith[W]) String(env Environment[W]) string {
	var (
		builder strings.Builder
		cz      = IsUnusedConstant(p.Op, p.Constant)
		cstr    = fmt.Sprintf("0x%s", p.Constant.Text(16))
		prefix  string
	)
	//
	switch {
	case len(p.Source) == 0:
		prefix = "ldc"
	case len(p.Source) == 1 && cz:
		prefix = "mov"
	default:
		prefix = p.Op.Prefix()
	}
	//
	builder.WriteString(prefix)
	builder.WriteString(" ")
	builder.WriteString(RegistersToString(array.Reverse(p.Target), env, "::"))
	builder.WriteString(" = ")
	builder.WriteString(RegistersToString(p.Source, env, p.Op.Symbol()))
	//
	if len(p.Source) == 0 {
		builder.WriteString(cstr)
	} else if !cz {
		builder.WriteString(p.Op.Symbol())
		builder.WriteString(cstr)
	}
	//
	return builder.String()
}
