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
package vm

import (
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	finsn "github.com/consensys/go-corset/pkg/zkc/vm/instruction/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/transform"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Monomial is a useful alias
type Monomial = finsn.Monomial

// Polynomial is a useful alias
type Polynomial = finsn.Polynomial

// SystemMap is a useful alias
type SystemMap = instruction.SystemMap

// LowerBitwise rewrites VM-level bitwise micro-instructions into CALLs to
// helper functions. The helper modules are appended to the returned module
// slice.
// We assume this lowering happens BEFORE vectorization and register splitting
func LowerBitwise[W Word[W]](modules []Module) []Module {
	return transform.LowerBitwise[W](modules)
}

// LowerComparisons rewrites SkipIf instructions with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
// This pass must run after LowerBitwise.
func LowerComparisons[W word.Word[W]](modules []Module) []Module {
	return transform.LowerComparisons[W](modules)
}

// LowerDivisions rewrites INT_DIV and INT_REM instructions into a
// non-deterministic hint followed by arithmetic validation:
//
//	FieldHint{targets:[wideQ, wideR], sources:[x, y]}  // prover fills both at 2n bits
//	q = cast(wideQ, n) ; r = cast(wideR, n)           // write results to n-bit outputs
//	wideX, wideY = cast(x, 2n), cast(y, 2n)
//	sum = wideQ * wideY                                // exact 2n-bit product
//	sum = sum + wideR
//	SkipIf(EQ, sum, wideX, 1)
//	Fail
//	SkipIf(LT, r, y, 1)                        // expanded later by LowerComparisons
//	Fail
//
// This pass must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](modules []Module) []Module {
	return transform.LowerDivisions[W](modules)
}

// SplitRegisters all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func SplitRegisters[W Word[W]](cfg field.Config, wm *WordMachine[W]) *WordMachine[W] {
	// Construct a suitable limbs mapping
	var limbsMap = newLimbsMap(cfg, wm.Modules()...)
	// Invoke subdivision algorithm
	return transform.SplitRegisters(limbsMap, wm)
}

// WordToWordMachine transforms a machine operating over a given word type (W1)
// into an identical machine which operates over a different word type (W2).
// Generally speaking, we are going from a larger word (e.g. word.Uint) to a
// smaller word (e.g. word.Uint64).  This function will panic if it encounters a
// register or constant which exceeds the bandwidth of the given word.
func WordToWordMachine[W1 word.Word[W1], W2 word.Word[W2]](m1 *machine.Word[W1]) (m2 *machine.Word[W2]) {
	return transform.WordToWordMachine[W1, W2](m1)
}

// WordToFieldMachine translates a machine over integer words into a machine over
// field elements.  In order to do this, it must "compile out" various
// high-level word operations (e.g. bitwise operations, division, etc) which
// have no direct correspondance within a field machine.
func WordToFieldMachine[W word.Word[W], F field.Element[F]](cfg field.Config, wm *WordMachine[W],
) (fm *FieldMachine[F]) {
	return transform.WordToFieldMachine[W, F](cfg, wm)
}

func newLimbsMap(config field.Config, modules ...Module) module.LimbsMap {
	var ms []register.Map = array.Map(modules, func(_ uint, m Module) register.Map {
		name := trace.ModuleName{Name: m.Name(), Multiplier: 1}
		return register.ArrayMap(name, m.Registers()...)
	})
	// NOTE: generic parameter is meaningless, and only retained for backwards
	// compatibility.
	return module.NewLimbsMap[uint](config, ms...)
}
