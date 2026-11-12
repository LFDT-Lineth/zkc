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
package view

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// RegisterView provides an abstract view of a given column.
type RegisterView interface {
	Get(uint) util.Option[big.Int]
	Len() uint
}

type registerView[F field.Element[F]] struct {
	trace trace.Module[F]
	regId register.Id
}

func (p *registerView[F]) Get(row uint) util.Option[big.Int] {
	var (
		data = p.trace.Column(p.regId.Unwrap())
		val  big.Int
	)
	// Sanity check something is there
	if data != nil {
		// Construct value from field element
		val.SetBytes(data.Get(row).Bytes())
		//
		return util.Some(val)
	}
	//
	return util.None[big.Int]()
}

func (p *registerView[F]) Len() uint {
	return p.trace.Height()
}
