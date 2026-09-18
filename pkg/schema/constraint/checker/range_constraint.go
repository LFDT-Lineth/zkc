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
package checker

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/ranged"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

func processRangeConstraint[F field.Element[F]](cp ConstraintProcessor[F], c *ranged.Constraint[F]) []Failure[F] {
	var (
		trModule = cp.shard.Module(c.Context)
		handle   = constraint.DetermineHandle(c.Handle, c.Context, cp.shard)
		column   = trModule.Column(c.Source.Unwrap())
		// Compute 2^n
		bound    = field.TwoPowN[F](c.Bitwidth)
		failures []Failure[F]
	)
	// Iterate every row
	for k := range trModule.Height() {
		// Perform the range check
		if column.Get(k).Cmp(bound) >= 0 {
			// Evaluation failure
			failures = append(failures, &ranged.Failure[F]{
				RangeHandle: handle,
				Context:     c.Context,
				Source:      c.Source,
				Bitwidth:    c.Bitwidth,
				Row:         k,
				Shard:       cp.shardId})
		}
	}
	// All good
	return failures
}
