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
package agnostic

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/util/math"
	util_math "github.com/consensys/go-corset/pkg/util/math"
	"github.com/consensys/go-corset/pkg/util/poly"
)

// USE_NEW_CHUNKUP determines the chunkup algorithm to use.  The new one is
// slower, but gets better results.
const USE_NEW_CHUNKUP = false

// Assignment provides a generic notion of an assignment from an arbitrary
// polynomial to a given set of target registers.
type Assignment struct {
	// Target registers with least significant first
	LeftHandSide []register.Id
	// Right hand side.
	RightHandSide StaticPolynomial
}

// NewAssignment constructs a new assignment with a given Left-Hand Side (LHS)
// and Right-Hand Side (RHS).
func NewAssignment(lhs []register.Id, rhs StaticPolynomial) Assignment {
	// Sanity check
	if rhs == nil {
		panic("malformed assignment")
	}
	//
	return Assignment{lhs, rhs}
}

func (p *Assignment) String(env register.Map) string {
	var (
		builder  strings.Builder
		width, _ = p.Width(env)
	)
	//
	builder.WriteString("[")
	//
	for i := len(p.LeftHandSide); i > 0; {
		if i != len(p.LeftHandSide) {
			builder.WriteString(",")
		}

		i = i - 1

		builder.WriteString(env.Register(p.LeftHandSide[i]).Name())
	}
	//
	builder.WriteString(" := ")
	builder.WriteString(StaticPoly2String(p.RightHandSide, env))
	//
	fmt.Fprintf(&builder, "]^%d", width)
	//
	return builder.String()
}

// Width determines the minimal field width required to safely evaluate this
// assignment.  Hence, this should not exceed the field bandwidth.  The
// calculation is fairly straightforward: it is simply the maximum width of the
// left-hand and right-hand sides.
func (p *Assignment) Width(env register.Map) (bitwidth uint, signed bool) {
	var (
		// Determine lhs width
		lhs = CombinedWidthOfRegisters(env, p.LeftHandSide...)
		//
		rhs uint
	)
	// Determine rhs width
	rhs, signed = WidthOfPolynomial(p.RightHandSide, StaticEnvironment(env))
	//
	return max(lhs, rhs), signed
}

// Signed determines whether or not this is a signed assignment.  That is, an
// assignment which must additionally consider negative values.
func (p *Assignment) Signed(env register.Map) bool {
	var _, signed = p.Width(env)
	return signed
}

// Split an assignment according to a given field bandwidth.  This creates one
// or more assignments implementing the original which operate safely within the
// given bandwidth.  For example, consider the following assignment where all
// limbs are u8 (i.e. X'0, X'1, Y'0, and Y'1) and b is u1:
//
// b, X'1, X'0 = (2^8*Y'1 + Y'0) + 1
//
// This assignment cannot be safely evaluated within a field bandwidth of 16
// bits (i.e. because the right-hand side could overflow).  This is determined
// by checking the bandwidth against the computed width of the assignment
// (which, in this case, is 17).  Since the computed width exceeds the available
// bandwidth, the assignment needs to split as follows:
//
// b, X'1 = Y'1 + c
//
// c, X'0 = Y'0 + 1
//
// Here, c is an introduced u1 register for holding the "carry" (this is
// analoguous to carry flags as commonly found in CPU architectures).  In
// general, the algorithm can result in temporary registers of arbitrary size
// being introduced.  For example, consider a more complex case (again, u8
// limbs):
//
// X'3,X'2,X'1,X'0 = (2^8*Y'1 + Y'0) * (2^8*Z'1 + Z'0)
//
// Here, the right-hand side expands as follows into the appropriate polynomial
// representation:
//
// X'3,X'2,X'1,X'0 = (2^16*Y'1*Z'1) + (2^8*Y'1*Z'0) + (2^8*Y'0*Z'1) + (Y'0*Z'0)
//
// The difficulty here is that the left- and right-hand sides are somewhat
// "misaligned".  We can attempt to resolve this through large carry registers
// as follows:
//
//	         X'3 = (Y'1*Z'1) + c1
//
//	c1, X'2, X'1 = (Y'1*Z'0) + (Y'0*Z'1) + c0
//
//	     c0, X'0 = (Y'0*Z'0)
//
// Here, c0 and c1 are u8 and u1 carry registers respectively.  Unfortunately,
// this means the middle assignment has a bandwidth requirement of 17bits (which
// still exceeds our original target of 16bits).  Of course, if our bandwidth
// requirement was just slightly larger, then it would fit and we would be done.
//
// For (sub-)assignments which still exceed the bandwidth requirement (such as
// above), we must further split them by introducing additional temporary
// registers.
func (p *Assignment) Split(field field.Config, env RegisterAllocator) (eqs []Assignment) {
	var width, _ = p.Width(env)
	// Check whether any splitting required
	if width > field.BandWidth && USE_NEW_CHUNKUP {
		// Yes!
		eqs = p.chunkUp2(field, env)
	} else if width > field.BandWidth {
		// Yes!
		eqs = p.chunkUp(field, env)
	} else {
		// Nope
		eqs = []Assignment{*p}
	}
	//
	return eqs
}

