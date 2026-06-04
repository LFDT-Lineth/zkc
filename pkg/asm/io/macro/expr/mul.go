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
package expr

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/math"
)

// Mul represents an expresion which computes the product of one or more terms.
type Mul struct {
	Exprs []Expr
}

// Equals implementation for the Expr interface.
func (p *Mul) Equals(e Expr) bool {
	if e, ok := e.(*Mul); ok {
		return EqualsAll(p.Exprs, e.Exprs)
	}
	//
	return false
}

// Eval implementation for the Expr interface.
func (p *Mul) Eval(env []big.Int) big.Int {
	var result big.Int
	//
	for i, e := range p.Exprs {
		ith := e.Eval(env)

		if i == 0 {
			result.Set(&ith)
		} else {
			result.Mul(&result, &ith)
		}
	}
	// Done
	return result
}

// Polynomial implementation for the Expr interface.
func (p *Mul) Polynomial() agnostic.StaticPolynomial {
	var result agnostic.StaticPolynomial
	//
	for i, e := range p.Exprs {
		ith := e.Polynomial()
		//
		if i == 0 {
			result = ith
		} else {
			result = result.Mul(ith)
		}
	}
	//
	return result
}

// RegistersRead implementation for the Expr interface.
func (p *Mul) RegistersRead() bit.Set {
	var reads bit.Set
	//
	for _, e := range p.Exprs {
		reads.Union(e.RegistersRead())
	}
	//
	return reads
}

// ValueRange implementation for the Expr interface.
func (p *Mul) ValueRange(mapping register.Map) math.Interval {
	var values math.Interval
	//
	for i, e := range p.Exprs {
		if i == 0 {
			values = e.ValueRange(mapping)
		} else {
			values.Mul(e.ValueRange(mapping))
		}
	}
	//
	return values
}

func (p *Mul) String(mapping register.Map) string {
	return String(p, mapping)
}
