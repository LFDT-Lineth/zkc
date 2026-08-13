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
	"fmt"
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/transform"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// TransformForExecution applies a transformation pipeline suitable for (fast
// mode) execution, whilst ignoring any pipeline stages requested (i.e. for
// debugging).  The target word W2 determines the size at which to split
// registers.
func TransformForExecution[W1 Word[W1], W2 Word[W2]](p Program[W1], ignores ...string) Program[W2] {
	// Use target word to determine config
	var targetWord W2
	// Done
	return TransformForExecutionRaw[W1, W2](p, targetWord.Config(), ignores...)
}

// TransformForExecutionRaw applies a transformation pipeline suitable for (fast
// mode) execution, whilst ignoring any pipeline stages requested (i.e. for
// debugging).  Here, the target word configuration determines the register
// width to split against, which is distinct from the word type W2 the program
// is finally concretized into.  Observe that W2 must be large enough to hold
// all values of the target word, otherwise this will panic.
func TransformForExecutionRaw[W1 Word[W1], W2 Word[W2]](p Program[W1], word WordConfig,
	ignores ...string) Program[W2] {
	//
	var (
		w2 W2
		//
		pipeline = NewTransformationPipeline("(fast mode) execution",
			InlineFunctions[W1](),
			Vectorize[W1](),
			SplitRegisters[W1](word),
			InsertCheckCasts[W1](),
		)
	)
	// Sanity check
	if word.BandWidth > w2.Bandwidth() {
		panic(fmt.Sprintf("%s does not fit within u%d", word.Name, w2.Bandwidth()))
	}
	// ignore requested stages
	pipeline = pipeline.Ignore(ignores...)
	// Apply pipeline transformations
	program := pipeline.Apply(p)
	// Concretize the program
	return transform.ProgramToProgram[W1, W2](program)
}

// TransformForTracing applies a transformation pipeline suitable for tracing,
// whilst ignoring any pipeline stages requested (i.e. for debugging).
func TransformForTracing[W1 Word[W1], W2 Word[W2]](p Program[W1], ignores ...string) Program[W2] {
	var (
		pipeline = NewTransformationPipeline("tracing",
			InlineFunctions[W1](),
			LowerFieldCasts[W1](),
			LowerBitwise[W1](),
			LowerDivisions[W1](),
			LowerComparisons[W1](),
			Vectorize[W1](),
			ThreadTimestamps[W1](),
			FactorSkipConditions[W1](),
			LowerSwitch[W1](),
			SplitRegisters[W1](p.Field()),
			FactorLimbEqualities[W1](),
			LowerOrXorAnd[W1](),
			FlattenLookupAccess[W1](),
			AddRangeConstraints[W1](),
			InsertCheckCasts[W1](),
		)
	)
	// ignore requested stages
	pipeline = pipeline.Ignore(ignores...)
	// Apply pipeline transformations
	program := pipeline.Apply(p)
	// Concretize the program
	return transform.ProgramToProgram[W1, W2](program)
}

// Transform represents a single transformation in a transformation pipeline.
// Every transform is uniquely identified by its name, and can specify other
// transforms which must (or must not) run before it.
type Transform[W Word[W]] struct {
	// name of this transform
	name string
	// Precondition which must hold for this transformation to be valid.
	precondition func(pipeline, transform string, i, n int, seen map[string]bool)
	// Transforms is the function which actually implements the transformation.
	transformer func(Program[W]) Program[W]
}

// TransformationPipeline represents single pipeline of transformations which
// are applied in sequence to transform a program in one form into an equivalent
// program in another form.  Example transformations would include register
// splitting, register allocation, lowering divisions, etc.
type TransformationPipeline[W Word[W]] struct {
	name string
	// config encapsulates necessary information for transforms
	//config transform.Config
	// transforms provides the list of transforms in the order they should be
	// applied.
	transforms []Transform[W]
}

