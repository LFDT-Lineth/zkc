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
package post

import (
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
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

// ToRegisters converts an array of register descriptors into an array of scheme
// registers.
func toRtraceRegisters(registers []register.Register) []rtrace.Register {
	var regs = make([]rtrace.Register, len(registers))
	//
	for i, r := range registers {
		var (
			bitwidth util.Option[[]uint]
		)
		// Determine bitwidth (if applicable)
		if !r.IsNative() {
			bitwidth = util.Some([]uint{r.Width()})
		}
		//
		regs[i] = rtrace.NewRegister(r.Name(), bitwidth)
	}
	//
	return regs
}
