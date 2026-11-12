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
package field

// Pow takes a given value to the power n.
func Pow[F Element[F]](val F, n uint64) F {
	if n == 0 {
		val = val.SetUint64(1)
	} else if n > 1 {
		m := n / 2
		// Check for odd case
		if n%2 == 1 {
			tmp := val
			val = Pow(val, m)
			val = val.Mul(val).Mul(tmp)
		} else {
			// Even case is easy
			val = Pow(val, m)
			val = val.Mul(val)
		}
	}
	//
	return val
}
