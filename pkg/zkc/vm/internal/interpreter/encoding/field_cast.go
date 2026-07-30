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

// ============================================================================
// UINT_TO_FIELD / FIELD_TO_UINT instruction.  Format of these instructions is
// the register-list layout shared with CAT (see encodeRegisterLists):
//
//	31                                0
//
// +--------+--------+--------+--------+
// |  nsrc  |  ntgt  |  n/a   | opcode |
// +--------+--------+--------+--------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...   |
// +-----------------------------------+
//
// For UINT_TO_FIELD there is a single target (the field register), with the
// sources being the uint limbs (least significant first); conversely, for
// FIELD_TO_UINT there is a single source (the field register), with the
// targets being the uint limbs.  The wide form retains the header but packs
// the (now u16) target and source registers two per word:
//
// +--------+--------+--------+--------+
// |  nsrc  |  ntgt  |  n/a   | opcode |
// +--------+--------+--------+--------+
// |       tgt1      |       tgt0      |
// +-----------------+-----------------+
// | ... packed source registers ...   |
// +-----------------------------------+
// ============================================================================

// UintToField encodes a uint-to-field conversion.
func UintToField[W word.Word[W]](p *bytecode.UintToField[W]) []uint32 {
	return encodeRegisterLists(UINT_TO_FIELD, []RegisterId{p.Target}, p.Source)
}

// FieldToUint encodes a field-to-uint conversion.
func FieldToUint[W word.Word[W]](p *bytecode.FieldToUint[W]) []uint32 {
	return encodeRegisterLists(FIELD_TO_UINT, p.Target, []RegisterId{p.Source})
}
