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
	"slices"
)

// ROM_READ representing reading from a read-only memory.
var ROM_READ = RwMode{0}

// SROM_READ representing reading from a (static) read-only memory.
var SROM_READ = RwMode{1}

// WOM_WRITE representing writing to a write-once memory.
var WOM_WRITE = RwMode{2}

// SRAM_READ representing reading from a (small) random-access memory.
var SRAM_READ = RwMode{3}

// SRAM_WRITE representing write to a (small) random-access memory.
var SRAM_WRITE = RwMode{4}

// PRAM_READ representing reading from a (paged) random-access memory.
var PRAM_READ = RwMode{5}

// PRAM_WRITE representing write to a (paged) random-access memory.
var PRAM_WRITE = RwMode{6}

// RwMode determines whether what kind of memory is being operated on (e.g. ROM
// or RAM, etc) and what operation is being performed (i.e. READ or WRITE).
type RwMode struct {
	tag uint8
}

// Tag gets the underlying tag for this enum.
func (p RwMode) Tag() uint8 {
	return p.tag
}

func (p RwMode) prefix() string {
	switch p {
	case SROM_READ:
		return "rdo"
	case ROM_READ:
		return "rdr"
	case WOM_WRITE:
		return "wrw"
	case SRAM_READ:
		return "rds"
	case SRAM_WRITE:
		return "wrs"
	case PRAM_READ:
		return "rdp"
	case PRAM_WRITE:
		return "wrp"
	default:
		panic("invalid read/write mode")
	}
}

// ReadWrite instruction captures memory read/writes.
type ReadWrite struct {
	// RwMode determines whether this is a read or write operation and,
	// furthermore, what kind of memory is being accessed.
	Mode RwMode
	// Identifies the memory being read or written.
	Id uint16
	// Address lines used to determine which data row to read.
	Address []RegisterId
	// Data lines identify where the data row is written.
	Data []RegisterId
}

// Clone implementation for Bytecode / Patched interfaces.
func (p *ReadWrite) Clone() Patched {
	return &ReadWrite{p.Mode, p.Id, slices.Clone(p.Address), slices.Clone(p.Data)}
}

// isWrite reports whether this is a memory write (rather than a read).
func (p *ReadWrite) isWrite() bool {
	switch p.Mode {
	case WOM_WRITE, SRAM_WRITE, PRAM_WRITE:
		return true
	default:
		return false
	}
}

// Uses implementation for Bytecode interface.  A read uses only its address
// registers, whereas a write uses both the address and data registers.
func (p *ReadWrite) Uses() []RegisterId {
	if p.isWrite() {
		return append(slices.Clone(p.Address), p.Data...)
	}
	//
	return p.Address
}

// Definitions implementation for Bytecode interface.  A read defines its data
// registers, whereas a write defines nothing in the surrounding frame.
func (p *ReadWrite) Definitions() []RegisterId {
	if p.isWrite() {
		return nil
	}
	//
	return p.Data
}

// Validate implementation for Bytecode interface.
func (p *ReadWrite) Validate(_ uint, _ FieldConfig, _ Environment) []error {
	return nil
}

func (p *ReadWrite) String(env Environment) string {
	var (
		name    = "???"
		address = RegistersToString(p.Address, env, ",")
		data    = RegistersToString(p.Data, env, ",")
		prefix  = p.Mode.prefix()
	)
	//
	if env != nil {
		name = env.Module(p.Id).Name()
	}
	//
	switch p.Mode {
	case SROM_READ, ROM_READ, SRAM_READ, PRAM_READ:
		return fmt.Sprintf("%s %s = %s[%s]", prefix, data, name, address)
	case WOM_WRITE, SRAM_WRITE, PRAM_WRITE:
		return fmt.Sprintf("%s %s[%s] = %s", prefix, name, address, data)
	default:
		panic("unknown read/write mode")
	}
}
