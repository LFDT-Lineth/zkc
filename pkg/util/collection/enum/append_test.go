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
package enum

import (
	"testing"
)

func Test_AppendEnumerator_1(t *testing.T) {
	enum := Append(Range[uint](0, 0))
	checkEnumerator(t, enum, []uint{}, uintEquals)
}

func Test_AppendEnumerator_2(t *testing.T) {
	enum := Append(Range[uint](0, 0), Range[uint](0, 0))
	checkEnumerator(t, enum, []uint{}, uintEquals)
}

func Test_AppendEnumerator_3(t *testing.T) {
	enum := Append(Range[uint](0, 1), Range[uint](0, 0))
	checkEnumerator(t, enum, []uint{0}, uintEquals)
}

func Test_AppendEnumerator_4(t *testing.T) {
	enum := Append(Range[uint](0, 0), Range[uint](0, 1))
	checkEnumerator(t, enum, []uint{0}, uintEquals)
}

func Test_AppendEnumerator_5(t *testing.T) {
	enum := Append(Range[uint](0, 0), Range[uint](0, 1))
	checkEnumerator(t, enum, []uint{0}, uintEquals)
}

func Test_AppendEnumerator_6(t *testing.T) {
	enum := Append(Range[uint](0, 1), Range[uint](0, 1))
	checkEnumerator(t, enum, []uint{0, 0}, uintEquals)
}

func Test_AppendEnumerator_7(t *testing.T) {
	enum := Append(Range[uint](0, 1), Range[uint](0, 2))
	checkEnumerator(t, enum, []uint{0, 0, 1}, uintEquals)
}

func Test_AppendEnumerator_8(t *testing.T) {
	enum := Append(Range[uint](0, 2), Range[uint](0, 1))
	checkEnumerator(t, enum, []uint{0, 1, 0}, uintEquals)
}
