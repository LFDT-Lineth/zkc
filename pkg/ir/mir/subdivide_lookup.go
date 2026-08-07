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
package mir

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// mappedVector is an intermediate form of a lookup vector, where every register
// has been mapped into the limbs it was divided into (but where those limbs
// have not yet been split out into individual source/target pairings).
type mappedVector struct {
	// Module in which all limbs are located.
	module module.Id
	// Selector for this vector (optional).  Observe that a selector is never
	// divided into more than one limb.
	selector util.Option[register.Id]
	// Limbs making up each register of the original vector.
	limbs [][]register.LimbId
}

// Subdivide implementation for the FieldAgnostic interface.
func (p *Subdivider[F]) subdivideLookup(c LookupConstraint[F]) LookupConstraint[F] {
	var (
		// Determine overall geometry for this lookup.
		geometry = lookup.NewGeometry(c, p.mapping)
		// Split all registers in the source vectors
		vSources = p.mapLookupVectors(c.Sources)
		// Split all registers in the target vectors
		vTargets = p.mapLookupVectors(c.Targets)
	)
	//
	targets := p.splitLookupVectors(geometry, vTargets)
	sources := p.splitLookupVectors(geometry, vSources)
	//
	return lookup.NewConstraint[F](c.Handle, targets, sources)
}

// Mapping lookup vectors essentially means applying the limb mapping to all
// registers used within the lookup vector.  For example, consider a simple
// lookup like "lookup (X) (Y)" where X=>X'1::X'0 and Y=>Y.  Then, after
// mapping, we have "lookup (X'1::X'0) (Y)".  Observe that mapping does not
// create more source/target pairings.  Rather, it splits registers within the
// existing pairings only.  Later stages will subdivide and pad the
// source/target pairings as necessary.
func (p *Subdivider[F]) mapLookupVectors(vectors []lookup.Vector) []mappedVector {
	//
	var nvectors = make([]mappedVector, len(vectors))
	//
	for i, vector := range vectors {
		var (
			modmap   = p.mapping.Module(vector.Module)
			limbs    = make([][]register.LimbId, len(vector.Registers))
			selector = util.None[register.Id]()
		)
		// Split registers
		for j, rid := range vector.Registers {
			limbs[j] = limbsOfRegister(rid, modmap)
		}
		// Split selector
		if vector.Selector.HasValue() {
			split := limbsOfRegister(vector.Selector.Unwrap(), modmap)
			// Sanity check
			if len(split) != 1 {
				panic("non-atomic selector encountered")
			}
			//
			selector = util.Some(split[0])
		}
		// Done
		nvectors[i] = mappedVector{vector.Module, selector, limbs}
	}
	//
	return nvectors
}

// Splitting lookup vectors means splitting source/target pairings into one or
// more registers.  For example, consider the lookup "lookup (X'1::X'0) (Y)"
// where X'1, X'0, and Y are all u16.  The geometry of this lookup is [u32].
// Assume a field bandwidth which cannot hold a u32 without overflow. Then,
// after splitting, we would expect "lookup (X'0, X'1) (Y, 0)". Here, the
// geometry has now changed to [u16,u16] to accommodate the field bandwidth.
// Furthermore, notice padding has been applied to ensure we have a matching
// number of columns on the left- and right-hand sides.
func (p *Subdivider[F]) splitLookupVectors(geometry lookup.Geometry, vectors []mappedVector) []lookup.Vector {
	//
	var nvectors = make([]lookup.Vector, len(vectors))
	//
	for i, vector := range vectors {
		nvectors[i] = p.splitLookupVector(geometry, vector)
	}
	//
	return nvectors
}

func (p *Subdivider[F]) splitLookupVector(geometry lookup.Geometry, vector mappedVector) lookup.Vector {
	//
	var limbs []register.LimbId
	// Check alignment
	for i, ith := range vector.limbs {
		// Pad & flatten
		limbs = append(limbs, p.padLookupLimb(uint(i), ith, geometry, vector.module)...)
	}
	// Done
	return lookup.NewVector(vector.module, vector.selector, limbs...)
}