// Cap all terms within a polynomial to ensure they can be safely evaluated
// within the given bandwidth.  For example, consider the following constraint
// (where both registers are u8):
//
// 0 == X * Y
//
// Suppose a bandwidth of 15bits.  Then, X*Y cannot be safely evaluated since it
// requires 16bits of information.  Instead, we have to break up either X or Y
// into smaller chunks.  Suppose we break X into two 4bit chunks, X'0 and X'1.
// Then we have:
//
// 0 == (256*X'1 + X'0) * Y
//
// --> 0 == 16*X'1*Y + X'0*Y
//
// At this point, each term can be safely evaluated within the given bandwidth
// and this equation can be chunked.  Observe that we assume supplementary
// constraints are included to enforce that X == 16*X'1 + X'0.
//
// The real challenge with this algorithm is, for a polynomial which cannot be
// chunked, to determine which variable(s) to subdivide and by how much.
func (p *Assignment) chunkUp(field field.Config, mapping RegisterAllocator) []Assignment {
	var (
		last      StaticPolynomial
		iteration = 0
		// Record initial number of registers
		n = uint(len(mapping.Registers()))
		//
		splitter = NewRegisterSplitter(n)
		// Determine the bitwidth of each chunk
		rhsChunks []RhsChunk
		lhsChunks []LhsChunk
		//
		initLhsChunks = initialiseLhsChunks(p.LeftHandSide, field, p.Signed(mapping), mapping)
	)
	//
	for {
		var (
			overflows bit.Set
			// Right-hand side
			right = splitter.Apply(p.RightHandSide, mapping)
		)
		// Attempt to chunk right-hand side
		lhsChunks, rhsChunks, overflows = determineRhsChunks(right, initLhsChunks, field, mapping)
		//
		if overflows.Count() == 0 {
			// Successful chunking, therefore include any constraints necessary
			// for splitting of non-linear terms and construct final equations.
			break
		} else if iteration > 1 && right.Equal(last) {
			// If we get here, then splitting made no improvement on the
			// previous iteration and we are stuck.  The idea is that this
			// should be unreachable, but it remains as a fall-back to prevent
			// the potential for an infinite loop.
			debugChunks(lhsChunks, rhsChunks, mapping)
			fmt.Printf("Divisions: %s\n", splitter.String(mapping))
			panic(fmt.Sprintf("malformed assignment (after %d iterations)", iteration))
		}
		// Update divisions based on identified overflows
		splitter.Subdivide(overflows)
		// Reset any allocated carry registers as we are starting over
		mapping.Reset(n)
		splitter.Reset()
		// Start next iteration
		last = right
		iteration++
	}
	// Initialise with splits
	assignments := splitter.assignments
	// Reconstruct equations
	for i := range len(lhsChunks) {
		l := lhsChunks[i]
		r := rhsChunks[i]
		//
		assignments = append(assignments, NewAssignment(l.contents, r.contents))
	}
	// Done
	return assignments
}

