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
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints/post"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

type (
	// Word provides a useful alias
	Word[W any] = vm.Word[W]
	// Element provides a useful alias
	Element[F any] = field.Element[F]
	// Function provides a useful alias
	Function = vm.WordFunction
	// Memory provides a useful alias
	Memory[W Word[W]] = vm.Memory[W]
)

// Post process the recorded state for a given module.  For memories, this means
// transcribing the state into a suitable trace format with auxiliary registers
// as required (e.g. for selector bits, etc).  For functions, this means
// transcribing each state generated for the function during execution.
func postProcess[W Word[W], F Element[F]](m vm.Module, states []vm.State[W]) rtrace.ArrayModule[F] {
	switch m := m.(type) {
	case *Function:
		if m.IsAtomic() {
			return post.ProcessOneLineFunction[W, F](*m, states)
		}
		//
		return post.ProcessMultiLineFunction[W, F](*m, states)
	case vm.Memory[W]:
		switch {
		case m.IsStatic():
			return rtrace.NewArrayModule[F](m.Name(), nil)
		case m.IsReadOnly() && !m.IsStatic():
			return post.ProcessAccessOnceMemory[W, F](m)
		case m.IsWriteOnly():
			return post.ProcessAccessOnceMemory[W, F](m)
		case m.IsReadWrite():
			return post.ProcessReadWriteMemory[W, F](m)
		}
	}
	//
	panic(fmt.Sprintf("unsupported module \"%s\" encountered", m.Name()))
}
