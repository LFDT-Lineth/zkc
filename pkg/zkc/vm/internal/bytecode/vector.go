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
package bytecode

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Vector instructions are instructions composed of some number of micro
// instructions which, with restrictions, can be executed by the underlying
// machine "in parallel".  The approach is analoguous to the concept of
// "Very-Long Instruction Words (VLIW)" but taken to more of an extreme ---
// there is no limit on the number of micro-instructions.
//
// To better understand vector instructions, consider two instructions executed
// in sequence (the at pc location 0, the second at pc location 1):
//
// (pc=0) x = y + 1 (pc=1) z = 0
//
// When executing these instructions, there is an intermediate state after the
// first instruction is executed but before the second has been where x has been
// written but z has not.  Alternatively, the two instructions can be composed
// together to form a vector instruction, written like so:
//
// (pc=0) x = y + 1 ; z = 0
//
// In this case, both instructions are executed together and there is no
// intermediate state where x is written but z is not.
//
// To ensure easy translation into polynomial constraints, there are
// restrictions on how vector instructions can be composed.  In particular, no
// variable can be assigned twice on the same execution path.  Thus, for
// example, this is an invalid vector instruction:
//
// (pc=0) x = 0 ; x = 1
//
// These writes are said to be _conflicting_.  In contrast, the following is a
// valid vector instruction:
//
// (pc=0) skip_if x != y 2 ; r = 0 ; ret ; r = 1 ; ret
//
// In this case, whilst there are two assignments to register r, neither are on
// the same path.  These writes are said to be _non-conflicting_.  Finally, we
// should note that register forwarding is applied within vector instructions.
// Thus, for example, the following is allowed:
//
// (pc=0) x = 0; y = x + 1; ret
//
// Here, the value of x written in the instruction is "forwarded" to the
// assignment for y.  This process is, roughly speaking, analoguous to register
// forwarding as found in CPU architectures.
type Vector[W word.Word[W]] struct {
	Bytecodes []Bytecode[W]
}

// NewVector creates a new Vector instruction from a variadic list of Bytecode instructions.
// A Vector instruction is composed of multiple micro-instructions that can be executed
// "in parallel" by the underlying machine with certain restrictions to ensure
// easy translation into polynomial constraints.
//
// The function accepts a variadic parameter of Bytecode instructions and returns
// a Vector containing those bytecodes.
func NewVector[W word.Word[W]](bytecodes ...Bytecode[W]) Vector[W] {
	return Vector[W]{bytecodes}
}

// String returns a human-readable representation of this vector, rendering each
// constituent bytecode (resolved against the given environment) separated by
// " ; ".  This mirrors instruction.Vector.String.
func (p *Vector[W]) String(env Environment[W]) string {
	var builder strings.Builder
	//
	for i, b := range p.Bytecodes {
		if i != 0 {
			builder.WriteString(" ; ")
		}
		//
		builder.WriteString(b.String(env))
	}
	//
	return builder.String()
}

// Validate checks that this vector instruction is well-formed: every
// constituent bytecode must itself be well-formed, and there must be no
// conflicting reads or writes on any execution path.  This mirrors
// instruction.Vector.Validate.
//
// A write conflict arises when a register is written which _may_ already have
// been written on the same path; a read conflict arises when a register is read
// which _may_ (but not _definitely_) have been written.
func (p *Vector[W]) Validate(field FieldConfig, env Environment[W]) []error {
	var (
		errors, structureSafe      = p.validateStructure(env)
		controlErrors, controlSafe = p.validateControlFlow()
	)

	errors = append(errors, controlErrors...)
	// Validate individual bytecodes. Each bytecode checks its operands before
	// performing any environment lookup.
	for _, b := range p.Bytecodes {
		if !isNilBytecode(b) {
			errors = append(errors, b.Validate(field, env)...)
		}
	}
	// WriteMap assumes that every control-flow destination is in bounds and
	// every bytecode is non-nil.
	if !structureSafe || !controlSafe {
		return errors
	}

	return append(errors, p.validateReadWriteConflicts(env)...)
}

