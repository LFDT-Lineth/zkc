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
package zkc

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/ir/air"
	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
)

// F is an arbitrary concrete field: numColumns is a purely structural measure,
// so the choice of field is immaterial.
type F = koalabear.Element

// access constructs a reference to register r shifted by the given amount.
func access(r uint, shift int) air.Term[F] {
	return term.NewRegisterAccess[F, air.Term[F]](register.NewId(r), 8, shift)
}

// vanishing wraps a term as the vanishing constraint "term == 0".
func vanishing(t air.Term[F]) air.VanishingConstraint[F] {
	return air.NewVanishingConstraint("test", 0, util.None[int](), t)
}

// Test_StatsNumColumns checks that numColumns counts distinct (column,
// shift) pairs, i.e. that a shifted access counts as a column of its own and
// that repeated accesses to the same cell count once.  The cases are those
// given in issue #2132.
func Test_StatsNumColumns(t *testing.T) {
	var (
		a    = access(0, 0)
		aMin = access(0, -1)
		b    = access(1, 0)
		c    = access(2, 0)
		cPls = access(2, 1)
		one  = term.Const64[F, air.Term[F]](1)
	)
	//
	tests := []struct {
		name     string
		term     air.Term[F]
		expected uint
	}{
		// A · B = C
		{"A.B=C", term.Subtract(term.Product(a, b), c), 3},
		// A · B = C[+1]
		{"A.B=C[+1]", term.Subtract(term.Product(a, b), cPls), 3},
		// A · (1 - A) = 0
		{"A.(1-A)=0", term.Product(a, term.Subtract(one, a)), 1},
		// A[-1] · (1 - A) = 0
		{"A[-1].(1-A)=0", term.Product(aMin, term.Subtract(one, a)), 2},
		// Constants alone read nothing.
		{"1=0", one, 0},
	}
	//
	for _, tc := range tests {
		if n := numColumns(vanishing(tc.term)); n != tc.expected {
			t.Errorf("%s: expected %d columns, got %d", tc.name, tc.expected, n)
		}
	}
}
