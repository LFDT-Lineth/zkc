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
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/vanishing"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Validate the given schema by ensuring that every register in every module is referenced in at least one vanishing
// constraint.  If any such register is encountered, this should a suitable error which identifies the enclosing module
// and register.
func Validate[F field.Element[F]](schema sc.AnySchema[F]) error {
	var validated = make([]bit.Set, schema.Modules().Count())
	// Iterate each constraint, marking any registers it uses.
	for iter := schema.Constraints(); iter.HasNext(); {
		var c = iter.Next()
		validateConstraint(c, validated, schema)
	}
	// Validate every register was marked
	for iter, mid := schema.Modules(), 0; iter.HasNext(); mid++ {
		var mod = iter.Next()
		//
		for i, r := range mod.Registers() {
			if !validated[mid].Contains(uint(i)) {
				return fmt.Errorf("dangling register \"%s\" in module \"%s\"", r.Name(), mod.Name().String())
			}
		}
	}
	//
	return nil
}

func validateConstraint[F field.Element[F]](c sc.Constraint[F], validations []bit.Set, schema sc.AnySchema[F]) {
	switch c := c.(type) {
	case air.VanishingConstraint[F]:
		validateVanishingConstraint(c.Unwrap(), validations, schema)
	case mir.VanishingConstraint[F]:
		validateVanishingConstraint(c, validations, schema)
	}
}

func validateVanishingConstraint[F field.Element[F], T term.Testable[F]](c vanishing.Constraint[F, T],
	validations []bit.Set, schema sc.AnySchema[F]) {
	//
	var (
		mid = c.Context
		mod = schema.Module(mid)
	)
	//
	for _, rid := range *c.Constraint.RequiredRegisters() {
		var reg = mod.Register(register.NewId(rid))
		fmt.Printf("Marking register %s in module %s\n", reg.Name(), mod.Name().String())
		validations[mid].Insert(rid)
	}
}
