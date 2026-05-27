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
	"strings"

	"github.com/consensys/go-corset/pkg/ir/term"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/util/field"
	util_math "github.com/consensys/go-corset/pkg/util/math"
	"github.com/consensys/go-corset/pkg/util/poly"
	"github.com/consensys/go-corset/pkg/util/word"
)

// RegisterAllocator is used to allocate fresh registers with optional
// "fillers". That is, computation which can be used to assign values to them in
// the final trace.
type RegisterAllocator = register.Allocator[term.Computation[word.BigEndian]]

// Equation provides a generic notion of an equation between two polynomials.
// An equation is in *balanced form* if neither side contains a negative
// coefficient.
type Equation struct {
	// Left hand side.
	LeftHandSide DynamicPolynomial
	// Right hand side.
	RightHandSide DynamicPolynomial
}

// NewEquation simply constructs a new equation.
func NewEquation(lhs DynamicPolynomial, rhs DynamicPolynomial) Equation {
	return Equation{lhs, rhs}
}

// Balance an equation means to convert it such that no negative coefficients
// remain. For example, balancing the equation "0 == x - 1" gives "1 == x".  The
// benefit of balancing is simply that it eliminates any requirement for an
// interpretation of signed values.
func (p *Equation) Balance() Equation {
	// Check whether any work to do
	if !p.LeftHandSide.Signed() && !p.RightHandSide.Signed() {
		return *p
	}
	// Yes, work to be done
	var (
		lhsPos, lhsNeg = balancePolynomial(p.LeftHandSide)
		rhsPos, rhsNeg = balancePolynomial(p.RightHandSide)
	)
	// Done
	return NewEquation(lhsPos.Add(rhsNeg), rhsPos.Add(lhsNeg))
}

// Width determines the minimal field width required to safely evaluate this
// assignment.  Hence, this should not exceed the field bandwidth.  The
// calculation is fairly straightforward: it is simply the maximum width of the
// left-hand and right-hand sides.
func (p *Equation) Width(mapping register.Map) uint {
	var (
		env = DynamicEnvironment()
		// Determine lhs width
		lhs, lSign = WidthOfPolynomial(p.LeftHandSide, env)
		// Determine rhs width
		rhs, rSign = WidthOfPolynomial(p.RightHandSide, env)
	)
	// Sanity check
	if lSign || rSign {
		panic("equation not balanced!")
	}
	//
	return max(lhs, rhs)
}

func (p *Equation) String(mapping register.Map) string {
	var (
		builder strings.Builder
		env     = DynamicEnvironment()
		// Determine lhs width
		lhs, lSign = WidthOfPolynomial(p.LeftHandSide, env)
		// Determine rhs width
		rhs, rSign = WidthOfPolynomial(p.RightHandSide, env)
		//
		width = fmt.Sprintf("%d", max(lhs, rhs))
	)
	// Sanity check
	if lSign || rSign {
		width = "?"
	}
	//
	builder.WriteString("[")
	// Write left-hand side
	builder.WriteString(DynamicPoly2String(p.LeftHandSide, mapping))
	//
	builder.WriteString(" == ")
	// write right-hand side
	builder.WriteString(DynamicPoly2String(p.RightHandSide, mapping))
	//
	fmt.Fprintf(&builder, "]^%s", width)
	//
	return builder.String()
}

