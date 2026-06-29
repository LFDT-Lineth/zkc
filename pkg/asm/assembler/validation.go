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
package assembler

import (
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// MicroComponent is a component whose instructions (if applicable) are
// themselves micro instructions.  A micro function represents the lowest
// representation of a function, where each instruction is made up of microcodes.
type MicroComponent = io.Component[micro.Instruction]

// ValidateMicro a micro program.  This is more challenging as we have no
// available source mapping information.  Instead, we just panic upon
// encountering an error.
func ValidateMicro(fieldWidth uint, functions []MicroComponent) {
	var srcmap source.Maps[any]
	//
	for _, fn := range functions {
		// TODO: support control-flow checks as well.
		validateMicroUnit[micro.Instruction](fieldWidth, fn, srcmap)
	}
}

func validateMicroUnit[T io.Instruction](fieldWidth uint, unit io.Component[T],
	srcmaps source.Maps[any]) []source.SyntaxError {
	//
	switch f := unit.(type) {
	case *io.Function[T]:
		return validateInstructions[T](fieldWidth, *f, srcmaps)
	default:
		panic("unknown component")
	}
}

// Check that each instruction in the function's body is correctly balanced.
// Amongst other things, this means ensuring the right number of bits are used
// on the left-hand side given the right-hand side.  For example, suppose "x :=
// y + 1" where both x and y are byte registers.  This does not balance because
// the right-hand side generates 9 bits but the left-hand side can only consume
// 8bits.
func validateInstructions[T io.Instruction](fieldWidth uint, fn io.Function[T],
	srcmaps source.Maps[any]) []source.SyntaxError {
	//
	var errors []source.SyntaxError

	for _, insn := range fn.Code() {
		err := insn.Validate(fieldWidth, &fn)
		//
		if err != nil {
			if !srcmaps.Has(insn) {
				panic(err)
			}
			//
			errors = append(errors, *srcmaps.SyntaxError(insn, err.Error()))
		}
	}
	//
	return errors
}
