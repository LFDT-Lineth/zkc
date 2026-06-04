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
package corset

import (
	"encoding/gob"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// SourceMap is a binary file attribute which provides debugging
// information about the relationship between registers and source-level
// columns.  This is used, for example, within the inspector.
type SourceMap struct {
	// Root module correspond to the top-level HIR modules.  Thus, indicates into
	// this table correspond to HIR module indices, etc.
	Root SourceModule
	// Enumerations are custom types for display.  For example, we might want to
	// display opcodes as ADD, MUl, SUB, etc.
	Enumerations []Enumeration
}

// AttributeName returns the name of the binary file attribute that this will
// generate.  This is used, for example, when listing attributes contained
// within a binary file.
func (p *SourceMap) AttributeName() string {
	return "CorsetSourceMap"
}

// Flatten modules in this tree matching a given criteria
func (p *SourceMap) Flatten(predicate func(*SourceModule) bool) []SourceModule {
	return p.Root.Flatten(predicate)
}

// SubstituteConstants updates the recorded value of constants within this
// source map.  This is typically done in conjunction with a substitution
// through the schema, in order to keep them both in sync.
func (p *SourceMap) SubstituteConstants(mapping map[string]big.Int) {
	path := file.NewAbsolutePath()
	p.Root.SubstituteConstants(path, mapping)
}

// Enumeration is a mapping from field elements to explicitly given names.  For
// example, mapping opcode bytes to their names.
type Enumeration map[uint64]string

// SourceModule represents an entity at the source-level which groups together
// related columns.  Modules can be either concrete (in which case they
// correspond with HIR modules) or virtual (in which case they are encoded
// within an HIR module).
type SourceModule struct {
	// Name of this submodule.
	Name string
	// Public indicates whether or not this module is externally visible or not.
	Public bool
	// Synthetic indicates whether or not this module was automatically
	// generated or not.
	Synthetic bool
	// Virtual indicates whether or not this is a "virtual" module.  That is, a
	// module which is artificially embedded in some outer (concrete) module.
	Virtual bool
	// Selector determines when this (sub)module is active.  Specifically, when
	// it evaluates to a non-zero value the module is active.
	Selector util.Option[string]
	// Submodules identifies any (virtual) submodules contained within this.
	// Currently, perspectives are the only form of submodule currently
	// supported.
	Submodules []SourceModule
	// Columns identifies any columns defined in this module.  Observe that
	// columns across modules are mapped to registers in a many-to-one fashion.
	Columns []SourceColumn
	// Constants identifiers any constants defined in this module.
	Constants []SourceConstant
}

// Submodule returns the matching submodule with the given name, or nil if no
// such module exists.
func (p *SourceModule) Submodule(name string) *SourceModule {
	for _, m := range p.Submodules {
		if m.Name == name {
			return &m
		}
	}
	//
	return nil
}

// Registers returns the set of underlying registers declared in this module.
// This only makes sense for non-virtual modules, and essentially includes all
// columns declare in this module or any of its virtual children.
func (p *SourceModule) Registers(nModules uint) []SourceColumn {
	var visited bit.Set
	return determineRegisters(*p, nModules, &visited)
}

// Flatten modules in this tree either including (or excluding) virtual
// modules.
func (p *SourceModule) Flatten(predicate func(*SourceModule) bool) []SourceModule {
	var modules []SourceModule

	if predicate(p) {
		modules = append(modules, *p)
		for _, child := range p.Submodules {
			modules = append(modules, child.Flatten(predicate)...)
		}
	}

	return modules
}

// SubstituteConstants updates the recorded value of constants within this
// source map.  This is typically done in conjunction with a substitution
// through the schema, in order to keep them both in sync.
func (p *SourceModule) SubstituteConstants(path file.Path, mapping map[string]big.Int) {
	// check all local constants
	for i := range p.Constants {
		ith := &p.Constants[i]
		if ith.Extern {
			ith_name := path.Extend(ith.Name).String()
			//
			if nval, ok := mapping[ith_name]; ok {
				ith.Value = nval
			}
		}
	}
	// recurse submodules
	for i := range p.Submodules {
		ith := &p.Submodules[i]
		ith.SubstituteConstants(*path.Extend(ith.Name), mapping)
	}
}

// SourceColumn represents a source-level column which is mapped to a given MIR
// register.  Observe that multiplie source-level columns can be mapped to the
// same register.
type SourceColumn struct {
	Name string
	// Length Multiplier of source-level column.
	Multiplier uint
	// Underlying bitwidth of the source-level column.
	Bitwidth uint
	// Provability requirement for source-level column.
	MustProve bool
	// Determines whether this is a Computed column.
	Computed bool
	// Display modifier for column. Here 0-256 are reserved, and values >256 are
	// entries in Enumerations map.  More specifically, 0=hex, 1=dec, 2=bytes.
	Display uint
	// Register in the generate schema to which this Corset register is mapped.
	// Observe that this has to be a reference, rather than just an ID.  This is
	// because a column in a given corset module may map into a different module
	// in the underlying schema (i.e. for interleavings).
	Register register.Ref
}

// DISPLAY_HEX shows values in hex
const DISPLAY_HEX = uint(0)

// DISPLAY_DEC shows values in dec
const DISPLAY_DEC = uint(1)

// DISPLAY_BYTES shows values as bytes.
const DISPLAY_BYTES = uint(2)

// DISPLAY_CUSTOM selects a custom layout
const DISPLAY_CUSTOM = uint(256)

// SourceConstant provides information about constant values which are exposed
// to the trace generator.  Such constants can, in some cases, be modified to
// reflect different environments (e.g. different chains, gas limits, etc).
type SourceConstant struct {
	Name string
	// value of the constant
	Value big.Int
	// Explicit bitwidth for this constant.  This maybe math.MaxUint if no type
	// was given and, instead, the type should be inferred from context.
	Bitwidth uint
	// Indicates whether this is an "externally visible" constant.  That is, one
	// whose value can be changed after the fact.
	Extern bool
}

// Identify all fundamental columns declared in this module.  The visited set is
// used to ensure the final list contains each column only once.
func determineRegisters(module SourceModule, width uint, visited *bit.Set) []SourceColumn {
	var (
		cols []SourceColumn
	)
	// Update visited set
	for _, c := range module.Columns {
		index := c.Register.Index(width)
		if !visited.Contains(index) {
			visited.Insert(index)
			//
			cols = append(cols, c)
		}
	}
	// Explore all virtual submodules
	for _, m := range module.Submodules {
		if m.Virtual {
			cols = append(cols, determineRegisters(m, width, visited)...)
		}
	}
	// Done
	return cols
}

func init() {
	gob.Register(binfile.Attribute(&SourceMap{}))
}
