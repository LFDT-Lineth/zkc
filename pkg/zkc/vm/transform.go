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
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// LowerBitwise rewrites VM-level bitwise bytecodes into CALLs to helper
// functions, returning the updated bytecode program (the helper function
// modules are appended to it).
//
// NOTE: this transform must be applied BEFORE vectorization and register
// splitting.
func LowerBitwise[W word.Word[W]](program Program[W]) Program[W] {
	return transform.LowerBitwise(program)
}

// LowerOrXorAnd rewrites bitwise AND/OR/XOR bytecodes into either a static-table
// lookup (a memory read) or a CALL to a recursive helper function, returning the
// updated bytecode program (the helper / table modules are appended to it).  An
// operation whose width is small enough that its 2^(2*width)-row truth table
// fits within maxStaticDepth is realised as a table read; wider operations are
// lowered recursively until their leaves are.
//
// NOTE: unlike LowerBitwise, this transform must be applied AFTER register
// splitting and BEFORE range-constraint generation.
func LowerOrXorAnd[W word.Word[W]](program Program[W], maxStaticDepth uint) Program[W] {
	return transform.LowerOrXorAnd(program, maxStaticDepth)
}

// LowerComparisons rewrites SkipIf bytecodes with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
func LowerComparisons[W word.Word[W]](program Program[W]) Program[W] {
	return transform.LowerComparisons(program)
}

// LowerFieldCasts inserts canonicality checks for extracting native field
// values into uint registers. Uint-to-field casts reduce modulo P and need no
// range check.
// It must run after register splitting; the checks it generates come out
// already comparison-lowered.
func LowerFieldCasts[W word.Word[W]](program Program[W]) Program[W] {
	return transform.LowerFieldCasts(program)
}

// LowerDivisions rewrites INT_DIV and INT_REM bytecodes into a non-deterministic
// hint followed by arithmetic validation:
//
//	Hint{DIV_HINT, q, r, w, x, y}   // prover fills quotient, remainder and range witness
//	qy = q * y
//	z0 = x - qy - r          // written into a 0-width register: asserts == 0
//	z1 = y - r - w - 1       // written into a 0-width register: asserts == 0
//
// This is a bytecode-level transform, so it runs before the program is
// decompiled into a word machine.
//
// NOTE: This pass must run before LowerComparisons.
func LowerDivisions[W word.Word[W]](program Program[W]) Program[W] {
	return transform.LowerDivisions(program)
}

// LowerSwitch rewrites Switch (multiway skip) bytecodes into equivalent
// sequences of SkipIf bytecodes.  Each dispatch case becomes a constant load
// of the case's value into a fresh register, followed by a conditional (EQ)
// skip against the dispatch register targeting the case's original
// destination.  Cases are tested in order, preserving the first-match-wins
// semantics of the multiway dispatch; when no case matches, control falls
// through exactly as before.
//
// NOTE: this transform must run before register splitting (which does not
// support Switch bytecodes).
func LowerSwitch[W word.Word[W]](program Program[W]) Program[W] {
	return transform.LowerSwitch(program)
}

// OptimizeDivisions is a fast-mode optimization which rewrites division by
// powers of 2 into right shifts, and remainder by powers of 2 into bitwise
// ANDs.
func OptimizeDivisions[W word.Word[W]](program Program[W]) Program[W] {
	return transform.OptimizeDivisions(program)
}

// Vectorize merges as many bytecodes as possible into each (vector / trace-line)
// bytecode, subject to register-conflict (data hazard) constraints.
func Vectorize[W word.Word[W]](program Program[W]) Program[W] {
	return transform.Vectorize(program)
}

// FlattenLookupAccess introduces a tmp register to hold a call (or memory access) argument
// when it's rewritten in the same vector:
// 1. x = f(x)
// 2. y = f(x); x = x + 1
// As we want to avoid shift in lookups, we must keep the original value of x in a tmp register,
// so that the call can be rewritten as:
// 1. tmp = x; x = f(tmp)
// 2. tmp = x; y = f(tmp); x = x + 1
func FlattenLookupAccess[W word.Word[W]](program Program[W]) Program[W] {
	return transform.FlattenLookupAccess(program)
}

// FactorSkipConditions rewrites equality SkipIf bytecodes (EQ/NEQ) so that the
// branch condition is materialised once into a fresh 1-bit register, rather
// than being replicated across each guarded write of the branch.
//
// NOTE: This transform must run after vectorisation and before register
// splitting.
func FactorSkipConditions[W word.Word[W]](program Program[W]) Program[W] {
	return transform.FactorSkipConditions(program)
}

// InlineFunctions returns an equivalent bytecode program in which every call to
// one of the named functions has been inlined at its call site, and the named
// function modules removed (module identifiers within Call / ReadWrite
// bytecodes are remapped accordingly).  Each call site is replaced by the
// callee's body operating on caller registers: inputs / outputs are aliased
// directly to the call's argument / return registers where provably equivalent;
// otherwise (e.g. for temporaries, or aliasing call sites such as "x = f(x)")
// fresh caller-local shadow registers are allocated, along with argument /
// return copies which preserve the dynamic width checks of a true call.
//
// NOTE: This transform must be applied before vectorisation, since it splits
// the vector enclosing a call at the call site.
func InlineFunctions[W word.Word[W]](program Program[W], names []string) Program[W] {
	return transform.InlineFunctions(program, names)
}

// SplitRegisters all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func SplitRegisters[W word.Word[W]](cfg WordConfig, program Program[W]) Program[W] {
	return transform.SplitRegisters(cfg, program)
}

// AddRangeConstraints adds a range-proof constraint for each register in the program.
// This is done by adding lookups from each (non-constant) register to a precomputed
// table of all valid values for that register width.
func AddRangeConstraints[W word.Word[W]](cfg field.Config, program Program[W], maxStaticDepth uint) Program[W] {
	return transform.AddRangeConstraints(cfg, program, maxStaticDepth)
}

// ProgramToProgram transforms a bytecode program operating over a given word
// type (W1) into an identical program which operates over a different word type
// (W2).  Generally speaking, we are going from a larger word (e.g. word.Uint) to
// a smaller word (e.g. word.Uint64).  This is the program-level analogue of
// WordToWordMachine.
//
// The transformation is purely structural: bytecodes are re-typed but not
// rewritten or lowered, register declarations are preserved verbatim (no
// splitting or width changes), and constants are not reduced modulo the field.
// Static memory contents are converted element-wise; non-static memories carry
// no contents in either representation.
//
// This function will panic if it encounters a register, constant or memory cell
// which exceeds the bandwidth of W2.  Callers needing to target a narrower word
// size than some source register widths should run SplitRegisters first.
func ProgramToProgram[W1 word.Word[W1], W2 word.Word[W2]](p Program[W1]) Program[W2] {
	return transform.ProgramToProgram[W1, W2](p)
}

// InsertCheckCasts inserts the width-check (CHECKCAST) bytecodes required by a
// bytecode program, returning the updated program.  Codegen emits operations
// without casts; this pass adds the cast checks each operation needs (resolving
// call / memory references against the program's module signatures) and rewrites
// branch offsets accordingly.  It must run on a complete program.
func InsertCheckCasts[W word.Word[W]](program Program[W]) Program[W] {
	return transform.InsertCheckCasts(program)
}