// Split an equation according to a given field bandwidth.  This creates one
// or more equations implementing the original which operate safely within the
// given bandwidth.
func (p *Equation) Split(field field.Config, env RegisterAllocator) (targets, context []Equation) {
	var (
		bp = p.Balance()
	)
	// Check whether any splitting required
	if bp.Width(env) > field.BandWidth {
		// Yes!
		targets, context = bp.chunkUp(field, env)
	} else {
		// Nope
		targets = []Equation{*p}
	}
	//
	return targets, context
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
func (p *Equation) chunkUp(field field.Config, mapping RegisterAllocator) (targets, context []Equation) {
	var (
		// Record initial number of registers
		n = uint(len(mapping.Registers()))
		// Determine the bitwidth of each chunk
		lhs, rhs []DynamicPolynomial
		// Current chunk width
		chunkWidth = field.RegisterWidth
	)
	// Attempt to divide polynomials into chunks.  If this fails, iterative
	// decrease chunk width until something fits.
	for {
		var (
			lhsChunked, rhsChunked bool
			// Calculate actual chunk widths based on current chunk width.
			chunkWidths = p.determineChunkBitwidths(chunkWidth, mapping)
			//
			left, leftEqs   = splitNonLinearTerms(chunkWidth, field, p.LeftHandSide, mapping)
			right, rightEqs = splitNonLinearTerms(chunkWidth, field, p.RightHandSide, mapping)
		)
		// Attempt to chunk polynomials
		lhsChunks, rhsChunks := chunkEquation(left, right, chunkWidths)
		lhs, lhsChunked = addCarryLines(lhsChunks, chunkWidths, field, mapping)
		rhs, rhsChunked = addCarryLines(rhsChunks, chunkWidths, field, mapping)
		//
		if lhsChunked && rhsChunked {
			// Successful chunking, therefore include any constraints necessary
			// for splitting of non-linear terms and construct final equations.
			context = append(leftEqs, rightEqs...)
			//
			break
		}
		// Chunking unsuccessful, therefore decrease chunk width and try again.
		chunkWidth /= 2
		// Reset any allocations made as we are starting over
		mapping.Reset(n)
	}
	// Reconstruct target equations
	for i := range max(len(lhs), len(rhs)) {
		if lhs[i].Len() > 0 || rhs[i].Len() > 0 {
			targets = append(targets, NewEquation(lhs[i], rhs[i]))
		}
	}
	// Done
	return targets, context
}

// Determine the width of individual chunks used to split the equation.  In
// theory, arbitrary chunk widths can be used provided the total bitwidth
// encloses both sides (i.e. contains all possible value for each side).  In
// practice, the chunking used can affect the overall efficiency of the
// splitting.  As an example consider the following simple equation, where x and
// y are u16:
//
//	x == y + 1
//
// Assuming a desired register width of u8, the derived equation is:
//
//	256*x1 + x0 == 256*y1 + y0 + 1
//
// At this point, we can compare the two sides as follows:
//
//	 15             8 7               0
//	+----------------+-----------------+
//	|     2^8*x1     |        x0       |
//	+----------------+-----------------+
//	+----------------+
//	|     2^8*y1     |
//	+----------------+
//	               +----------------+
//	               |     y0 + 1     |
//	               +----------------+
//
// In this example, a good chunking would be to divide into two u8 chunks.  This
// works well since 3/4 of our boxes are byte aligned already.
//
// In general, chunks do not have to have the same size (even though it did make
// sense above).  In particular, the most significant chunk is often a different
// size.
func (p *Equation) determineChunkBitwidths(maxWidth uint, mapping RegisterAllocator) []uint {
	var (
		bitwidth = p.Width(mapping)
		chunks   []uint
	)
	//
	for bitwidth > 0 {
		// Determine how much to take off
		width := min(maxWidth, bitwidth)
		// Update the chunk
		chunks = append(chunks, width)
		bitwidth -= width
	}
	//
	return chunks
}

func splitNonLinearTerms(regWidth uint, field field.Config, p DynamicPolynomial,
	mapping RegisterAllocator) (DynamicPolynomial, []Equation) {
	//
	var (
		env      = DynamicEnvironment()
		splitter = NewVariableSplitter(mapping, regWidth)
		vars     bit.Set
	)
	//
	for i := range p.Len() {
		var (
			term          = p.Term(i)
			width, signed = WidthOfMonomial(term, env)
		)
		//
		if signed {
			panic("unbalance equation encountered")
		} else if width > field.BandWidth {
			for _, v := range term.Vars() {
				// Check whether register is above threshold or not.
				if mapping.Register(v.Id()).Width() > regWidth {
					// Yes, so mark it for splitting.
					vars.Insert(v.Unwrap())
				}
			}
		}
	}
	// Split all variables according to the given register width.
	constraints := splitter.SplitVariables(vars)
	// Substitute through the given polynomial
	return splitter.Apply(p), constraints
}

// Divide a polynomial into "chunks", each of which has a maximum bitwidth as
// determined by the chunk widths.
func chunkEquation(lhs, rhs DynamicPolynomial, chunkWidths []uint) (l, r []DynamicPolynomial) {
	//
	var (
		lhsChunks []DynamicPolynomial
		rhsChunks []DynamicPolynomial
	)
	// Subdivide polynomial into chunks
	for _, chunkWidth := range chunkWidths {
		var lhsRem, rhsRem DynamicPolynomial
		// Chunk the polynomials
		lhs, lhsRem = lhs.Shr(chunkWidth)
		rhs, rhsRem = rhs.Shr(chunkWidth)
		// Include remainders (if non-zero)
		if lhs.Len() != 0 || rhs.Len() != 0 || lhsRem.Len() != 0 || rhsRem.Len() != 0 {
			lhsChunks = append(lhsChunks, lhsRem)
			rhsChunks = append(rhsChunks, rhsRem)
		}
	}
	//
	return lhsChunks, rhsChunks
}

// Add carry lines into chunks as needed to ensure correctness.
func addCarryLines(chunks []DynamicPolynomial, chunkWidths []uint, field field.Config,
	mapping RegisterAllocator) ([]DynamicPolynomial, bool) {
	//
	var (
		env = DynamicEnvironment()
	)
	// Add carry lines as necessary
	for i := 0; i < len(chunks); i++ {
		var (
			carry, borrow DynamicPolynomial
			ithWidth, _   = WidthOfPolynomial(chunks[i], env)
			chunkWidth    = chunkWidths[i]
		)
		// Calculate overflow from ith chunk (if any)
		if ithWidth > field.BandWidth {
			// This arises when a given term of the polynomial being chunked
			// cannot be safely evaluated within the given bandwidth (i.e.
			// cannot be evaluated without overflow).  To resolve this
			// situation, we need to further subdivide one or more registers to
			// reduce the maximum bandwidth required for any particular term.
			return []DynamicPolynomial{chunks[i]}, false
		} else if (i+1) != len(chunks) && ithWidth > chunkWidth {
			var (
				// Construct filler for carry register
				filler = NewPolyFil(chunkWidth, chunks[i])
				// Determine width of carry register
				carryWidth = ithWidth - chunkWidth
				// Allocate carry register
				carryReg = mapping.AllocateWith("c", carryWidth, filler)
				// Calculate amount to shift carry
				chunkShift = util_math.Pow2(chunkWidth)
			)
			// Subtract carry from this chunk
			chunks[i] = chunks[i].Sub(borrow.Set(poly.NewMonomial(*chunkShift, carryReg.AccessOf(carryWidth))))
			// Add carry to next chunk
			chunks[i+1] = chunks[i+1].Add(carry.Set(poly.NewMonomial(one, carryReg.AccessOf(carryWidth))))
		}
	}
	//
	return chunks, true
}

// Split a polynomial into its positive and negative components.
func balancePolynomial(poly DynamicPolynomial) (pos, neg DynamicPolynomial) {
	// Set both sides to zero
	pos = pos.Set()
	neg = neg.Set()
	//
	for i := range poly.Len() {
		var (
			tmp DynamicPolynomial
			ith = poly.Term(i)
		)
		//
		tmp = tmp.Set(ith)
		//
		if ith.IsNegative() {
			neg = neg.Sub(tmp)
		} else {
			pos = pos.Add(tmp)
		}
	}
	//
	return pos, neg
}

// DynamicPoly2String provides a convenient helper function for debugging polynomials.
func DynamicPoly2String(p DynamicPolynomial, env register.Map) string {
	return poly.String(p, RegAccess2String(env))
}

// RegAccess2String provides a convenient helper function for debugging polynomials.
func RegAccess2String(env register.Map) func(r register.AccessId) string {
	return func(r register.AccessId) string {
		var (
			name  = env.Register(r.Id()).Name()
			shift = r.RelativeShift()
		)
		//
		switch {
		case shift == 0:
			return fmt.Sprintf("%s[i]", name)
		case shift > 0:
			return fmt.Sprintf("%s[i+%d]", name, shift)
		default:
			return fmt.Sprintf("%s[i%d]", name, shift)
		}
	}
}
