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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// LimbsMap provides a high-level mapping of all registers across all
// modules before and after subdivision occurs.
type LimbsMap = Map[register.LimbsMap]

// NewLimbsMap constructs a new schema mapping for a given schema and
// parameter combination.  This determines, amongst other things,  the
// composition of limbs for all registers in the schema.
func NewLimbsMap[F any, M register.Map](field field.Config, modules ...M) LimbsMap {
	var mappings []register.LimbsMap
	//
	for _, m := range modules {
		regmap := register.NewLimbsMap[F](field, m)
		mappings = append(mappings, regmap)
	}
	//
	return limbsMap[register.LimbsMap]{field, mappings}
}

// ============================================================================
// LimbMap
// ============================================================================

// limbsMap provides a straightforward implementation of the schema.LimbMap
// interface.
type limbsMap[T register.Map] struct {
	field   field.Config
	modules []T
}

// Field implementation for schema.LimbMap interface
func (p limbsMap[T]) Field() field.Config {
	return p.field
}

// Module implementation for register.RegisterMappings interface
func (p limbsMap[T]) Module(mid Id) T {
	return p.modules[mid]
}

// ModuleOf implementation for register.RegisterMappings interface
func (p limbsMap[T]) ModuleOf(name Name) T {
	for _, m := range p.modules {
		if m.Name() == name {
			return m
		}
	}
	//
	panic(fmt.Sprintf("unknown module \"%s\"", name))
}

// Width returns the number of modules in this map
func (p limbsMap[T]) Width() uint {
	return uint(len(p.modules))
}

func (p limbsMap[T]) String() string {
	var builder strings.Builder
	//
	builder.WriteString("[")
	builder.WriteString(p.field.Name)
	builder.WriteString(":")
	//
	for i, m := range p.modules {
		if i != 0 {
			builder.WriteString(";")
		}
		//
		builder.WriteString(m.String())
	}

	builder.WriteString("]")

	return builder.String()
}
