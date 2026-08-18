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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// Binding represents an association between a name, as found in a source file,
// and concrete item (e.g. a column, function, etc).
type Binding interface {
	// Determine whether this binding is finalised or not.
	IsFinalised() bool
	// Determine whether this binding can be defined recursively or not.
	IsRecursive() bool
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
	// Context determines the enclosing module of this column, and should
	// always match the path's parent.
	ColumnContext file.Path
	// Absolute Path of column.  This determines the name of the column and its
	// enclosing module.
	Path file.Path
	// Column's datatype
	DataType Type
	// Determines whether this column must be proven (or not).
	MustProve bool
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
	return true
}

// IsRecursive implementation for Binding interface.
func (p *ColumnBinding) IsRecursive() bool {
	return p.Kind == COMPUTED_FWD || p.Kind == COMPUTED_BWD
}

// Context returns the module in which this column was declared.
func (p *ColumnBinding) Context() Context {
	return NewContext(p.ColumnContext.String())
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
	// Determines whether or not this binding is finalised (i.e. its expression
	// has been resolved).
	finalised bool
}

// NewConstantBinding creates a new constant binding (which is initially not
// finalised).
func NewConstantBinding(path file.Path, datatype Type, value Expr) ConstantBinding {
	return ConstantBinding{path, datatype, value, false}
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
