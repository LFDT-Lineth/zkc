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
package view

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
)

// RegisterView provides an abstract view of a given column.
type RegisterView interface {
	BitWidth() uint
	Get(uint) big.Int
	Len() uint
}

type registerView[F field.Element[F]] struct {
	trace   tr.Module[F]
	limbs   []register.LimbId
	mapping register.LimbsMap
}

func (p *registerView[F]) BitWidth() uint {
	var bitwidth uint
	//
	for _, lid := range p.limbs {
		bitwidth += p.mapping.Limb(lid).Width()
	}
	//
	return bitwidth
}

func (p *registerView[F]) Get(row uint) big.Int {
	var (
		bits  = uint(0)
		value big.Int
	)
	//
	for _, lid := range p.limbs {
		var (
			data    = p.trace.Column(lid.Unwrap()).Data()
			element = data.Get(row)
			val     big.Int
		)
		// Construct value from field element
		val.SetBytes(element.Bytes())
		// Shift and add
		value.Add(&value, val.Mul(&val, math.Pow2(bits)))
		// Shift value for multi-limb registers.  This specifically does nothing
		// in the case of single limb registers, since they include computed
		// registers which may not be present in the limbs map.
		if len(p.limbs) > 1 {
			// Extract limb information
			limb := p.mapping.Limb(lid)
			//
			bits += limb.Width()
		}
	}
	//
	return value
}

func (p *registerView[F]) Len() uint {
	return p.trace.Height()
}
