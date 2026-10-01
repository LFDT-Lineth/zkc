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
package stack

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
)

// Worklist provides a generic worklist structure which is suitable, for
// example, for implementing a depth-first search, etc.
type Worklist struct {
	stack   Stack[uint]
	visited bit.Set
}

// Size returns number of items remaining
func (p *Worklist) Size() uint {
	return p.stack.Len()
}

// Pop pops an item of the stack
func (p *Worklist) Pop() uint {
	return p.stack.Pop()
}

// Push pushes an item on the stack, provided that it has not been seen before.
func (p *Worklist) Push(n uint) {
	if !p.visited.Contains(n) {
		p.visited.Insert(n)
		p.stack.Push(n)
	}
}

// Visited checks whether the given value has (at some point) been pushed onto
// this stack, or not.
func (p *Worklist) Visited(n uint) bool {
	return p.visited.Contains(n)
}
