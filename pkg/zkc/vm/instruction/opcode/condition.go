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
package opcode

// Condition represents the set of permission comparitors for a SkipIf
// instruction.
type Condition uint

const (
	// EQ indicates an equality condition
	EQ Condition = 0
	// NEQ indicates a non-equality condition
	NEQ Condition = 1
	// LT indicates a less-than condition
	LT Condition = 2
	// GT indicates a greater-than condition
	GT Condition = 3
	// LTEQ indicates a less-than-or-equals condition
	LTEQ Condition = 4
	// GTEQ indicates a greater-than-or-equals condition
	GTEQ Condition = 5
)
