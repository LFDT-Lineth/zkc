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
package lex

import (
	"cmp"
)

// Scanner is a function which accepts a given item or not.
type Scanner[T any] func(item []T) uint

// And combines zero or more scanners such that the resulting scanner succeeds if
// all of the scanners succeed.  Observe, however, that there is an implicit
// left-to-right order of evaluation.
func And[T any](scanners ...Scanner[T]) Scanner[T] {
	return func(items []T) uint {
		n := uint(0)

		for _, scanner := range scanners {
			m := scanner(items)
			if m == 0 {
				// fail
				return 0
			}
			//
			n = max(n, m)
		}
		// fail
		return n
	}
}

// Or combines zero or more scanners such that the resulting scanner succeeds if
// any of the scanners succeeds.  Observe, however, that there is an implicit
// left-to-right order of evaluation.
func Or[T any](scanners ...Scanner[T]) Scanner[T] {
	return func(items []T) uint {
		for _, scanner := range scanners {
			if n := scanner(items); n > 0 {
				return n
			}
		}
		// fail
		return 0
	}
}

// Unit accepts a given sequence of characters.  That is, for this scanner to
// match, it must match all the given characters (one after the other) in their given order.
func Unit[T comparable](chars ...T) Scanner[T] {
	return func(items []T) uint {
		if len(items) >= len(chars) {
			for i := 0; i < len(chars); i++ {
				if items[i] != chars[i] {
					// fail
					return 0
				}
			}
			// success
			return uint(len(chars))
		}
		// fail
		return 0
	}
}

// String expects a given string s.
// It is equivalent to [Unit](s[0], s[1], ...)
func String(s string) Scanner[int32] {
	return func(items []int32) uint {
		if len(items) < len(s) {
			return 0
		}

		for i := range s {
			if int32(s[i]) != items[i] {
				return 0
			}
		}

		return uint(len(s))
	}
}

// Not matches everything except the given item.
func Not[T comparable](item T) Scanner[T] {
	return func(items []T) uint {
		if len(items) != 0 && items[0] != item {
			return 1
		}
		// fail
		return 0
	}
}

// Within accepts any character within a given range.
func Within[T cmp.Ordered](lowest T, highest T) Scanner[T] {
	return func(items []T) uint {
		if len(items) != 0 && lowest <= items[0] && items[0] <= highest {
			return 1
		}
		// fail
		return 0
	}
}

// Many matches zero or more of a given item.
func Many[T any](acceptor Scanner[T]) Scanner[T] {
	return func(items []T) uint {
		index := uint(0)
		//
		for index < uint(len(items)) {
			if n := acceptor(items[index:]); n != 0 {
				index += n
				continue
			}
			//
			break
		}
		// done
		return index
	}
}

// Until matches everything until a particular item is matched.
func Until[T comparable](item T) Scanner[T] {
	return func(items []T) uint {
		index := uint(0)
		//
		for index < uint(len(items)) {
			if items[index] == item {
				break
			}
			// continue match
			index = index + 1
		}
		// done
		return index
	}
}

// Eof matches the end of the input stream.
func Eof[T any]() Scanner[T] {
	return func(items []T) uint {
		if len(items) == 0 {
			return 1
		}
		//
		return 0
	}
}

// SequenceNullableLast matches all the scanners in order.
// Each scanner consumes the input right after the previous one ends.
// Only the final scanner is allowed a match length of 0.
func SequenceNullableLast[T comparable](scanners ...Scanner[T]) Scanner[T] {
	return func(items []T) uint {
		n, i := uint(0), 0
		for i = range scanners {
			if n == uint(len(items)) {
				break
			}

			m := scanners[i](items[n:])
			if m == 0 {
				break
			}

			n += m
		}

		if i < len(scanners)-1 { // if we ended prematurely
			return 0
		}

		return n
	}
}

// Sequence matches all the scanners in order.
// Each scanner consumes the input right after the previous one ends.
func Sequence[T comparable](scanners ...Scanner[T]) Scanner[T] {
	return func(items []T) uint {
		n := uint(0)
		for _, scanner := range scanners {
			if n == uint(len(items)) {
				return 0
			}

			m := scanner(items[n:])
			if m == 0 {
				return 0
			}

			n += m
		}

		return n
	}
}
