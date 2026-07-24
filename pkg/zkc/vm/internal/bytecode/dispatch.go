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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// DispatchCase is a single (bit, target) edge of a one-hot dispatch: when the
// bit register holds 1, control transfers to the target.
type DispatchCase struct {
	// Bit register examined by this case.
	Bit RegisterId
	// Skip amount.
	Skip uint16
}

// Dispatch is a one-hot multiway skip: each case's 1-bit register is examined,
// in order, and control transfers to the target of the first bit which is set.
// When no bit is set, control falls through to the following instruction.  The
// Default register is a 1-bit register holding the complement of the case-bit
// sum (i.e. it is 1 exactly when no case bit is set); it is not needed for
// execution, but identifies the fall-through indicator for consumers with
// one-hot knowledge.
//
// NOTE: this bytecode is compiler-internal — it is only ever emitted by the
// LowerSwitch transform, under a strict contract: the enclosing vector must
// constrain every case bit to be the indicator of a distinct dispatch
// condition (so that at most one bit is set), and the Default register to be
// the complement of their sum.  The branch condition derived for each case edge — "bit != 0", a
// single degree-1 atom rather than the conjunction of all preceding
// non-matches — is only sound under that contract; a free-standing Dispatch
// over unconstrained bits would allow a prover to steer execution
// arbitrarily.  (The fall-through condition, by contrast, is the syntactic
// complement of the case edges — every bit clear — so that the disjunction of
// all edge conditions simplifies away wherever the edges rejoin.)
type Dispatch[W word.Word[W]] struct {
	Cases []DispatchCase
	// Default identifies the (1-bit) register holding the sum of the case
	// bits.
	Default RegisterId
}

// NewDispatch constructs a one-hot dispatch over the given (bit, target)
// cases, with the given default (no-match indicator) register.
func NewDispatch[W word.Word[W]](cases []DispatchCase, dflt RegisterId) *Dispatch[W] {
	return &Dispatch[W]{Cases: cases, Default: dflt}
}

// Uses implementation for Bytecode interface.  A dispatch reads every case bit
// along with the default register.
func (p *Dispatch[W]) Uses() []RegisterId {
	uses := make([]RegisterId, 0, len(p.Cases)+1)
	//
	for _, c := range p.Cases {
		uses = append(uses, c.Bit)
	}
	//
	return append(uses, p.Default)
}

// Definitions implementation for Bytecode interface.
func (p *Dispatch[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.  Every register examined by
// a dispatch must be a 1-bit register: the branch conditions derived from this
// bytecode are only meaningful (and sound) over bits.
func (p *Dispatch[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errors := validateOperands(env, p.Uses())
	if len(errors) != 0 {
		return errors
	}
	//
	for _, r := range p.Uses() {
		if width := env.Register(r).Bitwidth(); width.UnwrapOr(0) != 1 {
			errors = append(errors, fmt.Errorf("dispatch register \"%s\" is not a 1-bit register",
				RegisterToString(r, env)))
		}
	}
	//
	return errors
}

func (p *Dispatch[W]) String(env Environment[W]) string {
	var b strings.Builder
	//
	b.WriteString("dispatch [")
	//
	for i, c := range p.Cases {
		if i != 0 {
			b.WriteString(", ")
		}
		//
		fmt.Fprintf(&b, "%s:%d", RegisterToString(c.Bit, env), c.Skip)
	}
	//
	fmt.Fprintf(&b, "] %s", RegisterToString(p.Default, env))
	//
	return b.String()
}
