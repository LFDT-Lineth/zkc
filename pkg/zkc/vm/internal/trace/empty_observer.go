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
package trace

import (
	"github.com/consensys/go-corset/pkg/zkc/vm/instruction"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
)

// EmptyObserver does nothing
type EmptyObserver[W machine.BaseWord[W], I instruction.Instruction, M machine.Core[W, I]] struct {
}

// Initialise implementation for Observer interface
func (p EmptyObserver[W, I, M]) Initialise(machine M) {
	// do nothing
}

// PreExecution implementation for Observer interface
func (p EmptyObserver[W, I, M]) PreExecution(machine M) {
	// do nothing
}

// PostExecution implementation for Observer interface
func (p EmptyObserver[W, I, M]) PostExecution(machine M) {
	// do nothing
}