// NewTransformationPipeline validates and constructs a new pipeline from a
// given set of transforms.  This will panic if the dependency requirements are
// not met.
func NewTransformationPipeline[W Word[W]](name string, transforms ...Transform[W],
) TransformationPipeline[W] {
	// See holds those transforms seen in the pipeline so far.
	var seen = make(map[string]bool)
	//
	for i, t := range transforms {
		// Check precondition
		t.precondition(name, t.name, i, len(transforms), seen)
		// Mark transform as visited
		seen[t.name] = true
	}
	// Done
	return TransformationPipeline[W]{name, transforms}
}

// Ignore any requested pipeline stages
func (p TransformationPipeline[W]) Ignore(ignores ...string) TransformationPipeline[W] {
	for _, ignore := range ignores {
		var (
			pred = func(t Transform[W]) bool {
				return t.name == ignore
			}
		)
		//
		if index := slices.IndexFunc(p.transforms, pred); index >= 0 {
			p.transforms = array.RemoveAt(p.transforms, uint(index))
		}
	}
	//
	return p
}

// Apply this transformation pipeline to a given program, producing a
// transformed (but otherwise equivalent) program.
func (p TransformationPipeline[W]) Apply(program Program[W]) Program[W] {
	// Apply each transformation in turn.
	for _, t := range p.transforms {
		program = t.transformer(program)
	}
	// Validate program to catch any introduced corruption as early as possible.
	if err := ValidateProgram(program); err != nil {
		panic(err)
	}
	//
	return program
}

const (
	// ADD_RANGE_CONSTRAINTS handle
	ADD_RANGE_CONSTRAINTS = "add-range-constraints"
	// FACTOR_LIMB_EQUALITIES handle
	FACTOR_LIMB_EQUALITIES = "factor-limb-equalities"
	// FACTOR_SKIP_CONDITIONS handle
	FACTOR_SKIP_CONDITIONS = "factor-skip-conditions"
	//FLATTERN_LOOKUP_ACCESSES handle
	FLATTERN_LOOKUP_ACCESSES = "flattern-lookup-accesses"
	// INLINE_FUNCTIONS handle
	INLINE_FUNCTIONS = "inline-functions"
	// INSERT_CHECKCASTS handle
	INSERT_CHECKCASTS = "insert-checkcasts"
	// LOWER_BITWISE handle
	LOWER_BITWISE = "lower-bitwise"
	// LOWER_COMPARISONS handle
	LOWER_COMPARISONS = "lower-comparisons"
	// LOWER_DIVISIONS handle
	LOWER_DIVISIONS = "lower-divisions"
	// LOWER_FIELDCASTS handle
	LOWER_FIELDCASTS = "lower-fieldcasts"
	// LOWER_ORXORAND handle
	LOWER_ORXORAND = "lower-orxorand"
	// LOWER_SWITCH handle
	LOWER_SWITCH = "lower-switch"
	// SPLIT_REGISTERS handle
	SPLIT_REGISTERS = "split-registers"
	// THREAD_TIMESTAMPS handle
	THREAD_TIMESTAMPS = "thread-timestamps"
	// VECTORIZE handle
	VECTORIZE = "vectorize"
)

// VALID_TRANSFORMS contains the complete set of valid transform handles.
var VALID_TRANSFORMS = []string{
	ADD_RANGE_CONSTRAINTS,
	FACTOR_LIMB_EQUALITIES,
	FACTOR_SKIP_CONDITIONS,
	FLATTERN_LOOKUP_ACCESSES,
	INLINE_FUNCTIONS,
	INSERT_CHECKCASTS,
	LOWER_BITWISE,
	LOWER_COMPARISONS,
	LOWER_DIVISIONS,
	LOWER_FIELDCASTS,
	LOWER_ORXORAND,
	LOWER_SWITCH,
	SPLIT_REGISTERS,
	THREAD_TIMESTAMPS,
	VECTORIZE,
}

// LowerBitwise rewrites VM-level bitwise bytecodes into CALLs to helper
// functions, returning the updated bytecode program (the helper function
// modules are appended to it).
//
// NOTE: this transform must be applied BEFORE vectorization and register
// splitting.
func LowerBitwise[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.LowerBitwise[W]
		precondition = before(VECTORIZE, SPLIT_REGISTERS)
	)
	//
	return Transform[W]{LOWER_BITWISE, precondition, transformer}
}

