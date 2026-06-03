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
package compiler

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// NativeColumn provides information about a column acting as a parameter or
// return in a given native function.
type NativeColumn struct {
	// type of assigned column
	datatype ast.Type
	// multiplier for assigned column
	multiplier uint
}

// NativeDefinition describes a native function, such as specifying how many
// arguments it requires, etc.
type NativeDefinition struct {
	// Name of the intrinsic (e.g. "+")
	name string
	// Number of arguments this native can accept.
	arity uint
	// Responsible for doing whatever the function does.
	constructor func([]NativeColumn) []NativeColumn
}

var _ ast.FunctionBinding = &NativeDefinition{}

// Name returns the name of the intrinsic being defined.
func (p *NativeDefinition) Name() string {
	return p.name
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (p *NativeDefinition) Path() *file.Path {
	path := file.NewAbsolutePath(p.name)
	return &path
}

// IsPure checks whether this pure (which intrinsics always are).
func (p *NativeDefinition) IsPure() bool {
	return false
}

// IsNative checks whether this function binding is native (or not).
func (p *NativeDefinition) IsNative() bool {
	return true
}

// IsRecursive implementation for Binding interface.
func (p *NativeDefinition) IsRecursive() bool {
	return false
}

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (p *NativeDefinition) Arity() util.Option[uint] {
	return util.Some(p.arity)
}

// IsFinalised checks whether this binding has been finalised yet or not.
func (p *NativeDefinition) IsFinalised() bool {
	return true
}

// Binding returns the binding associated with this intrinsic.
func (p *NativeDefinition) Binding() ast.Binding {
	return p
}

// Lisp returns a lisp representation of this intrinsic.
func (p *NativeDefinition) Lisp() sexp.SExp {
	panic("unreachable")
}

// Apply returns the output columns given a set of input columns.
func (p *NativeDefinition) Apply(args []NativeColumn) []NativeColumn {
	return p.constructor(args)
}

// Signature returns the function signature for this binding.
func (p *NativeDefinition) Signature() *ast.FunctionSignature {
	// This is safe because natives can only (currently) be used in very
	// specific situations.
	return nil
}

// ============================================================================
// Native Definitions
// ============================================================================

// NATIVE_SIGNATURES identifies all built-in native computations which can be used in
// defcomputed assignments.
var NATIVE_SIGNATURES []NativeDefinition = []NativeDefinition{
	// Simple identity function.
	{"id", 1, nativeId},
	// Filter based on second argument
	{"filter", 2, nativeFilter},
	// Guarded map
	{"map-if", 3, nativeMapIf},
	{"map-if", 4, nativeMapIf},
	{"map-if", 5, nativeMapIf},
	{"map-if", 6, nativeMapIf},
	{"map-if", 7, nativeMapIf},
	{"map-if", 8, nativeMapIf},
	{"map-if", 9, nativeMapIf},
	{"map-if", 10, nativeMapIf},
	{"map-if", 11, nativeMapIf},
	{"map-if", 12, nativeMapIf},
	{"map-if", 13, nativeMapIf},
	{"map-if", 14, nativeMapIf},
	{"map-if", 15, nativeMapIf},
	{"map-if", 16, nativeMapIf},
	// Identify changes of a column within a given region (in forwards direction).
	{"fwd-changes-within", 2, nativeChangeWithin},
	{"fwd-changes-within", 3, nativeChangeWithin},
	{"fwd-changes-within", 4, nativeChangeWithin},
	{"fwd-changes-within", 5, nativeChangeWithin},
	{"fwd-changes-within", 6, nativeChangeWithin},
	{"fwd-changes-within", 7, nativeChangeWithin},
	{"fwd-changes-within", 8, nativeChangeWithin},
	{"fwd-changes-within", 9, nativeChangeWithin},
	{"fwd-changes-within", 10, nativeChangeWithin},
	{"fwd-changes-within", 11, nativeChangeWithin},
	{"fwd-changes-within", 12, nativeChangeWithin},
	{"fwd-changes-within", 13, nativeChangeWithin},
	{"fwd-changes-within", 14, nativeChangeWithin},
	{"fwd-changes-within", 15, nativeChangeWithin},
	{"fwd-changes-within", 16, nativeChangeWithin},
	// Identify rows which don't change within a given region (in forwards direction).
	{"fwd-unchanged-within", 2, nativeChangeWithin},
	{"fwd-unchanged-within", 3, nativeChangeWithin},
	{"fwd-unchanged-within", 4, nativeChangeWithin},
	{"fwd-unchanged-within", 5, nativeChangeWithin},
	{"fwd-unchanged-within", 6, nativeChangeWithin},
	{"fwd-unchanged-within", 7, nativeChangeWithin},
	{"fwd-unchanged-within", 8, nativeChangeWithin},
	{"fwd-unchanged-within", 9, nativeChangeWithin},
	{"fwd-unchanged-within", 10, nativeChangeWithin},
	{"fwd-unchanged-within", 11, nativeChangeWithin},
	{"fwd-unchanged-within", 12, nativeChangeWithin},
	{"fwd-unchanged-within", 13, nativeChangeWithin},
	{"fwd-unchanged-within", 14, nativeChangeWithin},
	{"fwd-unchanged-within", 15, nativeChangeWithin},
	{"fwd-unchanged-within", 16, nativeChangeWithin},
	// Identify changes of a column within a given region (in backwards direction).
	{"bwd-changes-within", 2, nativeChangeWithin},
	{"bwd-changes-within", 3, nativeChangeWithin},
	{"bwd-changes-within", 4, nativeChangeWithin},
	{"bwd-changes-within", 5, nativeChangeWithin},
	{"bwd-changes-within", 6, nativeChangeWithin},
	{"bwd-changes-within", 7, nativeChangeWithin},
	{"bwd-changes-within", 8, nativeChangeWithin},
	{"bwd-changes-within", 9, nativeChangeWithin},
	{"bwd-changes-within", 10, nativeChangeWithin},
	{"bwd-changes-within", 11, nativeChangeWithin},
	{"bwd-changes-within", 12, nativeChangeWithin},
	{"bwd-changes-within", 13, nativeChangeWithin},
	{"bwd-changes-within", 14, nativeChangeWithin},
	{"bwd-changes-within", 15, nativeChangeWithin},
	{"bwd-changes-within", 16, nativeChangeWithin},
	// Flood fill (forwards) within a given region
	{"fwd-fill-within", 3, nativeFillWithin},
	// Flood fill (backwards) within a given region
	{"bwd-fill-within", 3, nativeFillWithin},
}

func nativeId(inputs []NativeColumn) []NativeColumn {
	if len(inputs) != 1 {
		panic("unreachable")
	}

	return inputs
}

func nativeFilter(inputs []NativeColumn) []NativeColumn {
	if len(inputs) != 2 {
		panic("unreachable")
	}
	//
	return []NativeColumn{inputs[0]}
}

func nativeMapIf(inputs []NativeColumn) []NativeColumn {
	n := (len(inputs) - 3) % 2
	m := len(inputs) - 1
	// Sanity check (for now)
	if n != 0 {
		panic(fmt.Sprintf("map-if expects 3 + 2*n columns (given %d)", len(inputs)))
	}
	//
	return []NativeColumn{inputs[m]}
}

func nativeChangeWithin(inputs []NativeColumn) []NativeColumn {
	if len(inputs) <= 1 {
		panic("unreachable")
	}
	//
	result := NativeColumn{ast.NewUintType(1), inputs[0].multiplier}
	//
	return []NativeColumn{result}
}

func nativeFillWithin(inputs []NativeColumn) []NativeColumn {
	if len(inputs) <= 2 {
		panic("unreachable")
	}
	//
	return []NativeColumn{inputs[2]}
}
