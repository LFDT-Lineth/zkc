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
package io

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
)

// Component defines a distinct entity within the system, such as a
// function, or a read-only memory or a static reference table.  Units contain
// registers, some of which may be marked as inputs/outputs and others as
// internal, etc.
type Component[T Instruction] interface {
	register.ConstMap
	schema.ModuleView

	// IsAtomic determines the lookup protocol for this component. Specifically,
	// for non-atomic units, we must filter the rows based on a selector when
	// performing lookups.
	IsAtomic() bool

	// Inputs returns the set of input registers for this component.  This exact
	// meaning of these depends upon the unit in question.  For example, inputs
	// correspond to parameters (for functions), address lines (for memories),
	// etc.
	Inputs() []Register

	// NumInputs returns the number of input registers for this component.
	NumInputs() uint

	// NumOutputs returns the number of output registers for this component.
	NumOutputs() uint

	// Outputs returns the set of output registers for this function.  This
	// exact meaning of these depends upon the unit in question.  For example,
	// outputs correspond to returns (for functions), data lines (for memories),
	// etc.
	Outputs() []Register

	// Validate that this component is well-formed.  For example, for a function
	// we must ensure that no instructions have conflicting writes, that all
	// temporaries have been allocated, etc.  The maximum bit capacity of the
	// underlying field is needed for various calculations as part of this.
	Validate(fieldWidth uint) []error
}
