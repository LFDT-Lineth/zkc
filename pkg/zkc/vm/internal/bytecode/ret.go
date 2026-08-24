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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Ret (return from function call) instruction.
type Ret[W word.Word[W]] struct {
	// Done indicates whether or not this is, in fact, a "done" terminator.
	Done bool
}

// Uses implementation for Bytecode interface.  The copying of return values is
// handled by the frame machinery rather than by named register operands, so a
// return reads no registers here.
func (p *Ret[W]) Uses() []RegisterId {
	return nil
}

// Definitions implementation for Bytecode interface.
func (p *Ret[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *Ret[W]) Validate(_ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *Ret[W]) String(_ Environment[W]) string {
	if p.Done {
		return "done"
	}
	//
	return "ret"
}
