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
package register

import (
	"bytes"
	"cmp"
	"encoding/gob"
)

// Type captures the type of a given register, such as whether it
// represents an input column, and output column or a computed register, etc.
type Type struct {
	kind uint8
}

// Cmp implementation for register types, where inputs come first, followed by
// outputs then computed registers.
func (p Type) Cmp(o Type) int {
	return cmp.Compare(p.kind, o.kind)
}

var (
	// INPUT_REGISTER signals a register used for holding the input values of a
	// function.
	INPUT_REGISTER = Type{uint8(0)}
	// OUTPUT_REGISTER signals a register used for holding the output values of
	// a function.
	OUTPUT_REGISTER = Type{uint8(1)}
	// COMPUTED_REGISTER signals a register whose values are computed from one
	// (or more) assignments during trace expansion.
	COMPUTED_REGISTER = Type{uint8(2)}
)

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (p Type) GobEncode() (data []byte, err error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	//
	if err = gobEncoder.Encode(&p.kind); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *Type) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.kind); err != nil {
		return err
	}
	// Success!
	return nil
}
