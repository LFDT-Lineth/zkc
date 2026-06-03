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
package data

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// ResolvedFixedArray represents a fixed-size array which contains only resolved identifiers.
type ResolvedFixedArray = FixedArray[symbol.Resolved]

// UnresolvedFixedArray represents a fixed-size array which contains only unresolved identifiers.
type UnresolvedFixedArray = FixedArray[symbol.Unresolved]

// FixedArray captures a fixed sized array type.
type FixedArray[I symbol.Symbol[I]] struct {
	DataType Type[I]
	// Size holds either a numeric literal or a symbolic name for the array
	// size.  The latter (if present) must be resolved during linking.
	Size util.Union[uint, I]
}

// NewFixedArray constructs a fixed-size array Type.
func NewFixedArray[I symbol.Symbol[I]](datatype Type[I], size util.Union[uint, I]) *FixedArray[I] {
	return &FixedArray[I]{DataType: datatype, Size: size}
}

// AsUint implementation for Type interface
func (p *FixedArray[I]) AsUint(env Environment[I]) *UnsignedInt[I] {
	return nil
}

// AsTuple implementation for Type interface
func (p *FixedArray[I]) AsTuple(Environment[I]) *Tuple[I] {
	return nil
}

// AsAlias implementation for Type interface
func (p *FixedArray[I]) AsAlias(Environment[I]) *Alias[I] {
	return nil
}

// AsField implementation for Type interface
func (p *FixedArray[S]) AsField(Environment[S]) *FieldElement[S] { return nil }

// AsFixedArray implementation for Type interface
func (p *FixedArray[I]) AsFixedArray(Environment[I]) *FixedArray[I] {
	return p
}

func (p *FixedArray[I]) String(env Environment[I]) string {
	return fmt.Sprintf("[%s;%s]", p.DataType.String(env), p.SizeString())
}

// SizeString returns the size of this string as a symbol.
func (p *FixedArray[I]) SizeString() string {
	if p.Size.HasFirst() {
		return fmt.Sprintf("%d", p.Size.First())
	} else {
		return p.Size.Second().String()
	}
}

// Resolve returns the type that this fixed-size array refers to in the given environment.
func (p *FixedArray[I]) Resolve(Environment[I]) Type[I] {
	return p.DataType
}
