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
package parser_test

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
)

func Test_ParseStatementCostAnnotation_MapsWrapperAndBody(t *testing.T) {
	src := source.NewSourceFile("cost_source_maps.zkc", []byte(`
fn main() {
    var x:u8 = 0

    #[cost:leaf]
    x = 1

    #[cost:block]
    if x == 1 {
        x = 2
    } else {
        x = 3
    }
}
`))

	item, errors := parser.Parse(src)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", errors)
	}

	fn, ok := item.Components[0].(*decl.UnresolvedFunction)
	if !ok {
		t.Fatalf("expected unresolved function, got %T", item.Components[0])
	}

	for _, index := range []int{1, 2} {
		cost, ok := fn.Code[index].(*stmt.Cost[symbol.Unresolved])
		if !ok {
			t.Fatalf("expected cost annotation at code index %d, got %T", index, fn.Code[index])
		}

		if !item.SourceMap.Has(cost) {
			t.Fatalf("expected source map for cost wrapper %q", cost.Label)
		}

		if !item.SourceMap.Has(cost.Body) {
			t.Fatalf("expected source map for body of cost wrapper %q", cost.Label)
		}
	}
}
