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
package stmt

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

func Test_UnwrapCost_UnwrapsNestedAnnotations(t *testing.T) {
	gotoStmt := &Goto[symbol.Resolved]{Target: 7}
	wrapped := &Cost[symbol.Resolved]{
		Label: "outer",
		Body: &Cost[symbol.Resolved]{
			Label: "inner",
			Body:  gotoStmt,
		},
	}

	if unwrapped := UnwrapCost[symbol.Resolved](wrapped); unwrapped != gotoStmt {
		t.Fatalf("expected nested cost annotations to unwrap to the inner statement")
	}
}
