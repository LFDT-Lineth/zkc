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
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/base"
)

// Fail aborts execution with a "machine panic".  It optionally carries a
// formatted-message specification (the chunks) describing the error; this is
// held in the program's chunk side-table exactly as Debug's is, and Index
// identifies this site's entry there — the only thing packed into the bytecode
// stream (see Codes).  When there are no chunks the panic carries no message.
type Fail struct {
	// Chunks is the optional formatted-message specification: literal text
	// interleaved with argument formats.  Carried through compilation into the
	// program's chunk side-table rather than encoded inline.
	Chunks []base.FormattedChunk
	// Index identifies this fail site's chunk-set within the program's chunk
	// side-table.  Assigned during encoding (see indexFormattedBytecodes).
	Index uint32
}

func (p *Fail) String(_ SystemMap) string {
	return "fail"
}

// Codes implementation for Bytecode interface.  The chunk-set index is packed
// above the opcode so the single FAIL word both identifies the instruction and
// locates its (optional) formatted-message specification in the side-table.
// FAIL is opcode 0, so it occupies the low bits implicitly (the index sits
// above it, mirroring Debug.Codes' "(Index << 8) | DEBUG").
func (p *Fail) Codes(_ uint32) []uint32 {
	return []uint32{p.Index << 8}
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *Fail) Clone() Patched {
	return &Fail{slices.Clone(p.Chunks), p.Index}
}

// NewFail constructs a fail bytecode carrying the given (possibly empty)
// formatted-message chunks.  Its side-table index is assigned during encoding.
func NewFail(chunks []base.FormattedChunk) *Fail {
	return &Fail{Chunks: chunks}
}