// validateStructure checks every index which an environment lookup or jump
// would dereference. The returned boolean indicates whether environment-
// dependent bytecode validation and write-map construction are safe.
func (p *Vector[W]) validateStructure(env Environment[W]) ([]error, bool) {
	var (
		errors []error
		safe   = true
	)

	for i, code := range p.Bytecodes {
		if isNilBytecode(code) {
			errors = append(errors, fmt.Errorf("bytecode %d is nil", i))
			safe = false

			continue
		}

		for _, registers := range [][]RegisterId{code.Uses(), code.Definitions()} {
			for _, id := range registers {
				if uint(id) >= env.RegisterCount() {
					safe = false
				}
			}
		}

		if jump, ok := code.(*Jmp[W]); ok && uint(jump.Target) >= env.VectorCount() {
			errors = append(errors, fmt.Errorf("bytecode %d: jump target %d does not exist", i, jump.Target))
			safe = false
		}
	}

	return errors, safe
}

// validateReadWriteConflicts checks for ambiguous reads and writes along every
// execution path through this vector.
func (p *Vector[W]) validateReadWriteConflicts(env Environment[W]) []error {
	var (
		errors   []error
		writeMap = p.WriteMap()
	)
	for i := range uint(len(p.Bytecodes)) {
		var (
			ithState = writeMap.StateOf(i)
			ith      = p.Bytecodes[i]
		)
		// Sanity check for conflicting reads.
		if !isUnsafeCall(ith, env) {
			for _, r := range ith.Uses() {
				if isZeroWidth(r, env) {
					continue
				}

				if rid := register.NewId(uint(r)); ithState.MaybeAssigned(rid) && !ithState.DefinitelyAssigned(rid) {
					errors = append(errors,
						fmt.Errorf("conflicting read on register \"%s\" in \"%s\"", RegisterToString(r, env), ith.String(env)))
				}
			}
		}
		// Sanity check for conflicting writes.
		for _, r := range ith.Definitions() {
			if isZeroWidth(r, env) {
				continue
			}

			if rid := register.NewId(uint(r)); ithState.MaybeAssigned(rid) {
				errors = append(errors,
					fmt.Errorf("conflicting write on register \"%s\" in \"%s\"", RegisterToString(r, env), ith.String(env)))
			}
		}
	}
	//
	return errors
}

func isUnsafeCall[W word.Word[W]](code Bytecode[W], env Environment[W]) bool {
	call, ok := code.(*Call[W])
	if !ok {
		return false
	}

	module := env.Module(call.Target)
	if module.IsEmpty() {
		return false
	}

	callee := module.Unwrap()

	return callee.IsFunction() && callee.HasUnsafeArgs()
}

// validateControlFlow checks the intra-vector control-flow graph.  Every skip
// destination must exist, including destinations in unreachable code, and
// every reachable path must end in a terminal bytecode.
func (p *Vector[W]) validateControlFlow() ([]error, bool) {
	var (
		errors []error
		n      = uint(len(p.Bytecodes))
		safe   = n != 0
	)
	if n == 0 {
		return []error{fmt.Errorf("empty vector")}, false
	}

	for i, code := range p.Bytecodes {
		if isNilBytecode(code) {
			safe = false
			continue
		}

		micro := uint(i)

		switch code := code.(type) {
		case *Skip[W]:
			errors, safe = validateSkipDestination(errors, safe, micro, uint(code.Skip), n)
		case *SkipIf[W]:
			errors, safe = validateSkipDestination(errors, safe, micro, uint(code.Skip), n)
		case *Switch[W]:
			for _, c := range code.Cases {
				errors, safe = validateSkipDestination(errors, safe, micro, uint(c.Skip), n)
			}
		case *Dispatch[W]:
			for _, c := range code.Cases {
				errors, safe = validateSkipDestination(errors, safe, micro, uint(c.Skip), n)
			}
		}
	}
	// Explore only valid successors. Invalid destinations were reported above.
	var (
		visited = make([]bool, n)
		visit   func(uint)
	)

	visit = func(micro uint) {
		if micro >= n {
			errors = append(errors, fmt.Errorf("reachable path falls off end of vector"))
			safe = false

			return
		}

		if visited[micro] {
			return
		}

		visited[micro] = true

		code := p.Bytecodes[micro]
		if isNilBytecode(code) {
			return
		}

		switch code := code.(type) {
		case *Fail[W], *Jmp[W], *Ret[W]:
			return
		case *Skip[W]:
			visitSkipDestination(micro, uint(code.Skip), n, visit)
		case *SkipIf[W]:
			visit(micro + 1)
			visitSkipDestination(micro, uint(code.Skip), n, visit)
		case *Switch[W]:
			visit(micro + 1)

			for _, c := range code.Cases {
				visitSkipDestination(micro, uint(c.Skip), n, visit)
			}
		case *Dispatch[W]:
			visit(micro + 1)

			for _, c := range code.Cases {
				visitSkipDestination(micro, uint(c.Skip), n, visit)
			}
		default:
			visit(micro + 1)
		}
	}
	visit(0)

	return errors, safe
}

