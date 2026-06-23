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
package ast

import (
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// Binding represents an association between a name, as found in a source file,
// and concrete item (e.g. a column, function, etc).
type Binding interface {
	// Determine whether this binding is finalised or not.
	IsFinalised() bool
	// Determine whether this binding can be defined recursively or not.
	IsRecursive() bool
}

// FunctionBinding is a special kind of binding which captures the essence of
// something which can be called.  For example, this could be a user-defined
// function or an intrinsic.
type FunctionBinding interface {
	Binding
	// IsPure checks whether this function binding has side-effects or not.
	IsPure() bool
	// IsNative checks whether this function binding is native (or not).
	IsNative() bool
	// Signature returns the function signature for this binding.
	Signature() *FunctionSignature
}

// FunctionSignature embodies a concrete function instance.  It is necessary to
// separate bindings from signatures because, in corset, function overloading is
// supported.  That is, we can have different definitions for a function of the
// same name and arity.  The appropriate definition is then selected for the
// given parameter types.
type FunctionSignature struct {
	// Pure or not
	pure bool
	// Parameter types for this function
	parameters []Type
	// Return type for this function
	ret Type
	// Body of this function
	body Expr
}

// NewFunctionSignature creates a new function signature with a given set of
// parameter/return types and a body.  The signature can also be identified as
// that of a pure function, or not.
func NewFunctionSignature(pure bool, parameters []Type, ret Type, body Expr) *FunctionSignature {
	return &FunctionSignature{pure, parameters, ret, body}
}

// IsPure checks whether this function binding has side-effects or not.
func (p *FunctionSignature) IsPure() bool {
	return p.pure
}

// Return the (optional) return type for this signature.  If no declared return
// type is given, then the intention is that it be inferred from the body.
func (p *FunctionSignature) Return() Type {
	return p.ret
}

// Parameter returns the given parameter in this signature.
func (p *FunctionSignature) Parameter(index uint) Type {
	return p.parameters[index]
}

// Arity returns the number of parameters in this signature.
func (p *FunctionSignature) Arity() uint {
	return uint(len(p.parameters))
}

// Apply a set of concreate arguments to this function.  This substitutes
// them through the body of the function producing a single expression.
func (p *FunctionSignature) Apply(args []Expr, srcmap *source.Maps[Node]) Expr {
	mapping := make(map[uint]Expr)
	// Setup the mapping
	for i, e := range args {
		mapping[uint(i)] = e
	}
	// Substitute through
	return Substitute(p.body, mapping, srcmap)
}

// ============================================================================
// ColumnBinding
// ============================================================================

const (
	// NOT_COMPUTED signals a column is not a computed column.
	NOT_COMPUTED = 0
	// COMPUTED signals a column is a (non-recursive) computed column.
	COMPUTED = 1
	// COMPUTED_FWD signals a column is a (forward recursive) computed column.
	// This means its value is computed starting from the first row (hence it
	// cannot use a forward shift in its declaration).
	COMPUTED_FWD = 2
	// COMPUTED_BWD signals a column is a (backward recursive) computed column.
	// This means its value is computed starting from the last row (hence it
	// cannot use a backward shift in its declaration).
	COMPUTED_BWD = 3
)

// ColumnBinding represents something bound to a given column.
type ColumnBinding struct {
	// Context determines the real (i.e. non-virtual) enclosing module of this
	// column, and should always be a prefix of the path.   If this column was
	// declared in a perspective then it will be the perspective's enclosing
	// module.  Otherwise, it will exactly match the path's parent.
	ColumnContext file.Path
	// Absolute Path of column.  This determines the name of the column, its
	// enclosing module and/or perspective.
	Path file.Path
	// Column's datatype
	DataType Type
	// Determines whether this column must be proven (or not).
	MustProve bool
	// Column's length Multiplier
	Multiplier uint
	// Determines the kind of this column.
	Kind uint8
	// Padding value (defaults to 0)
	Padding big.Int
	// Display modifier
	Display string
}

// AbsolutePath returns the fully resolved (absolute) path of the column in question.
func (p *ColumnBinding) AbsolutePath() *file.Path {
	return &p.Path
}

// IsComputed checks whether this binding is for a computed column (or not).
func (p *ColumnBinding) IsComputed() bool {
	return p.Kind != NOT_COMPUTED
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *ColumnBinding) IsFinalised() bool {
	return p.Multiplier != 0
}

// IsRecursive implementation for Binding interface.
func (p *ColumnBinding) IsRecursive() bool {
	return p.Kind == COMPUTED_FWD || p.Kind == COMPUTED_BWD
}

// Finalise this binding by providing the necessary missing information.
func (p *ColumnBinding) Finalise(multiplier uint, datatype Type) {
	p.Multiplier = multiplier
	p.DataType = datatype
}

// Context returns the of this column.  That is, the module in which this colunm
// was declared and also the length multiplier of that module it requires.
func (p *ColumnBinding) Context() Context {
	return NewContext(p.ColumnContext.String(), p.Multiplier)
}

// ============================================================================
// ConstantBinding
// ============================================================================

// ConstantBinding represents a constant definition
type ConstantBinding struct {
	Path file.Path
	// Explicit type for this constant.  This maybe nil if no type was given
	// and, instead, the type should be inferred from context.
	DataType Type
	// Constant expression which, when evaluated, produces a constant Value.
	Value Expr
	// Determines whether this is an "externalised" constant, or not.
	// Externalised constants are visible at the HIR level and can have their
	// values overridden.
	Extern bool
	// Determines whether or not this binding is finalised (i.e. its expression
	// has been resolved).
	finalised bool
}

// NewConstantBinding creates a new constant binding (which is initially not
// finalised).
func NewConstantBinding(path file.Path, datatype Type, value Expr, extern bool) ConstantBinding {
	return ConstantBinding{path, datatype, value, extern, false}
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *ConstantBinding) IsFinalised() bool {
	return p.finalised
}

// IsRecursive implementation for Binding interface.
func (p *ConstantBinding) IsRecursive() bool {
	// Constants can never be defined recursively
	return false
}

// Finalise this binding.
func (p *ConstantBinding) Finalise() {
	p.finalised = true
}

// Context returns the of this constant, noting that constants (by definition)
// do not have a context.
func (p *ConstantBinding) Context() Context {
	return VoidContext()
}

// ============================================================================
// ParameterBinding
// ============================================================================

// LocalVariableBinding represents something bound to a given column.
type LocalVariableBinding struct {
	// Name the local variable
	Name string
	// Type to use for this parameter.
	DataType Type
	// Identifies the variable or column Index (as appropriate).
	Index uint
}

// NewLocalVariableBinding constructs an (unitilalised) variable binding.  Being
// uninitialised means that its index identifier remains unknown.
func NewLocalVariableBinding(name string, datatype Type) LocalVariableBinding {
	return LocalVariableBinding{name, datatype, math.MaxUint}
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *LocalVariableBinding) IsFinalised() bool {
	return p.Index != math.MaxUint
}

// IsRecursive implementation for Binding interface.
func (p *LocalVariableBinding) IsRecursive() bool {
	return false
}

// Finalise this local variable binding by allocating it an identifier.
func (p *LocalVariableBinding) Finalise(index uint) {
	p.Index = index
}

// ============================================================================
// DefunBinding
// ============================================================================

// DefunBinding is a function binding arising from a user-defined function (as
// opposed, for example, to a function binding arising from an intrinsic).
type DefunBinding struct {
	// Flag whether or not is Pure function
	Pure bool
	// Types of parameters (optional)
	ParamTypes []Type
	// Type of return (optional)
	ReturnType Type
	// Body of the function in question.
	Body Expr
	// Indicates whether this symbol is finalised (i.e. all expressions have
	// been resolved).
	finalised bool
}

var _ FunctionBinding = &DefunBinding{}

// NewDefunBinding constructs a new function binding.
func NewDefunBinding(pure bool, paramTypes []Type, returnType Type, forced bool, body Expr) DefunBinding {
	return DefunBinding{pure, paramTypes, returnType, body, false}
}

// IsPure checks whether this is a defpurefun or not
func (p *DefunBinding) IsPure() bool {
	return p.Pure
}

// IsNative checks whether this function binding is native (or not).
func (p *DefunBinding) IsNative() bool {
	return false
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *DefunBinding) IsFinalised() bool {
	return p.finalised
}

// IsRecursive implementation for Binding interface.
func (p *DefunBinding) IsRecursive() bool {
	// Functions can never be defined recursively (for now, at least).
	return false
}

// Signature returns the corresponding function signature for this user-defined
// function.
func (p *DefunBinding) Signature() *FunctionSignature {
	return &FunctionSignature{p.Pure, p.ParamTypes, p.ReturnType, p.Body}
}

// Finalise this binding by providing the necessary missing information.
func (p *DefunBinding) Finalise() {
	p.finalised = true
}

// ============================================================================
// Perspective
// ============================================================================

// PerspectiveBinding contains key information about a perspective, such as its
// selector expression.
type PerspectiveBinding struct {
	// Expression which determines when this perspective is enabled.
	Selector Expr
	// Indicates whether or not the selector has been finalised.
	resolved bool
}

var _ Binding = &PerspectiveBinding{}

// NewPerspectiveBinding constructs a new binding for a given perspective.
func NewPerspectiveBinding(selector Expr) *PerspectiveBinding {
	return &PerspectiveBinding{selector, false}
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *PerspectiveBinding) IsFinalised() bool {
	return p.resolved
}

// IsRecursive implementation for Binding interface.
func (p *PerspectiveBinding) IsRecursive() bool {
	// Recursive perspectives don't make sense!
	return false
}

// Finalise this binding, which indicates the selector expression has been
// finalised.
func (p *PerspectiveBinding) Finalise() {
	p.resolved = true
}