// Cap all terms within a polynomial to ensure they can be safely evaluated
// within the given bandwidth.  For example, consider the following constraint
// (where both registers are u8):
//
// 0 == X * Y
//
// Suppose a bandwidth of 15bits.  Then, X*Y cannot be safely evaluated since it
// requires 16bits of information.  Instead, we have to break up either X or Y
// into smaller chunks.  Suppose we break X into two 4bit chunks, X'0 and X'1.
// Then we have:
//
// 0 == (256*X'1 + X'0) * Y
//
// --> 0 == 16*X'1*Y + X'0*Y
//
// At this point, each term can be safely evaluated within the given bandwidth
// and this equation can be chunked.  Observe that we assume supplementary
// constraints are included to enforce that X == 16*X'1 + X'0.
//
// The real challenge with this algorithm is, for a polynomial which cannot be
// chunked, to determine which variable(s) to subdivide and by how much.
func (p *Assignment) chunkUp2(field field.Config, mapping RegisterAllocator) []Assignment {
	var (
		// Record initial number of registers
		n = uint(len(mapping.Registers()))
		// Current chunk width
		chunkWidth = field.RegisterWidth
		// Flag indicating whether chunking successful
		overflows bool
		// Indicates whether any more candidates can be split at this chunk
		// width.
		candidates uint = 1
		// Determine the bitwidth of each chunk
		rhsChunks   []RhsChunk
		lhsChunks   []LhsChunk
		assignments []Assignment
		//
		initLhsChunks = initialiseLhsChunks(p.LeftHandSide, field, p.Signed(mapping), mapping)
	)
	//
outer:
	for iteration := 0; ; iteration++ {
		//
		var (
			right StaticPolynomial = p.RightHandSide
		)
		// keep going until no more candidates to split.
		for candidates > 0 {
			var localAssignments []Assignment
			// Split right-hand side
			right, localAssignments, candidates = splitAssignmentRhs(chunkWidth, field, right, mapping)
			// Append all assignments
			assignments = append(assignments, localAssignments...)
			// Record current number of registers
			m := uint(len(mapping.Registers()))
			// Attempt to chunk right-hand side
			lhsChunks, rhsChunks, overflows = determineRhsChunks2(right, initLhsChunks, field, mapping)
			//
			if !overflows {
				// Successful chunking, therefore include any constraints necessary
				// for splitting of non-linear terms and construct final equations.
				break outer
			}
			// Reset any allocated carry registers as we are starting over
			mapping.Reset(m)
		}
		// Reset candidates to ensure go through again.
		candidates = 1
		// Reset assignments
		assignments = nil
		// Chunking unsuccessful, therefore decrease chunk width and try again.
		chunkWidth /= 2
		// Reset any allocated carry registers as we are starting over
		mapping.Reset(n)
	}
	// Reconstruct equations
	for i := range len(lhsChunks) {
		l := lhsChunks[i]
		r := rhsChunks[i]
		//
		assignments = append(assignments, NewAssignment(l.contents, r.contents))
	}
	// Done
	return assignments
}

func initialiseLhsChunks(regs []register.Id, field field.Config, signed bool, mapping register.Map) []LhsChunk {
	var (
		chunks     []Chunk[[]register.Id]
		chunkWidth = determineInitialChunkWidth(field)
	)
	//
	for len(regs) != 0 {
		var chunk Chunk[[]register.Id]
		// Determine next chunkd
		chunk, regs = getNextLhsChunk(regs, chunkWidth, signed, mapping)
		chunks = append(chunks, chunk)
	}
	//
	return chunks
}

// Determining the chunkwidth to use for initialising the left-hand side is
// somewhat subtle, and can impact both the performance of the algorithm and the
// overall chance of success.  In particular, it is useful to have at least one
// additional bit over the field's register bitwidth to account for sign bits.
// The chosen solution here is a heuristic which aims to ensure: (i) there is at
// least one additional bit of information; (ii) there are unused bits in the
// given bandwidth (e.g. as needed for carries, etc).
func determineInitialChunkWidth(field field.Config) uint {
	//var delta = max(1, (field.BandWidth-field.RegisterWidth)/4)
	var delta = uint(1)
	//
	return field.RegisterWidth + delta
}