func validateSkipDestination(errors []error, safe bool, micro, skip, length uint) ([]error, bool) {
	target := micro + skip + 1
	if target >= length {
		errors = append(errors, fmt.Errorf("bytecode %d: skip target %d does not exist", micro, target))
		return errors, false
	}

	return errors, safe
}

func visitSkipDestination(micro, skip, length uint, visit func(uint)) {
	target := micro + skip + 1
	if target < length {
		visit(target)
	}
}

func isNilBytecode[W word.Word[W]](code Bytecode[W]) bool {
	if code == nil {
		return true
	}

	value := reflect.ValueOf(code)

	return value.Kind() == reflect.Pointer && value.IsNil()
}

// Zero-width registers are placeholders introduced by register splitting. They
// carry no data, so apparent reads and writes to a shared placeholder cannot
// conflict.
func isZeroWidth[W word.Word[W]](id RegisterId, env Environment[W]) bool {
	width := env.Register(id).Bitwidth()
	return width.HasValue() && width.Unwrap() == 0
}

// WriteMap constructs the write map for this vector instruction.
//
// For each bytecode, the write map records — on entry to that bytecode — which
// registers have been written by preceding bytecodes (on any path to this
// point). This identifies: (1) whether a register _may_ have been written on
// some path; (2) or, whether it was _definitely_ written along all paths.  For
// example, consider the following sequence:
//
// x = 0; skip_if ... 1; y = 0; ret
//
// When execution reaches the return bytecode, we know that x was definitely
// written but only that y may have been written (i.e. depending on which path
// was taken).
//
// The write map serves two purposes:  firstly, it allows conflict detection;
// secondly, it identifies where register forwarding should be used.  A write
// conflict arises when a register is written which _may_ have already been
// written; likewise a read conflict arises when a register is read that _may_
// (but not _definitely_) have been written.  Finally, register forwarding
// arises when a register has _definitely_ been written by an earlier bytecode
// in the vector and, hence, subsequent reads use the new value (rather than the
// previous value).
func (p *Vector[W]) WriteMap() dfa.Result[dfa.Writes] {
	return dfa.Construct(dfa.Writes{}, p.Bytecodes, writeDfaTransfer[W])
}

// BranchTable returns the branch table for this vector instruction, and also
// its write map (since this is needed to compute the branch table anway). The
// branch table maps a _branch condition_ to each bytecode in the vector.  This
// identifies the conditions under which the given bytecode will execute.  For
// example, consider the following sequence:
//
// skip_if x!=0 1; y=0; skip_if x!=1 2; y=1; ret; y = 2; ret
// --------------+----+---------------+----+----+------+----
// 0             | 1  | 2             | 3  | 4  | 5    | 6
//
// This sequence gives rise to the following branch table:
//
// --+-------------+-----------------------
// 0 | skip_if ... | TRUE
// 1 | y=0         | x==0
// 2 | skip_if ... | x!=0
// 3 | y=1         | x!=0 && x==1 ==> x==1
// 4 | ret         | x!=0 && x==1 ==> x==1
// 5 | y=2         | x!=0 && x!=1
// 6 | ret         | x!=0 && x!=1
// --+-------------+-----------------------
//
// Observe that the optimiser automatically reduces "x!=0 && x==1" to just x==1
// (this is why it is sometimes called _branch table optimisation_).
func (p *Vector[W]) BranchTable(limbWidth uint) (dfa.Result[dfa.Writes], dfa.Result[dfa.Path[W]]) {
	// Construct suitable branch table for this vector instruction.
	var (
		entry    = dfa.EntryPoint[W]()
		writeMap = p.WriteMap()
		btf      = branchTableTransfer[W](writeMap, limbWidth)
	)
	//
	return writeMap, dfa.Construct(entry, p.Bytecodes, btf)
}