// Padding is about ensuring a matching number of columns for each source/target
// pairing in the lookup.  To understand the purpose of padding, consider this
// lookup (where X is u256 and Y is u128):
//
// (lookup (X) (Y))
//
// Let's assume we want to split this lookup for a maximum register width of
// u128.  Then, without padding, we would end up with this:
//
// (lookup (X'0 X'1) (Y))
//
// Here, we have a mismatched number of columns because Y did not need to be
// split.  To resolve this, we need to pad the translation of Y as follows:
//
// (lookup (X'0 X'1) (Y 0))
//
// Here, 0 has been appended to the translation of Y to match the number of
// columns required for X.
//
// NOTE: this also sanity checks against so-called "irregular lookups". These
// only arise in relatively unlikely scenarios.  However, since the problem is
// dependent upon the particular field configuration used, it is important to
// support them in order to be truly field agnostic.  To understand the issue of
// alignment, consider this lookup (where X is u160 and Y is u128):
//
// (lookup (X) (Y))
//
// Let's assume we want to split this lookup for a maximum register width of
// u160.  Then, without alignment, we would get this:
//
// (lookup (X 0) (Y'0 Y'1))
//
// This translation is clearly unsound, because the most significant 32 bits of
// X are ignored.  The problem is that, since Y exceed the maximum register
// width it was split accordingly (and recall that splitting attempts to ensure
// a balanced division between limbs); however, since X did not exceed the
// maximum register width, it was not split.
//
// Irregular looks can be resolved only by introducing temporary registers to
// divide X into u128 limbs.  Something like this would be a valid translation
// of the above (where T'0,T'1 are u128 temporaries):
//
// (lookup (T'0 T'1) (Y'0 Y'1)) ; (vanish (X == (T'1 * 2^128) + T'0))
//
// NOTE: For now, this function only checks that limbs are aligned and panics
// otherwise.
func (p *Subdivider[F]) padLookupLimb(i uint, limbs []register.LimbId, geometry lookup.Geometry,
	mid module.Id) []register.LimbId {
	//
	var (
		modmap = p.mapping.Module(mid)
		widths = geometry.LimbWidths(i)
		// Determine expected geometry (i.e. number of columns) at this
		// position.
		n = len(widths)
		m = len(limbs) - 1
		// Append available limbs
		nlimbs = limbs
	)
	// Sanity check
	for i, limb := range limbs {
		bitwidth := modmap.Limb(limb).Width()
		// Sanity check for irregular lookups
		if i != n && bitwidth > widths[i] {
			panic(fmt.Sprintf("irregular lookup detected (u%d v u%d)", bitwidth, widths[i]))
		} else if i != m && bitwidth != widths[i] {
			panic(fmt.Sprintf("irregular lookup detected (u%d v u%d)", bitwidth, widths[i]))
		}
	}
	// Pad out with zeros to match geometry
	//nolint
	for m := n - len(limbs); m > 0; m-- {
		// Get access to a constant zero register
		zero := p.ZeroRegister(mid)
		// Pad out the vector
		nlimbs = append(nlimbs, zero)
	}
	//
	return nlimbs
}

// limbsOfRegister determines the limbs into which a given register was divided.
// Observe that any limbs which lie beyond the register's bitwidth are dropped,
// though at least one limb is always retained (as needed for registers whose
// bitwidth is zero, such as constant registers).
func limbsOfRegister(rid register.Id, mapping register.LimbsMap) []register.LimbId {
	var (
		bitwidth = mapping.Register(rid).Width()
		limbs    []register.LimbId
	)
	//
	for i, limbId := range mapping.LimbIds(rid) {
		limbWidth := min(bitwidth, mapping.Limb(limbId).Width())
		//
		if limbWidth > 0 || i == 0 {
			limbs = append(limbs, limbId)
		}
		//
		bitwidth -= limbWidth
	}
	//
	return limbs
}
