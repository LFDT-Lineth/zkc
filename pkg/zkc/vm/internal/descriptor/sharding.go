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
package descriptor

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ShardingStrategy is used to configure parallel tracing.
type ShardingStrategy struct {
	// Function around which sharding pivots.  That is, the function for which
	// checkpoints are made.
	ShardFunction string
	// Maximum executions of the given function to execute in each shard.
	ShardSteps uint64
}

// CheckShardingStrategy checks that a given sharding strategy is valid for a
// given program, returning the module identifier of its shard function.  That
// is, the shard function must exist and be #[global], and each shard must
// contain at least one of its executions.
func CheckShardingStrategy[W word.Word[W]](pr Program[W], strategy ShardingStrategy) (uint16, error) {
	// Locate the function whose calls are to be checkpointed.
	fid, ok := pr.HasModule(strategy.ShardFunction)
	//
	if !ok {
		return math.MaxUint16, fmt.Errorf("unknown function \"%s\"", strategy.ShardFunction)
	}
	// A checkpoint records no call stack, so each shard restarts at an entry
	// to the shard function with no caller to return into.  This is only
	// sound for #[global] functions, which never return and whose calls
	// cross shards over the bus.  Any other function can yield an incomplete
	// trace (or failing call lookups), so refuse it up front.
	if f, ok := pr.Module(fid).(*Function[W]); !ok || !f.Kind().IsGlobal() {
		return fid, fmt.Errorf("cannot shard on \"%s\" (shard function must be #[global])", strategy.ShardFunction)
	} else if strategy.ShardSteps == 0 {
		return fid, fmt.Errorf("cannot shard on \"%s\" (strategy has 0 steps)", strategy.ShardFunction)
	}
	// All good
	return fid, nil
}