func getNextLhsChunk(regs []register.Id, chunkWidth uint, signed bool, mapping register.Map) (LhsChunk, []register.Id) {
	var bitwidth uint
	//
	for i, r := range regs {
		var (
			reg  = mapping.Register(r)
			last = i+1 == len(regs)
			// Determine scenarios under which we will include the sign bit no
			// matter what.  This is necessary to prevent some odd situations,
			// such as where a chunk contains only the sign bit.
			includeSignBit = last && (reg.Width() == 1) && signed
		)
		//
		if (bitwidth+reg.Width() > chunkWidth) && !includeSignBit {
			return LhsChunk{bitwidth, regs[:i]}, regs[i:]
		}
		//
		bitwidth += reg.Width()
	}
	//
	return LhsChunk{bitwidth, regs}, nil
}

// nolint
func splitAssignmentRhs(regWidth uint, field field.Config, p StaticPolynomial,
	mapping RegisterAllocator) (StaticPolynomial, []Assignment, uint) {
	//
	var (
		splitter   = NewVariableSplitter(mapping, regWidth)
		env        = StaticEnvironment(mapping)
		vars       bit.Set
		maxWidth   = uint(0)
		candidates bit.Set
	)
	// Determine largest monomial(s) containing a register above chunk width.
	for i := range p.Len() {
		var (
			ith      = p.Term(i)
			width, _ = WidthOfMonomial[register.Id](ith, env)
		)
		//
		if hasCandidate(ith, regWidth, mapping) {
			maxWidth = max(maxWidth, width)
		}
	}
	// Pull out all candidate registers from largest monomial(s), whilst also
	// identifying whether any other monomials contain candidates.
	for i := range p.Len() {
		var (
			ith       = p.Term(i)
			width, _  = WidthOfMonomial[register.Id](ith, env)
			candidate = hasCandidate(ith, regWidth, mapping)
		)
		//
		if candidate && width == maxWidth {
			vars.Union(getCandidates(ith, regWidth, mapping))
		} else if candidate {
			candidates.Union(getCandidates(ith, regWidth, mapping))
		}
	}
	// Split all variables according to the given register width.
	assignments := splitter.StaticSplitVars(vars)
	// Substitute through the given polynomial
	return splitter.StaticApply(p), assignments, candidates.Count()
}

// Determine whether the given monomial contains a register which is greater
// than the given register width.  This is therefore a candidate for splitting.
// nolint
func hasCandidate(m StaticMonomial, regWidth uint, mapping register.Map) bool {
	for _, id := range m.Vars() {
		reg := mapping.Register(id)
		// Check for candidate
		if reg.Width() > regWidth {
			return true
		}
	}
	//
	return false
}

// Identify all registers in this monomial which are larger than the given
// register width.  These are therefore candidates for splitting.
// nolint
func getCandidates(m StaticMonomial, regWidth uint, mapping register.Map) bit.Set {
	var candidates bit.Set

	for _, id := range m.Vars() {
		reg := mapping.Register(id)
		// Check for candidate
		if reg.Width() > regWidth {
			candidates.Insert(id.Unwrap())
		}
	}
	//
	return candidates
}

// Divide a polynomial into "chunks", each of which has a maximum bitwidth as
// determined by the chunk widths.  This inserts carry and borrow registers as
// needed to ensure correctness of both signed and unsigned arithmetic.
func determineRhsChunks(p StaticPolynomial, chunks []LhsChunk, field field.Config,
	mapping RegisterAllocator) ([]LhsChunk, []RhsChunk, bit.Set) {
	//
	var (
		env       = StaticEnvironment(mapping)
		rhsChunks []RhsChunk
		lhsChunks []LhsChunk
		vars      bit.Set
		signed    bool
	)
	// Subdivide polynomial into chunks
	for i, chunk := range chunks {
		var (
			last      = i+1 == len(chunks)
			remainder StaticPolynomial
		)
		// Chunk the polynomial
		p, remainder = p.Shr(chunk.bitwidth)
		// Determine chunk width
		chunkWidth, s := RawWidthOfPolynomial(remainder, env)
		// Determine width of overflow
		overflow := chunkWidth - chunk.bitwidth
		// Check whether signed arithmetic begins
		signed = signed || s
		// Check whether chunk fits
		if chunkWidth > field.BandWidth {
			// No, it does not.
			vars.Union(RegisterReadSet(remainder))
		} else if !last && chunkWidth > chunk.bitwidth {
			// Overflow case.
			p, chunk = propagateCarry(chunk, overflow, p, mapping)
		}
		// Manage signed arithmetic
		if signed && !last {
			// Overflow case.
			p, chunk = propagateBorrow(chunk, overflow, p, mapping)
		}
		//
		rhsChunks = append(rhsChunks, RhsChunk{chunkWidth, remainder})
		lhsChunks = append(lhsChunks, chunk)
	}
	//
	return lhsChunks, rhsChunks, vars
}

