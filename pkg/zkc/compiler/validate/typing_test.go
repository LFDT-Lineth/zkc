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
package validate_test

import (
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
)

func Test_Typing_AllowsLoweredCopiesOfCostLabel(t *testing.T) {
	src := source.NewSourceFile("cost_label_lowered.zkc", []byte(`
fn main() {
    var x:u8 = 0

    #[cost:branch]
    if x == 0 {
        x = 1
    } else {
        x = 2
    }
}
`))

	_, _, errors := compiler.Compile(field.KOALABEAR_16, *src)
	for _, err := range errors {
		if strings.Contains(err.Message(), "duplicate cost label") {
			t.Fatalf("expected lowered copies of a source cost label to be allowed, got %v", err)
		}
	}
}

func Test_Typing_RejectsDuplicateCostLabelsAtDifferentSourceSites(t *testing.T) {
	src := source.NewSourceFile("cost_label_duplicate.zkc", []byte(`
fn main() {
    var x:u8 = 0

    #[cost:duplicate]
    x = 1

    #[cost:duplicate]
    x = 2
}
`))

	_, _, errors := compiler.Compile(field.KOALABEAR_16, *src)
	for _, err := range errors {
		if strings.Contains(err.Message(), `duplicate cost label "duplicate"`) {
			return
		}
	}

	t.Fatalf("expected duplicate cost label error, got %v", errors)
}
