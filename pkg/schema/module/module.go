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
package module

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Name abstracts the notion of a module name, since this is made up of two
// distinct components.
type Name = trace.ModuleName

// NewName constructs a new module name
func NewName(name string, multiplier uint) Name {
	return Name{Name: name, Multiplier: multiplier}
}

// Id abstracts the notion of a "module identifier"
type Id = uint

// Map provides a mapping from module identifiers (or names) to register
// maps.
type Map[T register.Map] interface {
	fmt.Stringer
	// Field returns the underlying field configuration used for this mapping.
	// This includes the field bandwidth (i.e. number of bits available in
	// underlying field) and the maximum register width (i.e. width at which
	// registers are capped).
	Field() field.Config
	// Module returns register mapping information for the given module.
	Module(Id) T
	// ModuleOf returns register mapping information for the given module.
	ModuleOf(Name) T
	// Returns number of modules in this map
	Width() uint
}

// NewMap constructs a new module map
func NewMap[T register.Map](field field.Config, modules []T) Map[T] {
	return limbsMap[T]{field, modules}
}

// Apply converts a module map of one kind into a module map of another kind.
func Apply[S, T register.Map](mapping Map[S], fn func(S) T) Map[T] {
	var (
		mods = make([]T, mapping.Width())
	)
	//
	for i := range mapping.Width() {
		mods[i] = fn(mapping.Module(i))
	}
	//
	return NewMap(mapping.Field(), mods)
}
