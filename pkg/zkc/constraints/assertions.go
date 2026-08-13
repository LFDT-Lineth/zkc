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
package constraints

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/util/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// collectAssertions scans a bytecode vector for Fail bytecodes and returns
// the reach condition of each.  Any row satisfying one of these is rejected
// by that fail's own constraint regardless of anything else, so each is a
// "don't care" region that may soundly be folded into (i.e. widen) any other
// write's guard condition within the same vector — see applyAssertions.
func collectAssertions[W vm.Word[W]](codes []vm.Bytecode[W],
	branchTable dfa.Result[dfa.Path[W]]) []dfa.BranchCondition {
	var assertions []dfa.BranchCondition
	//
	for i, c := range codes {
		if _, ok := c.(*vm.BytecodeFail[W]); ok {
			assertions = append(assertions, branchTable.StateOf(uint(i)).Condition())
		}
	}
	//
	return assertions
}

// applyAssertions widens condition by each given assertion in turn, keeping
// the widened form whenever doing so yields a strictly simpler proposition.
// Widening is always sound here: every row satisfying an assertion is
// rejected by its originating fail's own constraint regardless, so folding
// it into some other guard can never cause an invalid trace to be wrongly
// accepted.
func applyAssertions(condition dfa.BranchCondition, assertions []dfa.BranchCondition) dfa.BranchCondition {
	for _, assertion := range assertions {
		if trial := condition.Or(assertion); isSimplerCondition(trial, condition) {
			condition = trial
		}
	}
	//
	return condition
}

// isSimplerCondition reports whether the left condition is strictly simpler
// than the right one, comparing (in order): the number of conjuncts, then
// the total number of atoms.
func isSimplerCondition(lhs, rhs dfa.BranchCondition) bool {
	var ln, rn = len(lhs.Conjuncts()), len(rhs.Conjuncts())
	//
	if ln != rn {
		return ln < rn
	}
	//
	return atomCountOfCondition(lhs) < atomCountOfCondition(rhs)
}

// atomCountOfCondition returns the total number of atoms across all
// conjuncts of the given condition.
func atomCountOfCondition(p dfa.BranchCondition) int {
	var n int
	//
	for _, c := range p.Conjuncts() {
		n += len(c.Atoms())
	}
	//
	return n
}