// writeDfaTransfer is the data-flow transfer function for the writes analysis
// over a bytecode vector, mirroring the instruction-level analysis (see
// instruction.writeDfaTransfer).
func writeDfaTransfer[W word.Word[W]](offset uint, code Bytecode[W],
	state dfa.Writes) []dfa.Transfer[dfa.Writes] {
	//
	var arcs []dfa.Transfer[dfa.Writes]
	//
	switch code := code.(type) {
	case *Fail[W], *Ret[W], *Jmp[W]:
		// Control-flow terminators: no fall-through within the vector.
		return nil
	case *Skip[W]:
		// Unconditional skip: control transfers only to the branch target.
		return append(arcs, dfa.NewTransfer(state, offset+uint(code.Skip)+1))
	case *SkipIf[W]:
		// Conditional skip: join into the branch target, then fall through.
		arcs = append(arcs, dfa.NewTransfer(state, offset+uint(code.Skip)+1))
	case *Switch[W]:
		// Multiway skip: join into each case's branch target (the skip writes
		// nothing, so the propagated state is unchanged); the fall-through is
		// added below.
		for _, c := range code.Cases {
			arcs = append(arcs, dfa.NewTransfer(state, offset+uint(c.Skip)+1))
		}
	case *Dispatch[W]:
		// One-hot dispatch: join into each case's branch target (the dispatch
		// writes nothing, so the propagated state is unchanged); the
		// fall-through is added below.
		for _, c := range code.Cases {
			arcs = append(arcs, dfa.NewTransfer(state, offset+uint(c.Skip)+1))
		}
	}
	// Construct state after this code and transfer to the following bytecode.
	nState := state.Write(toRegisterIds(code.Definitions())...)
	arcs = append(arcs, dfa.NewTransfer(nState, offset+1))
	//
	return arcs
}

// toRegisterIds converts a slice of bytecode-level registers (Reg) into the
// register.Id currency used by the data-flow writes analysis.
func toRegisterIds(regs []RegisterId) []register.Id {
	ids := make([]register.Id, len(regs))
	//
	for i, r := range regs {
		ids[i] = register.NewId(uint(r))
	}
	//
	return ids
}

