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
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	finsn "github.com/consensys/go-corset/pkg/zkc/vm/instruction/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/lowering"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// Monomial is a useful alias
type Monomial = finsn.Monomial

// Polynomial is a useful alias
type Polynomial = finsn.Polynomial

// SystemMap is a useful alias
type SystemMap = instruction.SystemMap

// LowerWordMachine translates a machine over integer words into a machine over
// field elements.  In order to do this, it must "compile out" various
// high-level word operations (e.g. bitwise operations, division, etc) which
// have no direct correspondance within a field machine.
func LowerWordMachine[W word.Word[W], F field.Element[F]](cfg field.Config, wm *WordMachine[W]) (fm *FieldMachine[F]) {
	return lowering.LowerWordMachine[W, F](cfg, wm)
}

// LowerBitwise rewrites VM-level bitwise micro-instructions into CALLs to
// helper functions. The helper modules are appended to the returned module
// slice.
// We assume this lowering happens BEFORE vectorization and register splitting
func LowerBitwise[W Word[W]](modules []Module) []Module {
	return lowering.LowerBitwise[W](modules)
}

// LowerComparisons rewrites SkipIf instructions with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
// This pass must run after LowerBitwise.
func LowerComparisons[W word.Word[W]](modules []Module) []Module {
	return lowering.LowerComparisons[W](modules)
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
	return lowering.LowerDivisions[W](modules)
}
