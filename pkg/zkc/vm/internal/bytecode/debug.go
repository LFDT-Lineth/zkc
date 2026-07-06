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
	Chunks []FormattedChunk
	// Source registers used for displaying chunks
	Sources []RegisterVector
}

// Uses implementation for Bytecode interface.  A debug reads the registers
// referenced by its formatted arguments.
func (p *Debug) Uses() []RegisterId {
	var uses []RegisterId
	//
	for _, s := range p.Sources {
		uses = append(uses, s.Registers()...)
	}
	//
	return uses
}

// Definitions implementation for Bytecode interface.
func (p *Debug) Definitions() []RegisterId {
	return nil
}

// Validate implementation for Bytecode interface.
func (p *Debug) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *Debug) String(env Environment) string {
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
	return fmt.Sprintf("debug %s", tBuilder.String())
}
