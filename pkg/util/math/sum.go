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
package math

// Sum an array of numbers
func Sum[T uint8 | uint16 | uint32 | uint64 | uint | int8 | int16 | int32 | int64 | int](items ...T) T {
	var sum T
	//
	for _, item := range items {
		sum += item
	}
	//
	return sum
}
