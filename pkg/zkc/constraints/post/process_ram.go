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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// ProcessReadWriteMemory performs post-processing on a RAM trace.
func ProcessReadWriteMemory[W Word[W], F Element[F]](m vm.Memory[W]) rtrace.ArrayModule[F] {
	var regs = array.Map(m.Registers(), toRtraceRegisterLegacy)
	// TODO: flesh me out :)
	return rtrace.NewArrayModule[F](m.Name(), regs)
}
