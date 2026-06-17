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

// Debug carries a formatted-print (printf) specification so the interpreter can
// reproduce the reference machine's debug output.  The chunks themselves are
// held in the program's debug side-table (they cannot be encoded inline as
// uint32 words); Index identifies this site's entry there and is the only thing
// packed into the bytecode stream (see Codes).
type Debug struct {
	// Chunks is the formatted-print specification: literal text interleaved with
	// argument formats.  Carried through compilation into the program's debug
	// side-table rather than encoded inline.
	Chunks []base.FormattedChunk
	// Index identifies this debug site's chunk-set within the program's debug
	// side-table.  Assigned during encoding (see indexDebugBytecodes).
	Index uint32
}

func (p *Debug) String(mapping SystemMap) string {
	return "debug"
}

// Codes implementation for Bytecode interface.  The chunk-set index is packed
// above the opcode so the single DEBUG word both identifies the instruction and
// locates its formatted-print specification within the program's debug
// side-table.
func (p *Debug) Codes(_ uint32) []uint32 {
	return []uint32{(p.Index << 8) | DEBUG}
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *Debug) Clone() Patched {
	return &Debug{slices.Clone(p.Chunks), p.Index}
}

// NewDebug constructs a debug bytecode carrying the given formatted-print
// chunks.  Its side-table index is assigned later, during encoding.
func NewDebug(chunks []base.FormattedChunk) *Debug {
	return &Debug{Chunks: chunks}
}
