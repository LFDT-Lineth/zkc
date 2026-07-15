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

package split

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Switch splits a switch instruction.
func Switch[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.Switch[W],
) []Bytecode[W] {
	var limbs = ApplyLimbsMap(mapping, insn.Source)
	//
	if len(limbs) == 1 {
		return []Bytecode[W]{bytecode.MultiwaySkip(limbs[0], insn.Cases)}
	}
	//
	panic("support splitting switch bytecodes")
}
