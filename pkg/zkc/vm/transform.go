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
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	finsn "github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
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
// smaller word (e.g. word.Uint64).
//
// The transformation is purely structural: instructions are re-typed but not
// rewritten or lowered, register declarations are preserved verbatim (no
// splitting or width changes), and constants are not reduced modulo the field.
// The source machine's prime modulus is re-expressed in W2 so the new machine
// retains the same field semantics; this means the modulus itself must also
// fit in W2's bandwidth.  ROM/SROM contents are converted element-wise;
// WOM/RAM/Paged memories start empty in the new machine.
//
// This function will panic if it encounters a register, constant, modulus or
// memory cell which exceeds the bandwidth of W2.  Callers needing to target a
// narrower word size than some source register widths should run
// SplitRegisters first.
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

// WordToBytecodeInterpreter compiles a word machine into a bytecode sequence
// and, from this, constructs an interpreter.
func WordToBytecodeInterpreter[W word.Word[W]](wm *machine.Word[W]) *BytecodeInterpreter[W] {
	return transform.WordToBytecodeMachine(wm)
}

// WordToBytecodeProgram compiles a word machine into a bytecode sequence which
// can be executed by an interpreter.
func WordToBytecodeProgram[W word.Word[W]](wm *machine.Word[W]) BytecodeProgram[W] {
	return transform.WordToBytecodeProgram(wm)
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
