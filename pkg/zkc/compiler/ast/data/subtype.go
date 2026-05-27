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
	"github.com/consensys/go-corset/pkg/util"
	"github.com/consensys/go-corset/pkg/zkc/compiler/ast/symbol"
)

// SubtypeOf performs a subtype check, denoted "t1 <: t2", in a given typing
// environment.  A subtype check can be view as a subset relationship, where "t1
// <: t2" is read as saying: the set of values in t1 is a subset of those in t2.
// For example, it follows that "u4 <: u8" holds since "{0..15} ⊆ {0..255}"
// holds.  Furthermore, "u4 <: u1" does not hold as "{0..15} ⊆ {0..1}" does not.
// Finally, the subtype operator follows the general algebraic properties of the
// subset operator.  For example, it is reflesive and, hence, "u4 <: u4" holds.
//
// The only real challenge is the presence of open (i.e. existential) types such
// as "u4+". For example, does "u4+ <: u8" hold?   To understand this, we can
// interpret "u4+" as saying "this can be any type which is at least a u4" Thus,
// the query "u4+ <: u8" can be read as saying "is there a type which is at
// least a u4 that is a subtype of u8?".  The answer, of course, is yes: u8.
//
// NOTE: this function assumes that t1 and t2 are well-formed under the given
// environment.
func SubtypeOf[S symbol.Symbol[S]](t1, t2 Type[S], env Environment[S]) bool {
	// Resolve alias types so we compare underlying types from the Ref.
	if at2 := t2.AsAlias(env); at2 != nil {
		return SubtypeOf(t1, at2.Resolve(env), env)
	}
	//
	switch t1 := t1.(type) {
	case *UnsignedInt[S]:
		if t, isUint := t2.(*UnsignedInt[S]); isUint {
			return t1.BitWidth() <= t.BitWidth()
		} else if _, isField := t2.(*FieldElement[S]); isField {
			return true
		}
	case *Alias[S]:
		if at1 := t1.AsAlias(env); at1 != nil {
			return SubtypeOf(at1.Resolve(env), t2, env)
		}
	case *Tuple[S]:
		if t := t2.AsTuple(env); t != nil {
			if t1.Width() != t.Width() {
				return false
			}
			//
			for i := range t1.Width() {
				if !SubtypeOf(t1.Ith(i), t.Ith(i), env) {
					return false
				}
			}
			//
			return true
		}
	case *FieldElement[S]:
		if _, isUint := t2.(*UnsignedInt[S]); isUint {
			return true
		} else if _, isField := t2.(*FieldElement[S]); isField {
			return true
		}
	case *FixedArray[S]:
		if t, ok := t2.(*FixedArray[S]); ok {
			if !arraySizeEquals(t1.Size, t.Size) {
				return false
			}

			return SubtypeOf(t1.DataType, t.DataType, env)
		}
	}

	return false
}

// EquiTypes performs an equivalence check, denoted "t1 ~ t2", in a given typing
// environment.  In most cases, this amounts to check that the two types are
// equivalent.  For example, "u8 ~ u8" holds but "u4 ~ u8" does not.  The only
// real challenge is the presence of open (i.e. existential) types such as
// "u8+".  For such types, we have that e.g. "u8 ~ u4+" hold and, likewise, that
// "u8+ ~ u16".  To understand this, we can interpret "u8+" as saying "this can
// be any type which is at least a u8" Thus, the query "u8+ ~ u16" can be read
// as saying "is there a type which is at least a u8 equivalent to a u16?".  The
// answer, of course, is yes: u16. However, we note that "u16+ ~ u8" does not
// hold.
//
// NOTE: this function assumes that t1 and t2 are well-formed under the given
// environment.
func EquiTypes[S symbol.Symbol[S]](t1, t2 Type[S], env Environment[S]) bool {
	// Resolve alias types so we compare underlying types from the Ref.
	if at2 := t2.AsAlias(env); at2 != nil {
		return EquiTypes(t1, at2.Resolve(env), env)
	}

	switch t1 := t1.(type) {
	case *UnsignedInt[S]:
		if t, isUint := t2.(*UnsignedInt[S]); isUint {
			return t1.BitWidth() == t.BitWidth() ||
				(t1.IsOpen() && t1.BitWidth() <= t.BitWidth()) ||
				(t.IsOpen() && t.BitWidth() <= t1.BitWidth())
		}
	case *Alias[S]:
		if at1 := t1.AsAlias(env); at1 != nil {
			return EquiTypes(at1.Resolve(env), t2, env)
		}
	case *Tuple[S]:
		if t := t2.AsTuple(env); t != nil {
			if t1.Width() != t.Width() {
				return false
			}
			//
			for i := range t1.Width() {
				if !EquiTypes(t1.Ith(i), t.Ith(i), env) {
					return false
				}
			}
			//
			return true
		}
	case *FieldElement[S]:
		_, isField := t2.(*FieldElement[S])
		return isField
	case *FixedArray[S]:
		if t := t2.AsFixedArray(env); t != nil {
			if !arraySizeEquals(t1.Size, t.Size) {
				return false
			}

			return EquiTypes(t1.DataType, t.DataType, env)
		}
	}

	return false
}

// Check whether two array sizes are equal.  This only matches symbols exactly
// and, therefore, two arrays with symbolic sizes that are the same, but differ
// in name are considered different.
func arraySizeEquals[S symbol.Symbol[S]](lhs, rhs util.Union[uint, S]) bool {
	if lhs.HasFirst() && rhs.HasFirst() {
		return lhs.First() == rhs.First()
	} else if lhs.HasSecond() && rhs.HasSecond() {
		return lhs.Second().Cmp(rhs.Second()) == 0
	}
	//
	return false
}
