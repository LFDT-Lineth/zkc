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
package decl

import (
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/stmt"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/variable"
)

// ResolvedFunction represents a function which contains instructions whose
// external identifiers are otherwise resolved. As such, it should not be
// possible that such a declaration refers to unknown (or otherwise incorrect)
// external components.
type ResolvedFunction = Function[symbol.Resolved]

// UnresolvedFunction represents a function which contains string identifiers
// for external (i.e. unlinked) components.  As such, its possible that such a
// function may fail with an error at link time due to an unresolvable
// reference to an external component (e.g. function, RAM, ROM, etc).
type UnresolvedFunction = Function[symbol.Unresolved]

// Function contains information about an executable function in the system.  A
// function has one or more variables where: the first n are the parameters; the
// next m are the returns; and all remaining registers are internal.
// Additionally, a function has some number of "instructions" which capture its
// semantics (i.e. intended behaviour).  The notion of an instruction is
// specifically left undefined by this interface to support different levels of
// the compilation pipeline.  For example, a compiled function has instructions
// which are simply bytes (or words) for efficient execution.  However, the
// instructions of an "assembly" level function implement the Instruction
// interface, which is better suited to analysis and/or translation into
// constraints.
type Function[S symbol.Symbol[S]] struct {
	// Unique name of this function.
	name string
	// Annotations associated with this declaration.
	annotations []string
	// Effects describes zero or more external memories which this function is
	// permitted to access.
	Effects []*S
	// Registers describes zero or more variables of a given width.  Each
	// register can be designated as an input / output or temporary.
	Variables []variable.Descriptor[S]
	// Number of input variables
	NumInputs uint
	// Number of output variables
	NumOutputs uint
	// Code defines the body of this function.
	Code []stmt.Stmt[S]
}

// NewFunction constructs a new function with the given variables and code
func NewFunction[S symbol.Symbol[S]](name string, effects []*S, vars []variable.Descriptor[S],
	code []stmt.Stmt[S]) *Function[S] {
	//
	var (
		numInputs  = array.CountMatching(vars, func(r variable.Descriptor[S]) bool { return r.IsParameter() })
		numOutputs = array.CountMatching(vars, func(r variable.Descriptor[S]) bool { return r.IsReturn() })
	)
	//
	return &Function[S]{
		name:       name,
		Effects:    effects,
		Variables:  vars,
		NumInputs:  numInputs,
		NumOutputs: numOutputs,
		Code:       code,
	}
}

// Arity implementation for Declaration interface
func (p *Function[S]) Arity() (nInputs, nOutputs uint) {
	return p.NumInputs, p.NumOutputs
}

// Annotations implementation for Declaration interface
func (p *Function[S]) Annotations() []string {
	return p.annotations
}

// SetAnnotations implementation for Declaration interface
func (p *Function[S]) SetAnnotations(annotations []string) {
	p.annotations = annotations
}

// Name implementation for Declaration interface
func (p *Function[S]) Name() string {
	return p.name
}

// Externs implementation for Declaration interface
func (p *Function[S]) Externs() []S {
	panic("todo")
}

// Variable implementation for variable.Map interface
func (p *Function[S]) Variable(id variable.Id) variable.Descriptor[S] {
	return p.Variables[id]
}

// Inputs returns an array containing the output variables of this function
func (p *Function[S]) Inputs() []variable.Descriptor[S] {
	return p.Variables[:p.NumInputs]
}

// Outputs returns an array containing the output variables of this function
func (p *Function[S]) Outputs() []variable.Descriptor[S] {
	var (
		n = p.NumInputs
		m = n + p.NumOutputs
	)
	//
	return p.Variables[n:m]
}
