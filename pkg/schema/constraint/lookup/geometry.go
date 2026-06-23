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
package lookup

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// Geometry defines the "geometry" of a lookup.  That is the maximum
// bitwidth for each source-target pairing in the lookup.  For example, consider
// a lookup where (X Y) looksup into (A B).  Suppose X is 16bit and Y is 32bit,
// whilst A is 64bit and B is 8bit. Then, the geometry of the lookup is [16,32].
type Geometry struct {
	config field.Config
	// bitwidth for each source/target pairing
	geometry []uint
}

// NewGeometry returns the calculated "geometry" for this lookup.  That
// is, for each source/target pair, the maximum bitwidth of any source or target
// value.
func NewGeometry[F field.Element[F], E term.Evaluable[F], T register.Map](c Constraint[F, E],
	mapping module.Map[T]) Geometry {
	//
	var geometry []uint = make([]uint, c.Sources[0].Len())
	// Include sources
	for _, source := range c.Sources {
		updateGeometry(geometry, source, mapping)
	}
	// Include targets
	for _, target := range c.Targets {
		updateGeometry(geometry, target, mapping)
	}
	//
	return Geometry{mapping.Field(), geometry}
}

// BandWidth returns maximum field bandwidth available in the field.
func (p *Geometry) BandWidth() uint {
	return p.config.BandWidth
}

// RegisterWidth returns maximum permitted register width for the field.
func (p *Geometry) RegisterWidth() uint {
	return p.config.RegisterWidth
}

// LimbWidths returns the bitwidths for the required limbs for a given
// source/target pairing in the lookup.
func (p *Geometry) LimbWidths(i uint) []uint {
	if p.geometry[i] == 0 {
		return nil
	}
	//
	return register.LimbWidths(p.config.RegisterWidth, p.geometry[i])
}

func updateGeometry[F field.Element[F], E term.Evaluable[F], T register.Map](geometry []uint, source Vector[F, E],
	mapping module.Map[T]) {
	//
	var (
		regmap = mapping.Module(source.Module)
	)
	// Sanity check
	if source.Len() != uint(len(geometry)) {
		// Unreachable, as should be caught earlier in the pipeline.
		panic("misaligned lookup")
	}
	//
	for i, ith := range source.Terms {
		ithRange := ith.ValueRange()
		bitwidth, signed := ithRange.BitWidth()
		// Sanity check
		if signed {
			panic(fmt.Sprintf("signed lookup encountered (%s)", ith.Lisp(true, regmap).String(true)))
		}
		//
		geometry[i] = max(geometry[i], bitwidth)
	}
}
