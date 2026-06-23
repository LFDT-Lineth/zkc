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
	"encoding/gob"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
)

// Ref abstracts a complete (i.e. global) register identifier.
type Ref = trace.ColumnRef

// NewRef constructs a new register reference from the given module and
// register identifiers.
func NewRef(mid trace.ModuleId, id Id) Ref {
	return trace.NewColumnRef(mid, id)
}

// Refs is a "registers reference" which abstracts a set of registers in a given
// module.
type Refs struct {
	mid  trace.ModuleId
	regs []Id
}

// NewRefs constructs a new registers reference for a given module and
// identifier set.
func NewRefs(mid trace.ModuleId, ids ...Id) Refs {
	return Refs{mid, ids}
}

// Module returns the module identifier of this registers reference.
func (p *Refs) Module() trace.ModuleId {
	return p.mid
}

// Registers returns the register identifiers for the referenced registers.
func (p *Refs) Registers() []Id {
	return p.regs
}

// Apply a given mapping to this set of registers.
func (p *Refs) Apply(mapping LimbsMap) Refs {
	var nids []Id

	for _, ith := range p.regs {
		nids = append(nids, mapping.LimbIds(ith)...)
	}

	return NewRefs(p.mid, nids...)
}

// AsRefArray converts a register refs array into an array of register ref.
func AsRefArray(p Refs) []Ref {
	var nrefs = make([]Ref, len(p.regs))
	//
	for i, r := range p.regs {
		nrefs[i] = NewRef(p.mid, r)
	}
	//
	return nrefs
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (p Refs) GobEncode() (data []byte, err error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	//
	if err = gobEncoder.Encode(&p.mid); err != nil {
		return nil, err
	}
	//
	if err = gobEncoder.Encode(&p.regs); err != nil {
		return nil, err
	}
	// Done
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *Refs) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.mid); err != nil {
		return err
	}
	//
	if err := gobDecoder.Decode(&p.regs); err != nil {
		return err
	}
	// Success!
	return nil
}
