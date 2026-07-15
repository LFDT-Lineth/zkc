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
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Fail aborts execution with a "machine panic".  It optionally carries a
// formatted-message specification (the chunks) describing the error; this is
// held in the program's chunk side-table exactly as Debug's is, and Index
// identifies this site's entry there — the only thing packed into the bytecode
// stream (see Codes).  When there are no chunks the panic carries no message.
type Fail[W word.Word[W]] struct {
	// Chunks is the optional formatted-message specification: literal text
	// interleaved with argument formats.  Carried through compilation into the
	// program's chunk side-table rather than encoded inline.
	Chunks []FormattedChunk
	// Source registers used for displaying chunks
	Sources []RegisterVector
}

// Uses implementation for Bytecode interface.  A fail reads the registers
// referenced by its formatted (error message) arguments.
func (p *Fail[W]) Uses() []RegisterId {
	var uses []RegisterId
	//
	for _, s := range p.Sources {
		uses = append(uses, s.Registers()...)
	}
	//
	return uses
}

// Definitions implementation for Bytecode interface.
func (p *Fail[W]) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *Fail[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	return nil
}

func (p *Fail[W]) String(env Environment[W]) string {
	var (
		tBuilder strings.Builder
	)
	//
	tBuilder.WriteString("\"")
	//
	for _, c := range p.Chunks {
		tBuilder.WriteString(util.EscapeFormattedText(c.Text))
		//
		if c.Format.HasFormat() {
			tBuilder.WriteString(c.Format.String())
		}
	}
	//
	tBuilder.WriteString("\"")
	//
	for _, s := range p.Sources {
		tBuilder.WriteString(",")
		tBuilder.WriteString(RegisterVectorToString(s, env))
	}
	//
	return fmt.Sprintf("fail %s", tBuilder.String())
}

// FormattedChunk pairs a piece of literal text with the format used to render
// an accompanying value, as used to build fail/debug messages.
type FormattedChunk struct {
	Text   string
	Format util.Format
}

// Cmp compares two formatted chunks lexicographically, first by text and then
// by format.
func (p FormattedChunk) Cmp(o FormattedChunk) int {
	if c := strings.Compare(p.Text, o.Text); c != 0 {
		return c
	}
	//
	return strings.Compare(p.Format.String(), o.Format.String())
}
