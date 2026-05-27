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
package program

import (
	"github.com/consensys/go-corset/pkg/asm/io"
	"github.com/consensys/go-corset/pkg/schema"
	sc "github.com/consensys/go-corset/pkg/schema"
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/field"
)

// Module programs a wrapper around an io.Function which makes it look like a
// schema.Module.
type Module[F field.Element[F], T io.Instruction] struct {
	id       sc.ModuleId
	function io.Component[T]
}

// NewModule constructs a new wrapper around a given io.Function instance.
func NewModule[F field.Element[F], T io.Instruction](id sc.ModuleId, function io.Component[T]) *Module[F, T] {
	return &Module[F, T]{id, function}
}

// Function returns the underlying function being wrapped.
func (p *Module[F, T]) Function() io.Component[T] {
	return p.function
}

// ============================================================================
// register.RegisterMap
// ============================================================================

// Name implementation for register.RegisterMap interface.
func (p *Module[F, T]) Name() module.Name {
	return p.function.Name()
}

// HasRegister implementation for register.RegisterMap interface.
func (p *Module[F, T]) HasRegister(name string) (register.Id, bool) {
	return p.function.HasRegister(name)
}

// Register implementation for register.RegisterMap interface.
func (p *Module[F, T]) Register(id register.Id) register.Register {
	return p.function.Register(id)
}

// Registers implementation for register.RegisterMap interface.
func (p *Module[F, T]) Registers() []register.Register {
	return p.function.Registers()
}

// ============================================================================
// schema.Module
// ============================================================================

// Assignments implementation for schema.Module interface.
func (p *Module[F, T]) Assignments() iter.Iterator[schema.Assignment[F]] {
	panic("unsupported operation")
}

// AllowPadding implementation for schema.Module interface.
func (p *Module[F, T]) AllowPadding() bool {
	return false
}

// Constraints implementation for schema.Module interface.
func (p *Module[F, T]) Constraints() iter.Iterator[schema.Constraint[F]] {
	panic("unsupported operation")
}

// Consistent implementation for schema.Module interface.
func (p *Module[F, T]) Consistent(fieldWidth uint, schema schema.AnySchema[F]) []error {
	return nil
}

// Keys implementation of Module interface.
func (p *Module[F, T]) Keys() uint {
	panic("unsupported operation")
}

// LengthMultiplier implementation for schema.Module interface.
func (p *Module[F, T]) LengthMultiplier() uint {
	return 1
}

// IsPublic implementation for schema.Module interface.
func (p *Module[F, T]) IsPublic() bool {
	return p.function.IsPublic()
}

// IsSynthetic implementation for schema.Module interface.
func (p *Module[F, T]) IsSynthetic() bool {
	return false
}

// IsNative implementation for schema.Module interface.  Modules originating
// from the assembly pipeline are never native.
func (p *Module[F, T]) IsNative() bool {
	return false
}

// IsStatic implementation for schema.Module interface.  Modules originating
// from the assembly pipeline are never static.
func (p *Module[F, T]) IsStatic() bool {
	return false
}

// StaticContents implementation for schema.Module interface.  Modules
// originating from the assembly pipeline are never static, hence this function
// always panics.
func (p *Module[F, T]) StaticContents() [][]F {
	panic("non-static modules have no contents")
}

// Substitute any matchined labelled constants within this module
func (p *Module[F, T]) Substitute(mapping map[string]F) {
	// For now, this is a no-operation because assembly has no concept of
	// labelled constants.  In the future, we might expect this to change.
}

// Width implementation for schema.Module interface.
func (p *Module[F, T]) Width() uint {
	return uint(len(p.function.Registers()))
}

func (p *Module[F, T]) String() string {
	return p.function.String()
}
