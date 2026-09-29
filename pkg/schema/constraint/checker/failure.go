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
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// rowFailure captures what is common to every failure which arises on a
// specific row of a specific shard (e.g. vanishing, lookup and range failures).
// It provides the Handle() and Trace() parts of the schema.Failure interface,
// leaving Message() and RequiredCells() to the embedding type.
type rowFailure[F field.Element[F]] struct {
	// Handle of the failing constraint
	handle string
	// row on which the constraint failed
	row uint
	// shardId (index) on which the constraint failed
	shardId uint
	// shard (data) on which the constraint failed
	shard trace.Shard[F]
}

func newRowFailure[F field.Element[F]](handle string, row uint, cp ConstraintProcessor[F]) rowFailure[F] {
	return rowFailure[F]{handle, row, cp.shardId, cp.shard}
}

// Handle implementation of schema.Failure interface
func (p *rowFailure[F]) Handle() string {
	return p.handle
}

// Trace implementation of schema.Failure interface
func (p *rowFailure[F]) Trace() trace.Shard[F] {
	return p.shard
}
