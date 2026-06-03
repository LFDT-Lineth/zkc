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
package mir

import (
	"github.com/LFDT-Lineth/zkc/pkg/ir/assignment"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// InitialiseConstantRegisters initialises any constant registers declared
// within the given set of MIR modules.  At this time, the only form of a
// constant register is the so-called "zero register" (i.e. the register which
// is always zero).  At the moment, these are implemented manually using
// constraints and assignments to fill them.  Eventually, this approach should
// be deprecated in favour of direct prover support.
func InitialiseConstantRegisters[F Element[F]](offset uint, modules []Module[F]) {
	// Consider each module in turn
	for m, mod := range modules {
		var mid = offset + uint(m)
		// Consider each register in turn
		for r, reg := range mod.Registers() {
			var rid = register.NewId(uint(r))
			if reg.IsConst() {
				initialiseConstantRegister(rid, mid, mod)
			}
		}
	}
}

// Lower a constant register (currently only zero is supported).
func initialiseConstantRegister[F field.Element[F]](rid register.Id, mid module.Id, module Module[F]) {
	var (
		reg = module.Register(rid)
		val = field.Uint64[word.BigEndian](uint64(reg.ConstValue()))
	)
	// Construct computation
	computation := term.NewComputation[word.BigEndian, LogicalTerm[word.BigEndian]](
		term.Const[word.BigEndian, Term[word.BigEndian]](val))
	// Add assignment for filling said computed column
	module.AddAssignments(
		assignment.NewComputedRegister[F](computation, true, mid, rid))
	// add constraint
	module.AddConstraints(
		NewVanishingConstraint(val.String(), mid, util.None[int](),
			term.Equals[F, LogicalTerm[F], Term[F]](
				term.NewRegisterAccess[F, Term[F]](rid, reg.Width(), 0),
				term.Const64[F, Term[F]](uint64(reg.ConstValue())))))
}
