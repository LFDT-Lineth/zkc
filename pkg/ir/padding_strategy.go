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

// PaddingStrategy determines how much front padding is added to each module
// when expanding a trace (see TraceBuilder.WithPadding).  The zero value is
// NextPowerOfTwoPadding, making it the default strategy.
type PaddingStrategy uint

const (
	// NextPowerOfTwoPadding expands every module's (logical) height up to the
	// next power of two.  An empty module is expanded to a height of one; a
	// module whose height is already a (non-zero) power of two is left
	// unchanged.  This is the default strategy.
	NextPowerOfTwoPadding PaddingStrategy = iota
	// NoPadding leaves each module's height unchanged.
	NoPadding
	// SingleRowPadding prepends exactly one (logical) padding row to each
	// module.
	SingleRowPadding
)

// PADDING_STRATEGIES maps the name of each strategy (as accepted on the command
// line) to its value.
var PADDING_STRATEGIES = map[string]PaddingStrategy{
	"none":              NoPadding,
	"single-row":        SingleRowPadding,
	"next-power-of-two": NextPowerOfTwoPadding,
}

// GetPaddingStrategy resolves a strategy name (e.g. as supplied on the command
// line) to its value, returning false if the name is not recognised.
func GetPaddingStrategy(name string) (PaddingStrategy, bool) {
	strategy, ok := PADDING_STRATEGIES[name]
	return strategy, ok
}
