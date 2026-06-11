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
package bytecode

import (
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Cat concatenates source register bits and stores the result across targets.
type Cat struct {
	// Targets receive the concatenated value, least-significant limb first.
	Targets []Reg
	// Sources are concatenated with Sources[0] in the least-significant bits.
	Sources []Reg
}

func (p *Cat) String(mapping SystemMap) string {
	var builder strings.Builder
	//
	builder.WriteString("cat ")
	builder.WriteString(registersToString(array.Reverse(p.Targets), mapping, "::"))
	builder.WriteString(" = ")
	builder.WriteString(registersToString(array.Reverse(p.Sources), mapping, "::"))
	//
	return builder.String()
}

// Codes implementation for Bytecode interface.
func (p *Cat) Codes(_ uint32) []uint32 {
	return encodeCat(p.Targets, p.Sources)
}

func decodeCat[W word.Word[W]](pc uint32, codes []uint32) (Bytecode[W], uint32) {
	var (
		tIter, sIter, n = decodeCatOperands(pc, codes)
		targets         = OpIterToArray[uint16](tIter)
		sources         = OpIterToArray[uint16](sIter)
	)
	//
	return &Cat{targets, sources}, n
}

// ============================================================================
// CAT instruction. Format of this instruction is:
//
//	31                                0
//
// +--------+--------+--------+--------+
// |   n/a  |  nsrc  | ntgt   | opcode |
// +--------+--------+--------+--------+
// | tgt3   | tgt2   | tgt1   | tgt0   |
// +--------+--------+--------+--------+
// | ... packed source registers ...    |
// +------------------------------------+
//
// The first source and target are the least-significant limbs.
// ============================================================================

func encodeCat(targets []Reg, sources []Reg) []uint32 {
	if len(targets) == 0 || len(sources) == 0 || len(targets) >= 256 || len(sources) >= 256 {
		panic("wide concat instructions not supported")
	}
	//
	var (
		nsrc  = uint32(len(sources)) << 16
		ntgt  = uint32(len(targets)) << 8
		codes = []uint32{nsrc | ntgt | CAT}
		bytes = append(regsAsBytes(targets), regsAsBytes(sources)...)
	)
	//
	return append(codes, packRegsIntoCodes(bytes)...)
}

func decodeCatOperands(pc uint32, codes []uint32) (targets, sources Op8Iter, n uint32) {
	var (
		ntargets = uint((codes[pc] >> 8) & 0xff)
		nsources = uint((codes[pc] >> 16) & 0xff)
	)
	//
	targets = NewOp8Iter(0, ntargets, codes[pc+1:])
	sources = NewOp8Iter(ntargets, nsources, codes[pc+1:])
	n = 1 + nCodesPackedSmall(ntargets+nsources)
	//
	return
}

// Concat constructs a bit-concatenation bytecode.
func Concat(targets []register.Id, sources []register.Id) *Cat {
	return &Cat{asRegs(targets...), asRegs(sources...)}
}