// branchTableTransfer is the data-flow transfer function for the branch-table
// analysis over a bytecode vector, mirroring the instruction-level analysis
// (see instruction.branchTableTransfer).
func branchTableTransfer[W word.Word[W]](writeMap dfa.Result[dfa.Writes], limbWidth uint,
) dfa.PathTransferFunction[W, Bytecode[W]] {
	return func(offset uint, code Bytecode[W], state dfa.Path[W]) []dfa.Transfer[dfa.Path[W]] {
		var (
			arcs   []dfa.Transfer[dfa.Path[W]]
			writes = writeMap.StateOf(offset)
		)
		//
		switch code := code.(type) {
		case *Ret[W], *Jmp[W], *Fail[W]:
			// Control-flow terminators: their paths are valid executions which
			// genuinely never reach the subsequent codes, so they contribute
			// nothing to the conditions of those codes.
			return nil
		case *Skip[W]:
			// join into branch target
			return append(arcs, dfa.NewTransfer(state, offset+uint(code.Skip)+1))
		case *SkipIf[W]:
			var (
				// Determine true branch
				trueBranch = extendSkipIf(state, true, code, writes, limbWidth)
				// Determine false branch
				falseBranch = extendSkipIf(state, false, code, writes, limbWidth)
			)
			// join into branch target
			arcs = append(arcs, dfa.NewTransfer(trueBranch, offset+uint(code.Skip)+1))
			// join into following bytecode
			return append(arcs, dfa.NewTransfer(falseBranch, offset+1))
		case *Switch[W]:
			// Each case is reached when the source register equals that case's
			// value; the fall-through is reached only when no value matches.
			sid := register.NewId(uint(code.Source))
			source := dfa.NewBranchId(writes.MayAnybeAssigned(sid), sid)
			//
			for _, c := range code.Cases {
				branch := extendMultiway(state, source, c.Value, true)
				arcs = append(arcs, dfa.NewTransfer(branch, offset+uint(c.Skip)+1))
			}
			//
			return append(arcs, dfa.NewTransfer(extendMultiwayDefault(state, source, code.Cases), offset+1))
		case *Dispatch[W]:
			// One-hot dispatch: each edge's condition is just its own bit being
			// set — a single degree-1 atom, rather than the conjunction of all
			// preceding non-matches.  Dropping those prefix atoms is sound only
			// under the Dispatch contract (see its declaration): the enclosing
			// vector constrains the bits to be one-hot, so any satisfying trace
			// has bit_j = 1 imply every other bit (and hence every preceding
			// dispatch condition) clear.
			var (
				zero big.Int
				fall = state
			)
			//
			for _, c := range code.Cases {
				var (
					bid = register.NewId(uint(c.Bit))
					bit = dfa.NewBranchId(writes.MayAnybeAssigned(bid), bid)
				)
				//
				arcs = append(arcs, dfa.NewTransfer(state.NotEqualsConst(bit, zero), offset+uint(c.Skip)+1))
				// The fall-through is the syntactic complement of the case
				// edges — every bit clear — rather than the (logically
				// equivalent, degree-1) "default register clear".  This matters
				// wherever the edges rejoin: the disjunction of complementary
				// conditions simplifies back to the incoming path condition,
				// leaving the codes after the join unguarded, whereas a
				// default-register atom would survive the join and burden every
				// subsequent bytecode (and the constancy analysis) with a
				// disjunctive guard.
				fall = fall.EqualsConst(bit, zero)
			}
			//
			return append(arcs, dfa.NewTransfer(fall, offset+1))
		}
		// Transfer to following bytecode
		return append(arcs, dfa.NewTransfer(state, offset+1))
	}
}

// extendSkipIf conjoins the (in)equality tested by a conditional skip onto the
// incoming branch condition, for the given sign of the branch (true = skip
// taken).  The empty-tail handling exists because there is no implicit
// representation of logical truth: dfa.TRUE has no conjuncts, so the first atom
// replaces it rather than being and-ed onto it.
func extendSkipIf[W word.Word[W]](path dfa.Path[W], sign bool, code *SkipIf[W], writes dfa.Writes,
	limbWidth uint) dfa.Path[W] {
	//
	var (
		lhs = toRegisterIds(code.Left.Registers())
		//rhs      = toRegisterIds(code.Right.AsRegisters())
		rhsUsed = code.Right.IsRegisterVector()
		// NOTE: bytecode register vectors hold their most significant limb
		// first (i.e. at the lowest register id), hence big endian.
		left     = dfa.NewBigEndianBranchId(writes.MayAnybeAssigned(lhs...), lhs...)
		equality bool
	)
	// normalise condition
	switch code.Op {
	case CONDITION_EQ:
		equality = sign
	case CONDITION_NEQ:
		equality = !sign
	default:
		panic(fmt.Sprintf("unsupported skip condition (0x%x)", code.Op))
	}
	// Translate operation
	switch {
	case equality && rhsUsed:
		rhs := toRegisterIds(code.Right.AsRegisters())
		return path.Equals(left, dfa.NewBigEndianBranchId(writes.MayAnybeAssigned(rhs...), rhs...))
	case equality && !rhsUsed:
		// TODO: eventually we should be able to get right of asBigInt once path
		// condition is refactored.
		var rhs = asBigInt(limbWidth, code.Right.AsConstants()...)
		return path.EqualsConst(left, rhs)
	case !equality && rhsUsed:
		rhs := toRegisterIds(code.Right.AsRegisters())
		return path.NotEquals(left, dfa.NewBigEndianBranchId(writes.MayAnybeAssigned(rhs...), rhs...))
	case !equality && !rhsUsed:
		// TODO: eventually we should be able to get right of asBigInt once path
		// condition is refactored.
		var rhs = asBigInt(limbWidth, code.Right.AsConstants()...)
		return path.NotEqualsConst(left, rhs)
	default:
		panic("unreachable")
	}
}

