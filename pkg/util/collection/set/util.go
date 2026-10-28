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
package set

// IsSorted checks whether a given set of comparable items are in sorted order.
func IsSorted[S any, T Comparable[T]](items []S, fn func(S) T) bool {
	if len(items) > 0 {
		var last = fn(items[0])
		//
		for i := 1; i < len(items); i++ {
			ith := fn(items[i])
			if last.Cmp(ith) > 0 {
				return false
			}
			//
			last = ith
		}
	}
	//
	return true
}