// LowerOrXorAnd rewrites bitwise AND/OR/XOR bytecodes into either a static-table
// lookup (a memory read) or a CALL to a recursive helper function, returning the
// updated bytecode program (the helper / table modules are appended to it).  An
// operation whose width is small enough that its 2^(2*width)-row truth table
// fits within maxStaticHeight is realised as a table read; wider operations are
// lowered recursively until their leaves are.
//
// NOTE: unlike LowerBitwise, this transform must be applied AFTER register
// splitting and BEFORE range-constraint generation.
func LowerOrXorAnd[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.LowerOrXorAnd[W]
		precondition = and(after(SPLIT_REGISTERS), before("add-range-constraints"))
	)
	//
	return Transform[W]{LOWER_ORXORAND, precondition, transformer}
}

// LowerComparisons rewrites SkipIf bytecodes with LT/GT/LTEQ/GTEQ conditions
// into arithmetic-only sequences using biased subtraction and sign-bit extraction.
// EQ and NEQ conditions are left unchanged.
func LowerComparisons[W word.Word[W]]() Transform[W] {
	var transformer = transform.LowerComparisons[W]
	//
	return Transform[W]{LOWER_COMPARISONS, noPrecondition, transformer}
}

// LowerFieldCasts (𝔽↔uint) first: the canonicality check for 𝔽→uint is
// emitted as a high-level "value < P" comparison, which the comparison and
// register-splitting passes below then turn into a subtract-with- borrow chain.
// It must therefore run before LowerComparisons and SplitRegisters.
func LowerFieldCasts[W word.Word[W]]() Transform[W] {
	var (
		transformer = transform.LowerFieldCasts[W]
	)
	//
	return Transform[W]{LOWER_FIELDCASTS, noPrecondition, transformer}
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
func LowerDivisions[W word.Word[W]]() Transform[W] {
	var (
		transformer = transform.LowerDivisions[W]
	)
	//
	return Transform[W]{LOWER_DIVISIONS, noPrecondition, transformer}
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
func LowerSwitch[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.LowerSwitch[W]
		precondition = before(SPLIT_REGISTERS)
	)
	//
	return Transform[W]{LOWER_SWITCH, precondition, transformer}
}

// ThreadTimestamps threads a per-memory timestamp through every function which
// declares a read-write memory effect: each such function gains a stamp-in
// input and stamp-out output per accessed read-write memory, calls forward
// them, and every memory access carries a distinct timestamp in its Stamp
// operand (the k-th access executed carries stamp-in + k).  Applied on the
// constraint path only (the run-time memory maintains its own clock), after
// vectorisation — so a vector is genuinely one trace row and the canonical
// stamp register is written at most once per executed path through it — and
// before register splitting.
func ThreadTimestamps[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.ThreadTimestamps[W]
		precondition = before(SPLIT_REGISTERS)
	)
	//
	return Transform[W]{THREAD_TIMESTAMPS, precondition, transformer}
}

// Vectorize merges as many bytecodes as possible into each (vector / trace-line)
// bytecode, subject to register-conflict (data hazard) constraints.
func Vectorize[W word.Word[W]]() Transform[W] {
	var (
		transformer = transform.Vectorize[W]
	)
	//
	return Transform[W]{VECTORIZE, noPrecondition, transformer}
}

// FlattenLookupAccess introduces a tmp register to hold a call (or memory access) argument
// when it's rewritten in the same vector:
// 1. x = f(x)
// 2. y = f(x); x = x + 1
// As we want to avoid shift in lookups, we must keep the original value of x in a tmp register,
// so that the call can be rewritten as:
// 1. tmp = x; x = f(tmp)
// 2. tmp = x; y = f(tmp); x = x + 1
func FlattenLookupAccess[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.FlattenLookupAccess[W]
		precondition = after(SPLIT_REGISTERS)
	)
	//
	return Transform[W]{FLATTERN_LOOKUP_ACCESSES, precondition, transformer}
}