// Divide a polynomial into "chunks", each of which has a maximum bitwidth as
// determined by the chunk widths.  This inserts carry and borrow registers as
// needed to ensure correctness of both signed and unsigned arithmetic.
// nolint
func determineRhsChunks2(p StaticPolynomial, chunks []LhsChunk, field field.Config,
	mapping RegisterAllocator) ([]LhsChunk, []RhsChunk, bool) {
	//
	var (
		env       = StaticEnvironment(mapping)
		rhsChunks []RhsChunk
		lhsChunks []LhsChunk
		overflows bool
		signed    bool
	)
	// Subdivide polynomial into chunks
	for i, chunk := range chunks {
		var (
			last      = i+1 == len(chunks)
			remainder StaticPolynomial
		)
		// Chunk the polynomial
		p, remainder = p.Shr(chunk.bitwidth)
		// Determine chunk width
		chunkWidth, s := RawWidthOfPolynomial(remainder, env)
		// Determine width of overflow
		overflow := chunkWidth - chunk.bitwidth
		// Check whether signed arithmetic begins
		signed = signed || s
		// Check whether chunk fits
		if chunkWidth > field.BandWidth {
			// No, it does not.
			overflows = true
		} else if !last && chunkWidth > chunk.bitwidth {
			// Overflow case.
			p, chunk = propagateCarry(chunk, overflow, p, mapping)
		}
		// Manage signed arithmetic
		if signed && !last {
			// Overflow case.
			p, chunk = propagateBorrow(chunk, overflow, p, mapping)
		}
		//
		rhsChunks = append(rhsChunks, RhsChunk{chunkWidth, remainder})
		lhsChunks = append(lhsChunks, chunk)
	}
	//
	return lhsChunks, rhsChunks, overflows
}

// Propagate a given number of overflow bits from the current chunk into the
// polynomial being carried forward into the next chunk.  For example, consider
// these two chunks (viewed as instructions):
//
// var x'0, x'1, y'0, y'1 u8
// var c u2
//
//    x'0 = 3 * y'0
// c, x'1 = y'1
//
// Looking at the first statement, we have the following alignment of bits:
//
//         9 8 7 6 5 4 3 2 1 0
//            +-+-+-+-+-+-+-+-+
// x'0        | | | | | | | | |
//        +-+-+-+-+-+-+-+-+-+-+
// 3*y'0: | | | | | | | | | | |
//        +-+-+-+-+-+-+-+-+-+-+
//
// We can see the right-hand side has an overflow of 2 bits.  Thus, we need to
// allocate a "carry register" of the given size and then propagate this into
// the next chunk (i.e. instruction).  That gives the following:
//
// var x'0, x'1, y'0, y'1 u8
// var c, c$2 u2
//
// c$2, x'0 = 3 * y'0
//   c, x'1 = y'1 + c$2
//
// Here, c$2 is the carry register allocated to balance the first assignment,
// and this then must be propagated into the second instruction.

func propagateCarry(chunk LhsChunk, overflow uint, carry StaticPolynomial,
	mapping RegisterAllocator) (StaticPolynomial, LhsChunk) {
	// Overflow case.
	var tmp StaticPolynomial
	// Allocate new register to get an Id
	carryRegId := mapping.Allocate("c", overflow)
	// Propage carry forward
	carry = carry.Add(tmp.Set(poly.NewMonomial(one, carryRegId)))
	// include carry in lhs
	return carry, LhsChunk{chunk.bitwidth, array.Append(chunk.contents, carryRegId)}
}

