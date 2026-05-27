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

import "github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"

// Observer is a generic interface for extract information before and after an
// execution step of the VM.  For example, to generate debugging information.
type Observer[W any, M machine.Core[W]] interface {
	Initialise(machine M)
	// PreExecution is called directly before each instruction is executed
	PreExecution(machine M)
	// PostExecution is called directly after each instruction is executed.
	PostExecution(machine M)
}