// Convert a set of zero or more constant limbs into a big integer, where limbs
// are assumed to be arranged with the most signifciant limb first.  The width
// is required to know how wide each limbs should be.  This function is
// essentially the opposite of descriptor.SplitConstantReversed().
func asBigInt[W word.Word[W]](width uint, constants ...W) big.Int {
	var val big.Int
	//
	for i, c := range constants {
		if i != 0 {
			val.Lsh(&val, width)
		}
		// Add lower limb
		val.Add(&val, c.BigInt())
	}
	//
	return val
}

// extendMultiway conjoins a single dispatch comparison onto the incoming branch
// condition: "source == value" when equal is true, or "source != value"
// otherwise.  The empty-tail handling mirrors extendSkipIf (dfa.TRUE has no
// conjuncts, so the first atom replaces it rather than being and-ed onto it).
func extendMultiway[W word.Word[W]](path dfa.Path[W], source dfa.BranchId, value W, equal bool) dfa.Path[W] {
	//
	if equal {
		return path.EqualsConst(source, *value.BigInt())
	} else {
		return path.NotEqualsConst(source, *value.BigInt())
	}
}

// extendMultiwayDefault builds the condition under which a multiway skip falls
// through (i.e. no case matched): the conjunction of "source != value" over
// every case.
func extendMultiwayDefault[W word.Word[W]](path dfa.Path[W], source dfa.BranchId, cases []SwitchCase[W]) dfa.Path[W] {
	var branch = path
	//
	for _, c := range cases {
		branch = extendMultiway(branch, source, c.Value, false)
	}
	//
	return branch
}

// Map applies a function over each instruction in the vector which returns zero
// or more registers.  For example, the function could return nothing whenever
// it sees a "skip 0" operation (since this is a no-op).  A key feature of this
// function, however, is that it updates skip offsets correctly account the
// changes in width of instructions.  For example, consider this scenario:
//
// > skip_if x == 0 2 ; skip 0 ; ret ; jmp 1
//
// Then applying a map to remove "skip 0" instructions yields the following:
//
// > skip_if x == 0 1 ; ret ; jmp 1
//
// Specifically, we that the offset for the skip_if has been updated to reflect
// its new branch destination.  Observer, however, that the mapping process can
// fail if the mapping function is ill-behaved.  Consider this simple example:
//
// > skip 1 ; ret ; jmp 1
//
// Applying a mapping function which removed the "jmp 1" (for whatever reason)
// would lead to an invalid instruction and, hence, a mapping failure.
// Currently, mapping failures simply result in panics.
func (p *Vector[W]) Map(fun func(uint, Bytecode[W]) []Bytecode[W]) Vector[W] {
	var (
		packets [][]Bytecode[W] = make([][]Bytecode[W], len(p.Bytecodes))
		mapping []uint          = make([]uint, len(p.Bytecodes)+1)
		offset                  = uint(0)
	)
	// first, build all packets and the offset map
	for i, insn := range p.Bytecodes {
		ith := fun(uint(i), insn)
		// record package
		packets[i] = ith
		// update mapping
		mapping[i] = offset
		// update offset
		offset += uint(len(ith))
	}
	// assign offset to end-of-vector to support instructions which "skip of the
	// end".  Such instructions can arise before vectorisation is applied.
	mapping[len(p.Bytecodes)] = offset
	// reset offset
	offset = 0
	// recalculate skip offsets
	for i, pkt := range packets {
		// remap any skips which "escape" the packet.
		remapPacket(uint(i), offset, pkt, mapping)
		//
		offset += uint(len(pkt))
	}
	// flatten packets
	insns := array.FlatMap(packets, func(pkt []Bytecode[W]) []Bytecode[W] { return pkt })
	// done
	return NewVector(insns...)
}