// Propagate a borrow bit from the current chunk into the polynomial being
// carried forward into the next chunk.  For example, consider these two chunks
// (viewed as instructions):
//
// var x'0, x'1, y'0, y'1 u8
// var b u1
//
//    x'0 = y'0 - 1
// b, x'1 = y'1
//
// In this case, we have 9bits of information from the right-hand side of the
// first instruction flowing into only 8bits on the left-hand side.  To make
// this work, we must introduce a specific "sign bit" to propagate the borrow
// originating in the first instruction into the second, like so:
//
// var x'0, x'1, y'0, y'1 u8
// var b, b$2 u1
//
// b$2, x'0 = y'0 - 1
//   b, x'1 = y'1 - b$2
//
// Here, b$2 is the allocated sign bit to account for the potential underflow in
// the first instruction.

func propagateBorrow(chunk LhsChunk, overflow uint, carry StaticPolynomial,
	mapping RegisterAllocator) (StaticPolynomial, LhsChunk) {
	var borrow StaticPolynomial
	// Allocate new register to get an Id
	signBit := mapping.Allocate("b", 1)
	// Put sign bit after carry (i.e. overflow)
	carry = carry.Add(borrow.Set(poly.NewMonomial(*math.NegPow2(overflow), signBit)))
	// include sign in lhs
	return carry, LhsChunk{chunk.bitwidth, array.Append(chunk.contents, signBit)}
}

// Chunk represents a "chunk of information bits".
type Chunk[T any] struct {
	bitwidth uint
	contents T
}

// LhsChunk captures the chunk type used for the Left-Hand Side (LHS) of an
// assignment.
type LhsChunk = Chunk[[]register.Id]

// RhsChunk captures the chunk type used for the Right-Hand Side (RHS) of an
// assignment.
type RhsChunk = Chunk[StaticPolynomial]

// StaticPoly2String provides a convenient helper function for debugging polynomials.
func StaticPoly2String(p StaticPolynomial, env register.Map) string {
	return poly.String(p, func(r register.Id) string {
		return env.Register(r.Id()).Name()
	})
}

// RegisterSplitter is used to manage the mechanism of splitting variables into
// limbs in order to improve the precision of a chunk.
type RegisterSplitter struct {
	divisions   []uint
	assignments []Assignment
	parents     map[register.Id]register.Id
}

// NewRegisterSplitter constructs a new splitter for a given number of variables.
func NewRegisterSplitter(n uint) RegisterSplitter {
	var (
		divisions = make([]uint, n)
		parents   = make(map[register.Id]register.Id)
	)
	//
	for i := range divisions {
		divisions[i] = 1
	}
	//
	return RegisterSplitter{divisions, nil, parents}
}

// ParentOf gets the original source variable from which this variable was
// derived (if it was indeed derived).  For example, if a register X is split
// into limbs X'1 and X'0, then the parent of the two limbs is X.  In the case
// that a register was not derived through splitting, then it is its own parent.
func (p *RegisterSplitter) ParentOf(v register.Id) register.Id {
	if parent, ok := p.parents[v]; ok {
		return parent
	}
	//
	return v
}

// Subdivide takes all registers in the given set and further subdivides them.
// For example, if they were previously being divided in 2, then they will now
// be divided in 4, etc.
func (p *RegisterSplitter) Subdivide(vars bit.Set) bool {
	var (
		changed = false
		parents bit.Set
	)
	// Normalise variables
	for iter := vars.Iter(); iter.HasNext(); {
		rid := register.NewId(iter.Next())
		parents.Insert(p.ParentOf(rid).Unwrap())
	}
	// Update division
	for i := range p.divisions {
		if parents.Contains(uint(i)) {
			p.divisions[i] *= 2
			changed = true
		}
	}
	//
	return changed
}

// Reset assignments created for the current splitting, and the ledger of
// parents.
func (p *RegisterSplitter) Reset() {
	p.assignments = nil
	// Reset parent of relationship
	p.parents = make(map[register.Id]register.Id)
}

