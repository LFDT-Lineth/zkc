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
package bytecode

import (
	"github.com/consensys/go-corset/pkg/util/collection/heap"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/word"
)

type Interpreter[W word.Word[W]] struct {
	// Program counter position for the bytecode interpreter
	pc uint32
	// Frame pointer position for the bytecode interpreter
	fp       uint32
	bytecode []uint32
	// word stack
	wordStack heap.Heap[W]
	// call stack
	callStack heap.Heap[uint32]
}
