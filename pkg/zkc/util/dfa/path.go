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
	return Path[W]{logical.Truth[BranchId, BranchEquality](true)}
}

// Path adapts a branch condition to be an instance of State.
type Path[W util.BigInter] struct {
	condition BranchCondition
}

// Condition returns the underlying branch condition
func (p Path[W]) Condition() BranchCondition {
	return p.condition
}

// Equals extends the current path with a new constraint that two variables are equal.
func (p Path[W]) Equals(lhs, rhs BranchId) Path[W] {
	var prop = logical.NewProposition(logical.Equals(lhs, rhs))
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop}
	}
	//
	return Path[W]{p.condition.And(prop)}
}

// NotEquals extends the current path with a new constraint that two variables are not equal.
func (p Path[W]) NotEquals(lhs, rhs BranchId) Path[W] {
	var prop = logical.NewProposition(logical.NotEquals(lhs, rhs))
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop}
	}
	//
	return Path[W]{p.condition.And(prop)}
}

// EqualsConst extends the current path with a new constraint that a given variable equals a given constant.
func (p Path[W]) EqualsConst(lhs BranchId, rhs big.Int) Path[W] {
	var (
		prop = logical.NewProposition(logical.EqualsConst(lhs, rhs))
	)
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop}
	}
	//
	return Path[W]{p.condition.And(prop)}
}

// NotEqualsConst extends the current path with a new constraint that a given variable does not equal a given constant.
func (p Path[W]) NotEqualsConst(lhs BranchId, rhs big.Int) Path[W] {
	var (
		prop = logical.NewProposition(logical.NotEqualsConst(lhs, rhs))
	)
	//
	if len(p.condition.Conjuncts()) == 0 {
		return Path[W]{prop}
	}
	//
	return Path[W]{p.condition.And(prop)}
}

// Join implementation for State interface
func (p Path[W]) Join(st Path[W]) Path[W] {
	return Path[W]{
		p.condition.Or(st.condition),
	}
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