// Apply the current subdivisions to a given polynomial.  Specifically, this
// splits all registers into their limbs and subsitutes them into the
// polynomial.  As a by-product this also records the assignments needed for the
// mapping from registers to their limbs.
func (p *RegisterSplitter) Apply(poly StaticPolynomial, mapping RegisterAllocator) StaticPolynomial {
	var cache = make(map[register.Id]StaticPolynomial)
	//
	for i, div := range p.divisions {
		var (
			rid = register.NewId(uint(i))
			reg = mapping.Register(rid)
		)
		//
		if div != 1 && reg.Width() > 1 {
			var limbPoly = p.splitVariable(rid, div, mapping)
			//
			cache[rid] = limbPoly
		}
	}
	//
	return SubstitutePolynomial(poly, func(reg register.Id) StaticPolynomial {
		if rp, ok := cache[reg]; ok {
			return rp
		}
		// no substitution
		return nil
	})
}

func (p *RegisterSplitter) splitVariable(rid register.Id, div uint, mapping RegisterAllocator) (r StaticPolynomial) {
	var (
		one      big.Int = *big.NewInt(1)
		rhs      StaticPolynomial
		terms    []StaticMonomial
		reg      = mapping.Register(rid)
		maxWidth = reg.Width() / div
		bitwidth = reg.Width()
		width    uint
	)
	// Round up (if necessary)
	if (maxWidth * div) < reg.Width() {
		maxWidth++
	}
	// Determine limb widths
	limbWidths := register.LimbWidths(maxWidth, reg.Width())
	// Allocate limbs
	limbs := p.alloc(rid, limbWidths, mapping)
	// Construct limb polynomial
	for i, limb := range limbs {
		var (
			c         = util_math.Pow2(width)
			limbWidth = min(bitwidth, limbWidths[i])
		)
		//
		if limbWidth > 0 {
			terms = append(terms, poly.NewMonomial(*c, limb))
			width += limbWidth
		}
		//
		bitwidth -= limbWidth
	}
	// Update assignments
	assignment := NewAssignment(limbs, rhs.Set(poly.NewMonomial(one, rid)))
	p.assignments = append(p.assignments, assignment)
	//
	return r.Set(terms...)
}

func (p *RegisterSplitter) String(mapping register.Map) string {
	var (
		builder strings.Builder
		first   = true
	)
	//
	builder.WriteString("[")
	//
	for i, div := range p.divisions {
		rid := register.NewId(uint(i))
		//
		if div > 1 {
			if !first {
				builder.WriteString(", ")
			}
			//
			name := mapping.Register(rid).Name()
			fmt.Fprintf(&builder, "%s/%d", name, div)
			//
			first = false
		}
	}
	//
	builder.WriteString("]")
	//
	return builder.String()
}

// Allocate the limbs for a parent variable which is being split.  This records
// the fact that those limbs were derived from this parent.
func (p *RegisterSplitter) alloc(parent register.Id, limbWidths []uint, mapping RegisterAllocator) []register.Id {
	var (
		reg   = mapping.Register(parent)
		limbs = mapping.AllocateN(reg.Name(), limbWidths)
	)
	// Record parent of each limbs
	for _, limb := range limbs {
		p.parents[limb] = parent
	}
	// Done
	return limbs
}

// useful for debugging the splitting algorithm.
//
// nolint
func debugChunks(lhs []LhsChunk, rhs []RhsChunk, mapping register.Map) {
	//
	for i := len(lhs); i > 0; i-- {
		ith := lhs[i-1]
		fmt.Printf("[u%d ", ith.bitwidth)

		for j := len(ith.contents); j > 0; j-- {
			rid := ith.contents[j-1]
			if j < len(ith.contents) {
				fmt.Printf(", ")
			}

			fmt.Print(mapping.Register(rid).Name())
		}
		//
		fmt.Print("]")
	}
	//
	fmt.Print(" := ")
	//
	for i := len(rhs); i > 0; i-- {
		ith := rhs[i-1]
		fmt.Printf("[u%d ", ith.bitwidth)
		//
		fmt.Print(StaticPoly2String(ith.contents, mapping))
		//
		fmt.Print("]")
	}
	//
	fmt.Println()
}
