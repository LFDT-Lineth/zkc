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
package parser

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/util/collection/bit"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

type globalEnvironment struct {
	effects []*symbol.Unresolved
	// Variables identifies set of declared variables.
	variables []VariableDescriptor
}

type localEnvironment struct {
	// Set of visible variables in this environment
	visible bit.Set
	// inLoop indicates whether we are currently inside a loop body
	inLoop bool
}

// Environment captures useful information used during the assembling process.
type Environment struct {
	global *globalEnvironment
	local  *localEnvironment
}

// EmptyEnvironment constructs an initially empty environment
func EmptyEnvironment() Environment {
	return Environment{
		global: &globalEnvironment{nil, nil},
		local:  &localEnvironment{},
	}
}

// Clone constructs a clone of this environment, such that variables declared in
// the clone will not clash with those declared elsewhere.  The inLoop parameter
// indicates whether the cloned environment is inside a loop.
func (p *Environment) Clone(inLoop bool) Environment {
	var local localEnvironment
	// Clone local variables
	local.visible = p.local.visible.Clone()
	local.inLoop = inLoop
	// Otherwise, keep global as is
	return Environment{global: p.global, local: &local}
}

// InLoop returns whether the current environment is inside a loop body.
func (p *Environment) InLoop() bool {
	return p.local.inLoop
}

// Effects returns the set of memory effects declared globally
func (p *Environment) Effects() []*symbol.Unresolved {
	return p.global.effects
}

// Variables returns the set of variables declared globally
func (p *Environment) Variables() []VariableDescriptor {
	return p.global.variables
}

// DeclareEffect declares a new effect.  If an effect with the same name
// already exists, this panics.
func (p *Environment) DeclareEffect(effect *symbol.Unresolved) {
	//
	if p.IsDeclared(effect.Name) {
		panic(fmt.Sprintf("effect %s already declared", effect.Name))
	}
	//
	p.global.effects = append(p.global.effects, effect)
}

// DeclareVariable declares a new register with the given name and bitwidth.  If
// a register with the same name already exists, this panics.
func (p *Environment) DeclareVariable(kind variable.Kind, name string, datatype Type) {
	// Determine global index of this variable
	var index = uint(len(p.global.variables))
	// Check whether it clashes with another variable in the same (local) environment
	if p.IsDeclared(name) {
		panic(fmt.Sprintf("variable %s already declared", name))
	}
	// Update global environment
	p.global.variables = append(p.global.variables, variable.New(kind, name, datatype))
	// Update local environment
	p.local.visible.Insert(index)
}

// IsDeclared checks whether or not a given name is already declared (either as
// an effect or a variable).
func (p *Environment) IsDeclared(name string) bool {
	// check effects
	for _, effect := range p.global.effects {
		if effect.Name == name {
			return true
		}
	}
	// check local variables
	return p.IsDeclaredVariable(name)
}

// IsDeclaredVariable checks whether or not a given name is already declared as
// a variable.
func (p *Environment) IsDeclaredVariable(name string) bool {
	// check local variables
	for iter := p.local.visible.Iter(); iter.HasNext(); {
		var index = iter.Next()
		if p.global.variables[index].Name == name {
			return true
		}
	}
	//
	return false
}

// LookupVariable looks up the index for a given register.
func (p *Environment) LookupVariable(name string) variable.Id {
	// check local variables
	for iter := p.local.visible.Iter(); iter.HasNext(); {
		var index = iter.Next()
		if p.global.variables[index].Name == name {
			return index
		}
	}
	//
	panic(fmt.Sprintf("unknown register %s", name))
}
