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
package schema

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
)

// Any converts a concrete schema into a generic view of the schema.
func Any[F any, C Constraint[F]](schema Schema[F, C]) AnySchema[F] {
	return schema.(Schema[F, Constraint[F]])
}

// AnySchema captures a generic view of a schema, which is useful in situations
// where exactly details about the schema are not important.
type AnySchema[F any] Schema[F, Constraint[F]]

// ============================================================================

// Schema provides a fundamental interface which attempts to capture the essence
// of an arithmetisation.  For simplicity, a schema consists entirely of one or
// more modules, where each module comprises some number of registers,
// constraints and assignments.  Registers can be loosely thought of as columns
// in the final trace, whilst constraints are properties which should hold for
// any acceptable trace.  Finally, assignments represent arbitrary computations
// which "assign" values to registers during "trace expansion".
type Schema[F any, C any] interface {
	// Assignments returns an iterator over the assignments of this schema.
	// That is, the set of computations used to determine values for all
	// computed columns.
	Assignments() iter.Iterator[Assignment[F]]
	// Consistent applies a number of internal consistency checks.  Whilst not
	// strictly necessary, these can highlight otherwise hidden problems as an aid
	// to debugging.
	Consistent(fieldWidth uint) []error
	// Constraints returns an iterator over all constraints defined in this
	// schema.  Observe that this does include assertions which, strictly
	// speaking, are not constraints in the true sense.  That is, they are never
	// compiled into vanishing polynomials but, instead, are used purely for
	// debugging.
	Constraints() iter.Iterator[C]
	// HasModule checks whether a module with the given name exists and, if so,
	// returns its module identifier.  Otherwise, it returns false.
	HasModule(name module.Name) (ModuleId, bool)
	// Access a given module in this schema.
	Module(module uint) Module[F]
	// Modules returns an iterator over the declared set of modules within this
	// schema.
	Modules() iter.Iterator[Module[F]]
	// Access a given register in this schema.
	Register(register.Ref) register.Register
	// Returns the number of modules in this schema.
	Width() uint
}

// Failure embodies structured information about a failing constraint.
// This includes the constraint itself, along with the row
type Failure interface {
	// Provides a suitable error message
	Message() string
}
