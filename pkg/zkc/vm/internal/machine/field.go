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
package machine

import (
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
)

// Field --- see documentation on vm.FieldMachine.
type Field[F field.Element[F]] = Base[F, instruction.Field, FieldExecutor[F]]

// NewField constructs a new empty field machine.
func NewField[F field.Element[F]](modules ...Module) *Field[F] {
	return NewBase(FieldExecutor[F]{}, modules...)
}

// ==============================================================
// Field Executor
// ==============================================================

// FieldExecutor provides an executor implementation suitable for field
// arithmetic only.
type FieldExecutor[F field.Element[F]] struct {
}

// Execute implementation for Executor interface.
func (p FieldExecutor[F]) Execute(insn instruction.Field, frame StackFrame[F, instruction.Field]) (err error) {
	panic("got here")
}

// FieldExecutor carries no state, but it still needs an explicit gob
// implementation because gob refuses to encode a struct with no exported
// fields.

// nolint
func (p *FieldExecutor[F]) GobEncode() ([]byte, error) {
	return nil, nil
}

// nolint
func (p *FieldExecutor[F]) GobDecode(_ []byte) error {
	return nil
}
