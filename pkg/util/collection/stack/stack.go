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

// Stack represents a reusable LIFO stack which is implemented using an array.
type Stack[T any] struct {
	items []T
}

// NewStack returns an empty stack
func NewStack[T any]() *Stack[T] {
	return &Stack[T]{}
}

// AsArray returns the contents of this stack as an array
func (p *Stack[T]) AsArray() []T {
	return p.items
}

// Clear resets the heap to an empty state whilst retaining any allocated
// capacity.
func (p *Stack[T]) Clear() {
	p.items = p.items[:0]
}

// IsEmpty checks whether or not there are still items on the stack
func (p *Stack[T]) IsEmpty() bool {
	return p.Len() == 0
}

// Len returns the number of items on the stack.
func (p *Stack[T]) Len() uint {
	return uint(len(p.items))
}

// Peek at nth item from top of stack.
func (p *Stack[T]) Peek(offset uint) T {
	var n = len(p.items) - int(offset) - 1
	//
	if n < 0 {
		panic("peek out-of-bounds")
	}
	// Get last item
	return p.items[n]
}

// Push a new item onto the stack
func (p *Stack[T]) Push(item T) {
	p.items = append(p.items, item)
}

// PushAll pushes zero or more items onto the stack
func (p *Stack[T]) PushAll(item []T) {
	p.items = append(p.items, item...)
}

// Pop the last item off the stack
func (p *Stack[T]) Pop() T {
	var n = len(p.items)
	//
	if n == 0 {
		panic("cannot pop from empty stack")
	}
	// Get last item
	item := p.items[n-1]
	// Remove last item
	p.items = p.items[:n-1]
	// Done
	return item
}

// Top peeks at the last item on the stack.
func (p *Stack[T]) Top() *T {
	var n = len(p.items) - 1
	//
	if n < 0 {
		panic("top out-of-bounds")
	}
	// Get last item
	return &p.items[n]
}