// Remap all skip instructions within given "packet" of instructions.  There are
// two cases to consider: an internal or external skip.  Here, the latter are
// subject to remapping whilst the former are not. To understand this consider a
// mapping to remove "skip 0" in the following:
//
// > skip_if x == 0 2 ; skip 0 ; ret ; jmp 1
//
// The packets generated for this would be:
//
// > [skip_if x == 0 2][][ret][jmp 1]
//
// You can see the second packet is empty, which is how the "skip 0" ends up
// being removed.  Now, looking at the first packet.  This contains only a
// single skip instruction and, hence, the target of that skip lies outside the
// packet itself.  In such case, we refer to the skip as an "external" skip.
//
// Finally, to understand internal skips.  These are skips introduced by the
// mapping function itself to implement conditional logic within the new
// instructions.  As such, they should not be remapped as they should already
// have correct skip offsets.
func remapPacket[W word.Word[W]](oldOffset, newOffset uint, packet []Bytecode[W], mapping []uint) {
	var (
		n = uint(len(packet))
	)
	//
	for i, insn := range packet {
		if isExternalSkip(n-uint(i), insn) {
			packet[i] = remapSkip(n-uint(i), oldOffset, newOffset+uint(i), insn, mapping)
		}
	}
}

// isExternalSkip determines whether insn contains a skip whose target lies
// outside the enclosing packet -- i.e. a skip of n or more, where n is the
// number of instructions from insn to the end of the packet.  A multiway skip
// is external when any of its cases is.
func isExternalSkip[W word.Word[W]](n uint, insn Bytecode[W]) bool {
	var c any = insn
	//
	switch insn := c.(type) {
	case *Skip[W]:
		return uint(insn.Skip) >= n
	case *SkipIf[W]:
		return uint(insn.Skip) >= n
	case *Switch[W]:
		for _, cse := range insn.Cases {
			if uint(cse.Skip) >= n {
				return true
			}
		}
		//
		return false
	case *Dispatch[W]:
		for _, cse := range insn.Cases {
			if uint(cse.Skip) >= n {
				return true
			}
		}
		//
		return false
	default:
		return false
	}
}

// remapSkip rewrites the external skip(s) in insn so they continue to identify
// the same target after the surrounding instructions have been re-laid-out.
// Skips internal to the packet (offset < n) are left unchanged; that only
// matters for a multiway skip, whose cases may mix internal and external skips.
func remapSkip[W word.Word[W]](n, oldOffset, newOffset uint, insn Bytecode[W], mapping []uint) Bytecode[W] {
	// remap maps an (external) skip offset to its new value: look up the new
	// position of the original target, then make it relative to newOffset.
	remap := func(skip uint16) uint16 {
		var (
			// determine target address
			target = mapping[oldOffset+uint(skip)+1]
			// calculate new skip
			nskip = target - newOffset - 1
		)
		//
		return util.Cast[uint16](nskip)
	}
	// reconstruct skip
	switch insn := insn.(type) {
	case *Skip[W]:
		return &Skip[W]{Skip: remap(insn.Skip)}
	case *SkipIf[W]:
		return &SkipIf[W]{Op: insn.Op, Left: insn.Left, Right: insn.Right, Skip: remap(insn.Skip)}
	case *Switch[W]:
		ncases := make([]SwitchCase[W], len(insn.Cases))
		//
		for j, cse := range insn.Cases {
			ncases[j] = cse
			// only external cases are remapped; internal ones are unchanged
			if uint(cse.Skip) >= n {
				ncases[j].Skip = remap(cse.Skip)
			}
		}
		//
		return &Switch[W]{Source: insn.Source, Cases: ncases}
	case *Dispatch[W]:
		ncases := make([]DispatchCase, len(insn.Cases))
		//
		for j, cse := range insn.Cases {
			ncases[j] = cse
			// only external cases are remapped; internal ones are unchanged
			if uint(cse.Skip) >= n {
				ncases[j].Skip = remap(cse.Skip)
			}
		}
		//
		return &Dispatch[W]{Cases: ncases, Default: insn.Default}
	default:
		panic("unreachable")
	}
}
