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
package dfa

import (
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// Construct performs the data-flow analysis over a given kind of state using a
// given transfer function, producing some results.
func Construct[T State[T], C any](initial T, codes []C, transfer func(uint, C, T) []Transfer[T]) Result[T] {
	var (
		nCodes = uint(len(codes))
		result = Result[T]{make([]util.Option[T], nCodes)}
	)
	//
	if nCodes > 0 {
		result.states[0] = util.Some(initial)
	} else if nCodes == 0 {
		panic("unreachable")
	}
	//
	for i := range nCodes {
		ith := result.StateOf(i)
		//
		for _, tfr := range transfer(i, codes[i], ith) {
			result.JoinInto(tfr.target, tfr.state)
		}
	}
	//
	return result
}

// Transfer is essentially a pair identifying where a given state should be
// propagated during a dataflow analysis.
type Transfer[T any] struct {
	state  T
	target uint
}

// NewTransfer constructs a new transfer arc to join a given state into a given
// target.
func NewTransfer[T any](state T, target uint) Transfer[T] {
	return Transfer[T]{state, target}
}

// State represents an abstract data-flow state.  The purpose of doing this is
// to simplify the construction of different data-flow analyses over micro
// instructions.
type State[T any] interface {
	// String representation (primarily used for debugging)
	String(register.Map) string
	// Join combines two states together to produce a state representing both.
	// Typically, this happens when two paths converge on the same location and
	// the states from them are combined.
	Join(other T) T
}

// Result provides a mapping from micro-codes to dfa states.  In essence, it
// represents the output of the flow analysis.
type Result[T State[T]] struct {
	// For each micro-code, identifes the write state on entry to that micro-code.
	states []util.Option[T]
}

// StateOf returns the current state on entry to the given micro-code.
func (p *Result[T]) StateOf(i uint) T {
	var empty T
	//
	if p.states[i].HasValue() {
		return p.states[i].Unwrap()
	}
	//
	return empty
}

// JoinInto updates the write state for a given micro-code to include that from
// another branch.
func (p *Result[T]) JoinInto(i uint, st T) {
	var (
		ith = p.states[i]
		nst T
	)
	// Check for bottom
	if ith.HasValue() {
		nst = st.Join(ith.Unwrap())
	} else {
		nst = st
	}
	//
	p.states[i] = util.Some(nst)
}

func (p *Result[T]) String(rmap register.Map) string {
	var builder strings.Builder
	//
	for _, st := range p.states {
		if st.HasValue() {
			val := st.Unwrap()
			builder.WriteString(val.String(rmap))
		} else {
			builder.WriteString("⊥")
		}
	}
	//
	return builder.String()
}
