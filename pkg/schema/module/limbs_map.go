// Copyright Consensys Incorporated
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
type LimbsMap[F field.Element[F]] = Map[F, register.LimbsMap[F]]

// NewLimbsMap constructs a new schema mapping for a given schema and
// parameter combination.  This determines, amongst other things,  the
// composition of limbs for all registers in the schema.
func NewLimbsMap[F field.Element[F]](modules ...register.LimbsMap[F]) LimbsMap[F] {
	return limbsMap[F, register.LimbsMap[F]]{modules}
}

// IdentityMap constructs a "identity" mapping which maps each register to
// itself.
func IdentityMap[F field.Element[F], M register.Map[F]](modules ...M) LimbsMap[F] {
	var mappings []register.LimbsMap[F]
	//
	for _, m := range modules {
		regmap := register.IdentityMap[F](m)
		mappings = append(mappings, regmap)
	}
	//
	return limbsMap[F, register.LimbsMap[F]]{mappings}
}

// ============================================================================
// LimbMap
// ============================================================================

// limbsMap provides a straightforward implementation of the schema.LimbMap
// interface.
type limbsMap[F field.Element[F], T register.Map[F]] struct {
	modules []T
}

// Module implementation for register.RegisterMappings interface
func (p limbsMap[F, T]) Module(mid Id) T {
	return p.modules[mid]
}

// ModuleOf implementation for register.RegisterMappings interface
func (p limbsMap[F, T]) ModuleOf(name Name) T {
	for _, m := range p.modules {
		if m.Name() == name {
			return m
		}
	}
	//
	panic(fmt.Sprintf("unknown module \"%s\"", name))
}

// Width returns the number of modules in this map
func (p limbsMap[F, T]) Width() uint {
	return uint(len(p.modules))
}

func (p limbsMap[F, T]) String() string {
	var builder strings.Builder
	//
	builder.WriteString("[")
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