// FactorSkipConditions rewrites equality SkipIf bytecodes (EQ/NEQ) so that the
// branch condition is materialised once into a fresh 1-bit register, rather
// than being replicated across each guarded write of the branch.
//
// NOTE: This transform must run after vectorisation and before register
// splitting.
func FactorSkipConditions[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.FactorSkipConditions[W]
		precondition = and(after(VECTORIZE), before(SPLIT_REGISTERS))
	)
	//
	return Transform[W]{FACTOR_SKIP_CONDITIONS, precondition, transformer}
}

// FactorLimbEqualities rewrites equality SkipIf bytecodes (EQ/NEQ) comparing
// two multi-limb register vectors, materialising each limb inequality into its
// own fresh 1-bit register and testing the resulting bit vector against zero
// instead.  This bounds the degree of the comparison independently of the limb
// count (the bits being sign-definite, unlike the original limb differences).
//
// NOTE: This transform must run after register splitting and before range
// constraints are added.
func FactorLimbEqualities[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.FactorLimbEqualities[W]
		precondition = and(after(SPLIT_REGISTERS), before("add-range-constraints"))
	)
	//
	return Transform[W]{FACTOR_LIMB_EQUALITIES, precondition, transformer}
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
func InlineFunctions[W word.Word[W]]() Transform[W] {
	var (
		transformer  = transform.InlineFunctions[W]
		precondition = before(VECTORIZE)
	)
	//
	return Transform[W]{INLINE_FUNCTIONS, precondition, transformer}
}

// SplitRegisters all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func SplitRegisters[W word.Word[W]](field field.Config) Transform[W] {
	var (
		transformer = func(p Program[W]) Program[W] {
			return transform.SplitRegisters[W](field, p)
		}
	)
	//
	return Transform[W]{SPLIT_REGISTERS, noPrecondition, transformer}
}

// AddRangeConstraints adds a range-proof constraint for each register in the program.
// This is done by adding lookups from each (non-constant) register to a precomputed
// table of all valid values for that register width.
func AddRangeConstraints[W word.Word[W]]() Transform[W] {
	var (
		transformer = transform.AddRangeConstraints[W]
	)
	//
	return Transform[W]{ADD_RANGE_CONSTRAINTS, noPrecondition, transformer}
}

// InsertCheckCasts inserts the width-check (CHECKCAST) bytecodes required by a
// bytecode program, returning the updated program.  Codegen emits operations
// without casts; this pass adds the cast checks each operation needs (resolving
// call / memory references against the program's module signatures) and rewrites
// branch offsets accordingly.  It must run on a complete program.
func InsertCheckCasts[W word.Word[W]]() Transform[W] {
	var (
		transformer = transform.InsertCheckCasts[W]
	)
	//
	return Transform[W]{INSERT_CHECKCASTS, last, transformer}
}

func after(deps ...string) func(string, string, int, int, map[string]bool) {
	return func(pipeline, name string, _ int, _ int, seen map[string]bool) {
		for _, dep := range deps {
			if _, ok := seen[dep]; !ok {
				panic(
					fmt.Sprintf("transformation \"%s\" must run after \"%s\" in pipeline \"%s\"", name, dep, pipeline))
			}
		}
	}
}

func before(deps ...string) func(string, string, int, int, map[string]bool) {
	return func(pipeline, name string, _ int, _ int, seen map[string]bool) {
		for _, dep := range deps {
			if _, ok := seen[dep]; ok {
				panic(
					fmt.Sprintf("transformation \"%s\" must run before \"%s\" in pipeline \"%s\"", name, dep, pipeline))
			}
		}
	}
}

func and(conds ...func(string, string, int, int, map[string]bool)) func(string, string, int, int, map[string]bool) {
	return func(pipeline, name string, i int, n int, seen map[string]bool) {
		for _, c := range conds {
			c(pipeline, name, i, n, seen)
		}
	}
}

func noPrecondition(_, _ string, _, _ int, _ map[string]bool) {
	// do nothing
}

func last(name, pipeline string, i, n int, _ map[string]bool) {
	if i+1 != n {
		panic(
			fmt.Sprintf("transformation \"%s\" must run last in pipeline \"%s\"", name, pipeline))
	}
}
