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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// InitReadWriteMemory initialises a trace module for a RandomAccessMemory.
func initReadWriteMemory[W Word[W], F Element[F], M ModuleBuilder[F, M]](m vm.Memory[W]) (module M) {
	var regs = array.Map(m.Registers(), toRtraceRegister)
	//Done
	return module.Initialise(m.Name(), regs)
}

// traceReadWriteMemory materialises the trace rows for a given RandomAccessMemory.
func traceReadWriteMemory[W Word[W], F Element[F]](m vm.RuntimeMemory[W], module Module[F]) {
	// TODO: flesh me out :)
}
