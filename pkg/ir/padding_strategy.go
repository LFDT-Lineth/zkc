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
package ir

import util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"

// PaddingStrategy captures the notion of an algorithm that determines how much front padding is added to each module
// when expanding a trace (see TraceBuilder.WithPadding).
type PaddingStrategy = func(height, multiplier uint) uint

// PADDING_STRATEGIES maps the name of each strategy (as accepted on the command
// line) to its value.
var PADDING_STRATEGIES = map[string]PaddingStrategy{
	"none":              NaryRowPadding(0),
	"single-row":        NaryRowPadding(1),
	"double-row":        NaryRowPadding(2),
	"triple-row":        NaryRowPadding(3),
	"next-power-of-two": NextPowerOfTwoPadding,
}

// NextPowerOfTwoPadding rounds the logical height to the next power of 2.  For
// example, a height of 3 would become 4, etc.  An empty module is expanded to a
// height of one; a module whose height is already a (non-zero) power of two is
// left unchanged.
func NextPowerOfTwoPadding(height, multiplier uint) uint {
	return util_math.NextPowerOfTwo(height) * multiplier
}

// NaryRowPadding rounds up the height of a module by a given number of complete
// rows.
func NaryRowPadding(n uint) PaddingStrategy {
	return func(height, multiplier uint) uint {
		return (height + n) * multiplier
	}
}

// GetPaddingStrategy resolves a strategy name (e.g. as supplied on the command
// line) to its value, returning false if the name is not recognised.
func GetPaddingStrategy(name string) (PaddingStrategy, bool) {
	strategy, ok := PADDING_STRATEGIES[name]
	return strategy, ok
}
