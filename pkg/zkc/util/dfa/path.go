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
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/logical"
)

// PathTransferFunction represents a transfer function over branch state.
type PathTransferFunction[W util.BigInter, I any] func(offset uint, code I, state Path[W]) []Transfer[Path[W]]

// EntryPoint constructs a path representing the entry point of a function.
func EntryPoint[W util.BigInter]() Path[W] {
	return Path[W]{
		logical.Truth[BranchId, BranchEquality](true),
		logical.Truth[BranchId, BranchEquality](false),
	}
}

// Path adapts a branch condition to be an instance of State.
type Path[W util.BigInter] struct {
	condition BranchCondition
	// dead holds the condition under which this code is reached only through
	// a preceding fail.  Rows satisfying it are rejected by that fail's own
	// constraint, so it is a "don't care" for every consumer of the reach
	// condition: any condition may soundly be widened by (any part of) it.
	dead BranchCondition
}

// Condition returns the underlying branch condition
func (p Path[W]) Condition() BranchCondition {
	return p.condition
}

// Dead returns the condition under which this code is reached only through a
// preceding fail — i.e. only on rows which the fail's constraint rejects.
func (p Path[W]) Dead() BranchCondition {
	return p.dead
}

// Die marks this path as dying at a fail: the resulting path's reach
// condition is unreachable, whilst its dead condition records the conditions
// under which the fail fires.  Successor codes joined with this path thus
// inherit no reach condition from it, but may absorb the dead condition where
// that simplifies (see Join).
func (p Path[W]) Die() Path[W] {
	return Path[W]{
		logical.Truth[BranchId, BranchEquality](false),
		p.condition.Or(p.dead),
	}
}

// Equals extends the current path with a new constraint that two variables are equal.
func (p Path[W]) Equals(lhs, rhs BranchId) Path[W] {
	var prop = logical.NewProposition(logical.Equals(lhs, rhs))
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop, p.dead}
	}
	//
	return Path[W]{p.condition.And(prop), p.dead}
}

// NotEquals extends the current path with a new constraint that two variables are not equal.
func (p Path[W]) NotEquals(lhs, rhs BranchId) Path[W] {
	var prop = logical.NewProposition(logical.NotEquals(lhs, rhs))
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop, p.dead}
	}
	//
	return Path[W]{p.condition.And(prop), p.dead}
}

// EqualsConst extends the current path with a new constraint that a given variable equals a given constant.
func (p Path[W]) EqualsConst(lhs BranchId, rhs big.Int) Path[W] {
	var (
		prop = logical.NewProposition(logical.EqualsConst(lhs, rhs))
	)
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop, p.dead}
	}
	//
	return Path[W]{p.condition.And(prop), p.dead}
}

// NotEqualsConst extends the current path with a new constraint that a given variable does not equal a given constant.
func (p Path[W]) NotEqualsConst(lhs BranchId, rhs big.Int) Path[W] {
	var (
		prop = logical.NewProposition(logical.NotEqualsConst(lhs, rhs))
	)
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop, p.dead}
	}
	//
	return Path[W]{p.condition.And(prop), p.dead}
}

// Join implementation for State interface.  Both components are joined
// pointwise; afterwards, the dead (fail-path) condition is absorbed into the
// reach condition whenever that yields a strictly simpler proposition.
// Absorption is sound — rows satisfying the dead condition are rejected by
// the originating fail's own constraint, so widening never changes the set of
// accepted traces.
// We do it only when it is beneficial: where a fail's successor
// is a sibling branch rather than a join for example.
func (p Path[W]) Join(st Path[W]) Path[W] {
	var (
		condition = p.condition.Or(st.condition)
		dead      = p.dead.Or(st.dead)
	)
	//
	if widened := condition.Or(dead); simpler(widened, condition) {
		condition = widened
	}
	//
	return Path[W]{condition, dead}
}

// simpler reports whether the left condition is strictly simpler than the
// right one, comparing (in order):
// - the number of conjuncts
// - the total number of atoms.
func simpler(lhs, rhs BranchCondition) bool {
	var ln, rn = len(lhs.Conjuncts()), len(rhs.Conjuncts())
	//
	if ln != rn {
		return ln < rn
	}
	//
	return atomCount(lhs) < atomCount(rhs)
}

// atomCount returns the total number of atoms across all conjuncts of the
// given condition.
func atomCount(p BranchCondition) int {
	var n int
	//
	for _, c := range p.Conjuncts() {
		n += len(c.Atoms())
	}
	//
	return n
}

// String implementation for State interface
func (p Path[W]) String(mapping register.Map) string {
	return p.condition.String(func(rid BranchId) string {
		var name = mapping.Register(rid.Id).Name()
		//
		if rid.Forwarding {
			return name
		}
		//
		return fmt.Sprintf("'%s", name)
	})
}
