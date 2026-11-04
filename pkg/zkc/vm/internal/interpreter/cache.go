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
package interpreter

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/heap"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// FunCache provides, for a given function, a mapping of arguments to returns.
type funCache[W word.Word[W]] = hash.Map[hash.Array[W], []W]

// FunctionCallCache provides a simple cache for memoising function calls.  That
// is, caching the return values computed for the given inputs.  The primary
// purpose of such a cache is to eliminate duplicate rows in the execution trace
// for a given program.  A secondary purpose is to improving tracing performance
// by avoiding needless (expensive) executions.
type FunctionCallCache[W word.Word[W]] struct {
	program descriptor.Program[W]
	cache   []*funCache[W]
}

func newFunctionCallCache[W word.Word[W]](program descriptor.Program[W], tracing bool) FunctionCallCache[W] {
	var (
		cache = make([]*funCache[W], len(program.Modules()))
	)
	// Only employ the cache when tracing
	if tracing {
		for i, m := range program.Modules() {
			if f, ok := m.(*descriptor.Function[W]); ok && isCacheable(f) {
				cache[i] = hash.NewMap[hash.Array[W], []W](128)
			}
		}
	}
	//
	return FunctionCallCache[W]{program, cache}
}

// Get checks whether a matching invocation of the given function is stored in
// the cache and, if so, returns this. NOTE: the returned array (if present) is
// owned by the cache, and hence, should be copied into its final destination to
// avoid aliasing.
func (p *FunctionCallCache[W]) Get(fid uint16, fp uint32, nArgs uint32, dataStack heap.Heap[W]) util.Option[[]W] {
	var fidCache = p.cache[fid]
	//
	if fidCache != nil {
		var args = dataStack.Slice(uint(fp), uint(fp+nArgs))
		//
		if returns, ok := fidCache.Get(hash.NewArray(args)); ok {
			return util.Some(returns)
		}
	}
	//
	return util.None[[]W]()
}

// Insert allocates an invocation of the given function into the cache.  NOTE: both
// input arrays are cloned by this function and, hence, are not aliased.
func (p *FunctionCallCache[W]) Insert(fid uint16, fp uint32, dataStack heap.Heap[W]) {
	var fidCache = p.cache[fid]
	//
	if fidCache != nil {
		var (
			callee = p.program.Module(fid)
			nArgs  = callee.NumInputs()
			nRets  = callee.NumOutputs()
			// Slice out the current arguments / returns
			args = dataStack.Slice(uint(fp), uint(fp)+nArgs)
			rets = dataStack.Slice(uint(fp)+nArgs, uint(fp)+nArgs+nRets)
		)
		// Clone arguments
		args = slices.Clone(args)
		// Clone returns
		rets = slices.Clone(rets)
		// Insert into cache
		fidCache.Insert(hash.NewArray(args), rets)
	}
}

// Determine whether a given function should be considered suibable for caching
// or not.
func isCacheable[W word.Word[W]](f *descriptor.Function[W]) bool {
	return !f.Kind().IsGlobal() && !f.Kind().IsNative() &&
		len(f.Effects()) == 0 && f.NumInputs() <= 8
}
