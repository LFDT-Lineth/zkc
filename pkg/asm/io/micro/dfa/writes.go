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

	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/bit"
)

// Writes identifies, for a set of registers, whether each may have been
// written (i.e. maybe assigned) and, furthermore, whether it has definitely
// been written (i.e. definitely assigned).  Observe that definitely assigned
// registers are also always maybe assigned registers.  This is used to provide
// information about writes occurring within the micro instruction.  This is
// necessary, for example, to determine when forwarding should be applied.
// Specifically, on entry to each micro-code, this identifies which variables
// may have been written at the given point. Furthermore, it allows us to
// distinguish between those which may have been written and those which have
// definitely been written.
type Writes struct {
	maxRegister    uint
	definiteWrites bit.Set
	maybeWrites    bit.Set
}

// Clone this write state producing an otherwise identical but physically
// disjoint state.
func (p Writes) Clone() Writes {
	var nst Writes
	//
	nst.maxRegister = p.maxRegister
	nst.definiteWrites.Union(p.definiteWrites)
	nst.maybeWrites.Union(p.maybeWrites)
	//
	return nst
}

// Write constructs a write state representing the give state after a set of
// writes have occurred.
func (p Writes) Write(regs ...register.Id) Writes {
	var nst = p.Clone()
	//
	for _, r := range regs {
		nst.maxRegister = max(nst.maxRegister, r.Unwrap())
		nst.definiteWrites.Insert(r.Unwrap())
		nst.maybeWrites.Insert(r.Unwrap())
	}
	//
	return nst
}

// Join combines two write states together.
func (p Writes) Join(q Writes) Writes {
	var nst Writes
	//
	nst.maxRegister = max(p.maxRegister, q.maxRegister)
	//
	for i := range nst.maxRegister + 1 {
		rid := register.NewId(i)
		//
		if p.DefinitelyAssigned(rid) && q.DefinitelyAssigned(rid) {
			nst.definiteWrites.Insert(i)
			nst.maybeWrites.Insert(i)
		} else if p.MaybeAssigned(rid) || q.MaybeAssigned(rid) {
			nst.maybeWrites.Insert(i)
		}
	}
	//
	return nst
}

// MaybeAssigned determines whether or not a give register may have been
// assigned.
func (p Writes) MaybeAssigned(reg register.Id) bool {
	return p.maybeWrites.Contains(reg.Unwrap())
}

// MayAnybeAssigned determines whether or not any of the given registers may have been
// assigned.
func (p Writes) MayAnybeAssigned(regs ...register.Id) bool {
	for _, r := range regs {
		if p.maybeWrites.Contains(r.Unwrap()) {
			return true
		}
	}

	return false
}

// DefinitelyAssigned determines whether or not a give register has definitely
// been assigned.  Observe that, for any register r, it follows that
// DefinitelyAssigned(r) implies MaybeAssigned(r).
func (p Writes) DefinitelyAssigned(reg register.Id) bool {
	return p.definiteWrites.Contains(reg.Unwrap())
}

func (p Writes) String(rmap register.Map) string {
	var (
		builder strings.Builder
		first   = true
		nRegs   = uint(len(rmap.Registers()))
	)
	//
	builder.WriteString("{")
	//
	for i := uint(0); i < nRegs; i++ {
		if !p.maybeWrites.Contains(i) {
			continue
		}
		//
		if !first {
			builder.WriteString(",")
		} else {
			first = false
		}
		//
		if !p.definiteWrites.Contains(i) {
			builder.WriteString("?")
		}
		//
		var (
			rid  = register.NewId(i)
			name = rmap.Register(rid).Name()
		)
		//
		builder.WriteString(name)
	}
	//
	builder.WriteString("}")
	//
	return builder.String()
}
