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
	"fmt"

	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

// WordToWordMachine transforms a machine operating over a given word type (W1)
// into an identical machine which operates over a different word type (W2).
// Generally speaking, we are going from a larger word (e.g. word.Uint) to a
// smaller word (e.g. word.Uint64).  This function will panic if it encounters a
// register or constant which exceeds the bandwidth of the given word.
func WordToWordMachine[W1 word.Word[W1], W2 word.Word[W2]](wm *machine.Word[W1]) (fm *machine.Word[W2]) {
	var (
		w1           W1
		w2           W2
		w1_bandwidth = w1.Bandwidth()
		w2_bandwidth = w2.Bandwidth()
	)
	panic(fmt.Sprintf("todo: lower from machine over u%d to one over u%d", w1_bandwidth, w2_bandwidth))
}
