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
package constraints

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Validate the given schema by ensuring that every register in every module is referenced in at least one vanishing
// constraint.  If any such register is encountered, this should a suitable error which identifies the enclosing module
// and register.
func Validate[F field.Element[F]](schema sc.AnySchema[F]) (errs []error) {
	// Validate constraints (e.g. no dangling registers)
	errs = validateConstraints(schema)
	// Validate static tables (e.g. padded correctly)
	errs = append(errs, validateStaticTables(schema)...)
	// Done
	return errs
}

func validateConstraints[F field.Element[F]](schema sc.AnySchema[F]) (errs []error) {
	var validated = make([]bit.Set, schema.Modules().Count())
	// Iterate each constraint, marking any registers it uses.
	for iter := schema.Constraints(); iter.HasNext(); {
		var c = iter.Next()
		validateConstraint(c, validated)
	}
	// Validate every register was marked
	for iter, mid := schema.Modules(), 0; iter.HasNext(); mid++ {
		var mod = iter.Next()
		// Static reference tables and native modules naturally have dangling
		// registers, and can be ignored.
		if mod.IsStatic() || mod.IsNative() {
			continue
		}
		// Check everything else.
		for i, r := range mod.Registers() {
			if !validated[mid].Contains(uint(i)) {
				err := fmt.Errorf("dangling register \"%s\" in module \"%s\"", r.Name(), mod.Name().String())
				//
				errs = append(errs, err)
			}
		}
	}
	//
	return errs
}

func validateConstraint[F field.Element[F]](c sc.Constraint[F], validations []bit.Set) {
	switch c := c.(type) {
	case air.VanishingConstraint[F]:
		validateVanishingConstraint(c.Unwrap(), validations)
	case mir.VanishingConstraint[F]:
		validateVanishingConstraint(c, validations)
	case air.LookupConstraint[F]:
		validateLookupConstraint(c.Unwrap(), validations)
	case mir.LookupConstraint[F]:
		validateLookupConstraint(c, validations)
	}
}

func validateVanishingConstraint[F field.Element[F], T term.Testable[F]](c vanishing.Constraint[F, T],
	validations []bit.Set) {
	//
	for _, rid := range *c.Constraint.RequiredRegisters() {
		validations[c.Context].Insert(rid)
	}
}

func validateLookupConstraint[F field.Element[F], T term.Evaluable[F]](c lookup.Constraint[F, T],
	validations []bit.Set) {
	//
	validateLookupVectors(c.Targets, validations)
	validateLookupVectors(c.Sources, validations)
}

func validateLookupVectors[F field.Element[F], T term.Evaluable[F]](vs []lookup.Vector[F, T],
	validations []bit.Set) {
	for _, v := range vs {
		validateLookupVector(v, validations)
	}
}

func validateLookupVector[F field.Element[F], T term.Evaluable[F]](v lookup.Vector[F, T],
	validations []bit.Set) {
	for _, e := range v.Terms {
		for _, rid := range *e.RequiredRegisters() {
			validations[v.Context()].Insert(rid)
		}
	}
}

// validateStaticTables validates that all static tables in the given schema have a power-of-two height.
func validateStaticTables[F field.Element[F]](schema sc.AnySchema[F]) []error {
	var errors []error

	for iter, mid := schema.Modules(), 0; iter.HasNext(); mid++ {
		var mod = iter.Next()
		if mod.IsStatic() {
			if n := uint(len(mod.StaticContents())); n == 0 || n&(n-1) != 0 {
				err := fmt.Errorf("height (%d) of static table \"%s\" not power-of-two", n, mod.Name().String())
				errors = append(errors, err)
			}
		}
	}

	return errors
}
