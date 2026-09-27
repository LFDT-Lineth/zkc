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
package transform

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Test_FlattenableArgs_SkipsPathDefiniteWrite constructs a vector where a call
// argument register is definitely written on every path reaching the call, and
// is written again only on a *mutually exclusive* branch (never on the same
// path as the call).  Prior to the fix for issue #2244, flattenableArgs scanned
// the vector's bytecodes linearly and flagged this argument for an unnecessary
// snapshot, since it did not distinguish which later writes actually share a
// path with the call.  The vector is:
//
//	skip_if cond != 0, 3 ; r = 0 ; t = call(r) ; ret ; r = 1 ; ret
//
// On the path reaching the call, r is definitely 0 before the call and the
// later write "r = 1" lies on the other (skipped) branch, so no snapshot is
// needed.
func Test_FlattenableArgs_SkipsPathDefiniteWrite(t *testing.T) {
	const (
		cond bytecode.RegisterId = 0
		r    bytecode.RegisterId = 1
		tmp  bytecode.RegisterId = 2
	)

	var zero word.Uint

	vec := bytecode.NewVector[word.Uint](
		bytecode.NewSkipIf[word.Uint](bytecode.CONDITION_NEQ, 3,
			bytecode.NewRegisterVector(cond), bytecode.NewConstantOperand[word.Uint](zero)),
		bytecode.LoadConst[word.Uint](r, zero),
		bytecode.CallFun[word.Uint](0, []bytecode.RegisterId{r}, []bytecode.RegisterId{tmp}),
		&bytecode.Ret[word.Uint]{},
		bytecode.LoadConst[word.Uint](r, zero),
		&bytecode.Ret[word.Uint]{},
	)

	snapshot := flattenableArgs(&vec)

	flags, ok := snapshot[2]
	if !ok {
		t.Fatalf("expected call at index 2 to be recorded in the snapshot map")
	}

	if flags[0] {
		t.Errorf("argument r should not require snapshotting: it is definitely assigned before the call, and the later write to r lies on a mutually exclusive path")
	}
}
