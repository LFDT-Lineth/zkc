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
package encoding

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FieldCast encodes a field-cast bytecode.  Its wire format is identical to CAT
// (target vector then source vector), so it reuses the shared encoder and the
// CAST opcode; the operands are decoded with DecodeCatOperands.
func FieldCast[W word.Word[W]](p *bytecode.FieldCast[W]) []uint32 {
	return encodeCatLike(CAST, p.Target, p.Source)
}

// DecodeFieldCast decodes a field-cast instruction at the given program counter.
func DecodeFieldCast[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		tIter, sIter, n = DecodeCatOperands(pc, codes)
		targets         = OpIterToArray[uint16](tIter)
		sources         = OpIterToArray[uint16](sIter)
	)
	//
	return &bytecode.FieldCast[W]{Target: targets, Source: sources}, n
}
