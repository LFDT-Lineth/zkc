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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// CheckCast splits a CheckCast bytecode into one or more CheckCasts.
func CheckCast[W word.Word[W]](mapping descriptor.LimbsMap[W], insn *bytecode.CheckCast[W]) []Bytecode[W] {
	var (
		bytecodes []Bytecode[W]
		bitwidth  = uint(insn.Bitwidth)
	)
	// Split cast
	for _, r := range ApplyLimbsMapReversed(mapping, insn.Target) {
		var (
			ith            = mapping.Limb(r)
			declared_width = ith.Bitwidth().Unwrap()
			cast_width     = min(bitwidth, declared_width)
		)
		// Add cast for registers whose casted width is below its
		// declared width.
		if cast_width < declared_width {
			bytecodes = append(bytecodes, bytecode.NewCheckCast[W](r, util.Cast[uint16](cast_width)))
		}
		// Decrease remaining bitwidth
		bitwidth -= cast_width
	}
	//
	return bytecodes
}
