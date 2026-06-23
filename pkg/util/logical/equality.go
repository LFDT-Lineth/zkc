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
package logical

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// Variable defines some required characteristics of any variable identifier
// suitable for use within an equality.
type Variable[I any] interface {
	fmt.Stringer
	array.Comparable[I]
}

// Equality represents a fundamental atom which is either an Equality (=) or a
// non-Equality (≠) between a variable and either a variable or a constant.
type Equality[I Variable[I]] struct {
	// Sign indicates whether this is an Equality (==) or a non-Equality (!=).
	Sign bool
	// Left variable
	Left I
	// Right variable or constant
	Right util.Union[I, big.Int]
}

// Equals returns an equality over two variables (i.e. l = r)
func Equals[I Variable[I]](l I, r I) Equality[I] {
	// Ensure lowest variable always on the left-hand side.
	if l.Cmp(r) > 0 {
		l, r = r, l
	}
	//
	return Equality[I]{true, l, util.Union1[I, big.Int](r)}
}

// EqualsConst returns an equality over a variable and a constant
func EqualsConst[I Variable[I]](l I, r big.Int) Equality[I] {
	return Equality[I]{true, l, util.Union2[I, big.Int](r)}
}

// NotEquals returns a non-Equality over two variables (i.e. l ≠ r)
func NotEquals[I Variable[I]](l I, r I) Equality[I] {
	// Ensure lowest variable always on the left-hand side.
	if l.Cmp(r) > 0 {
		l, r = r, l
	}
	//
	return Equality[I]{false, l, util.Union1[I, big.Int](r)}
}

// NotEqualsConst returns an equality over a variable and a constant
func NotEqualsConst[I Variable[I]](l I, r big.Int) Equality[I] {
	return Equality[I]{false, l, util.Union2[I, big.Int](r)}
}

// CloseOver implementation for Atom interface
func (p Equality[I]) CloseOver(o Equality[I]) Equality[I] {
	if p.Cmp(o) == 0 {
		// Do nothing when p == o
	} else if p.Left.Cmp(o.Left) == 0 && o.Sign {
		if p.Sign {
			// x == e1 && x == e2 ==> e1 == e2
			return equate(p.Right, o.Right, p.Left)
		}
		// x != e1 && x == e2 ==> e1 != e2
		return unequate(p.Right, o.Right, p.Left)
	} else if p.Right.HasSecond() || !o.Sign {
		// x == c && y == e
		// x == e && y != e
		return p
	} else if right := p.Right.First(); right.Cmp(o.Left) == 0 {
		// x == y && y == e => x == e
		return Equality[I]{p.Sign, p.Left, o.Right}
	}
	//
	return p
}

func equate[I Variable[I]](l util.Union[I, big.Int], r util.Union[I, big.Int], v I) Equality[I] {
	switch {
	case l.HasFirst() && r.HasFirst():
		return Equals(l.First(), r.First())
	case l.HasFirst() && r.HasSecond():
		return EqualsConst(l.First(), r.Second())
	case l.HasSecond() && r.HasFirst():
		return EqualsConst(r.First(), l.Second())
	default:
		var (
			lc = l.Second()
			rc = r.Second()
		)
		//
		if lc.Cmp(&rc) == 0 {
			// true
			return Equals(v, v)
		}
		// false
		return NotEquals(v, v)
	}
}

func unequate[I Variable[I]](l util.Union[I, big.Int], r util.Union[I, big.Int], v I) Equality[I] {
	switch {
	case l.HasFirst() && r.HasFirst():
		return NotEquals(l.First(), r.First())
	case l.HasFirst() && r.HasSecond():
		return NotEqualsConst(l.First(), r.Second())
	case l.HasSecond() && r.HasFirst():
		return NotEqualsConst(r.First(), l.Second())
	default:
		var (
			lc = l.Second()
			rc = r.Second()
		)
		//
		if lc.Cmp(&rc) != 0 {
			// true
			return Equals(v, v)
		}
		// false
		return NotEquals(v, v)
	}
}

// Cmp implementation for Comparable interface
func (p Equality[I]) Cmp(o Equality[I]) int {
	if p.Sign != o.Sign {
		if p.Sign {
			return -1
		}
		//
		return 1
	}
	//
	if c := p.Left.Cmp(o.Left); c != 0 {
		return c
	}
	//
	return cmpRhs(p.Right, o.Right)
}

// Is implementation of Atom interface
func (p Equality[I]) Is(truth bool) bool {
	// x == x ==> truth
	// x != x ==> false
	return p.Sign == truth && p.Right.HasFirst() && p.Left.Cmp(p.Right.First()) == 0
}

// Negate this Equality (i.e. turn it from "==" to "!=" or vice-versa)
func (p Equality[I]) Negate() Equality[I] {
	return Equality[I]{!p.Sign, p.Left, p.Right}
}

func (p Equality[I]) String(mapping func(I) string) string {
	var l, r string
	//
	if mapping != nil {
		l = mapping(p.Left)
	} else {
		l = p.Left.String()
	}
	//
	if p.Right.HasFirst() && mapping != nil {
		r = mapping(p.Right.First())
	} else if p.Right.HasFirst() {
		r = p.Right.First().String()
	} else {
		bi := p.Right.Second()
		r = bi.String()
	}
	//
	if p.Sign {
		return fmt.Sprintf("%s=%s", l, r)
	}
	//
	return fmt.Sprintf("%s≠%s", l, r)
}

func cmpRhs[I array.Comparable[I]](l util.Union[I, big.Int], r util.Union[I, big.Int]) int {
	switch {
	case l.HasFirst() && r.HasSecond():
		return -1
	case l.HasSecond() && r.HasFirst():
		return 1
	case l.HasFirst():
		return l.First().Cmp(r.First())
	default:
		lbi := l.Second()
		rbi := r.Second()

		return lbi.Cmp(&rbi)
	}
}
