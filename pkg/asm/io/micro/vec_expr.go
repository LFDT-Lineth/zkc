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
package micro

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// VecExpr represents an vectorizeable expression at the micro level, which is
// either a vector of register accesses or a constant.
type VecExpr struct {
	util.Union[register.Vector, big.Int]
}

// NewVecExpr constructs an vectorizable expression from a register vector.
func NewVecExpr(regs register.Vector) VecExpr {
	//
	return VecExpr{util.Union1[register.Vector, big.Int](regs)}
}

// ConstVecExpr constructs an vectorizable expression from a constant.
func ConstVecExpr(c big.Int) VecExpr {
	//
	return VecExpr{util.Union2[register.Vector](c)}
}

// Bitwidth returns the minimum number of bits required to store any evaluation
// of this expression.
func (e VecExpr) Bitwidth(fn register.Map) uint {
	if e.HasFirst() {
		return e.First().BitWidth(fn)
	}
	//
	var val = e.Second()
	//
	return uint(val.BitLen())
}

// Eval evaluates a set of zero or more expressions producing a set of zero or
// more values.
func (e VecExpr) Eval(state io.State) *big.Int {
	var (
		val    big.Int
		offset uint
	)
	//
	if e.HasSecond() {
		val = e.Second()
		return &val
	}
	// evaluate vector
	for _, rid := range e.First().Registers() {
		var (
			reg = state.Registers()[rid.Unwrap()]
			ith big.Int
		)
		// Load & clone ith value
		ith.Set(state.Load(rid))
		// Shift into position
		ith.Lsh(&ith, offset)
		// Include in total
		val.Add(&val, &ith)
		//
		offset += reg.Width()
	}
	//
	return &val
}

func (e VecExpr) String(fn register.Map) string {
	if e.HasFirst() {
		return e.First().String(fn)
	}
	//
	val := e.Second()
	//
	return val.String()
}

// Split this vectorizable expression according to a given limbs mapping.
func (e VecExpr) Split(mapping register.LimbsMap) VecExpr {
	if e.HasSecond() {
		return e
	}
	//
	return VecExpr{util.Union1[register.Vector, big.Int](e.First().Split(mapping))}
}
