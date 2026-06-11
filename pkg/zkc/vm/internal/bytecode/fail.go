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
package bytecode

// Fail instruction
type Fail struct{}

func (p *Fail) String(_ SystemMap) string {
	return "fail"
}

// Codes implementation for Bytecode interface
func (p *Fail) Codes(_ uint32) []uint32 {
	return []uint32{FAIL}
}
