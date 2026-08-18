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
	// Validate every module is reached, via lookups, from the entry point.
	errs = append(errs, validateModuleReachability(schema)...)
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
				err := fmt.Errorf("dangling register \"%s\" in module \"%s\"", r.Name(), mod.Name())
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

func validateLookupConstraint[F field.Element[F]](c lookup.Constraint[F], validations []bit.Set) {
	//
	validateLookupVectors(c.Targets, validations)
	validateLookupVectors(c.Sources, validations)
}

func validateLookupVectors(vs []lookup.Vector, validations []bit.Set) {
	for _, v := range vs {
		validateLookupVector(v, validations)
	}
}

func validateLookupVector(v lookup.Vector, validations []bit.Set) {
	for _, rid := range v.Registers {
		validations[v.Context()].Insert(rid.Unwrap())
	}
}

// validateModuleReachability checks that every module is reached by some chain
// of lookups originating in the entry point "main".
func validateModuleReachability[F field.Element[F]](schema sc.AnySchema[F]) (errs []error) {
	// TODO: https://github.com/LFDT-Lineth/zkc/issues/1869 parametrize "main" name
	for _, name := range UnreachableModules(schema) {
		errs = append(errs, fmt.Errorf("module \"%s\" unreachable via lookups from entry point \"main\"",
			name))
	}
	//
	return errs
}

// UnreachableModules returns the name of every module in the given schema which
// cannot be reached by any chain of lookups originating in the entry point
// "main".  A lookup reaches a module when one of its target vectors sits in
// that module; it emanates from the modules its source vectors sit in.  When
// the schema has no "main" module there is no entry point, and every module is
// considered reachable.
func UnreachableModules[F field.Element[F]](schema sc.AnySchema[F]) (unreachable []string) {
	var (
		reached  = make([]bool, schema.Modules().Count())
		outgoing = make(map[sc.ModuleId][]sc.ModuleId)
		worklist []sc.ModuleId
	)
	// Seed the traversal with the entry point; without one, nothing to check.
	// TODO: https://github.com/LFDT-Lineth/zkc/issues/1869 parametrize "main" name
	for iter, mid := schema.Modules(), sc.ModuleId(0); iter.HasNext(); mid++ {
		if iter.Next().Name() == "main" {
			reached[mid] = true
			worklist = append(worklist, mid)
		}
	}
	// no entry point found
	if len(worklist) == 0 {
		return nil
	}
	// Index every lookup by the modules it emanates from.
	for iter := schema.Constraints(); iter.HasNext(); {
		switch c := iter.Next().(type) {
		case air.LookupConstraint[F]:
			indexLookupEdges(c.Unwrap(), outgoing)
		case mir.LookupConstraint[F]:
			indexLookupEdges(c, outgoing)
		}
	}
	// Follow lookups from reached modules until a fixpoint is hit.
	for len(worklist) > 0 {
		mid := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		//
		for _, target := range outgoing[mid] {
			if !reached[target] {
				reached[target] = true
				worklist = append(worklist, target)
			}
		}
	}
	// Every module left unreached is unused, and reported.
	for iter, mid := schema.Modules(), 0; iter.HasNext(); mid++ {
		if mod := iter.Next(); !reached[mid] {
			unreachable = append(unreachable, mod.Name())
		}
	}
	//
	return unreachable
}

// indexLookupEdges records, for each source module of the given lookup, the
// target modules the lookup reaches.
func indexLookupEdges[F field.Element[F]](c lookup.Constraint[F], outgoing map[sc.ModuleId][]sc.ModuleId) {
	for _, src := range c.Sources {
		for _, tgt := range c.Targets {
			outgoing[src.Context()] = append(outgoing[src.Context()], tgt.Context())
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
				err := fmt.Errorf("height (%d) of static table \"%s\" not power-of-two", n, mod.Name())
				errors = append(errors, err)
			}
		}
	}

	return errors
}
