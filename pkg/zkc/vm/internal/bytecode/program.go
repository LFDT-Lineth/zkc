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

// Program represents a self-contained bytecode program with a given entry
// point.
type Program struct {
	// Determines the initial program counter position when executing the given
	// bytecode sequence.
	entry uint
	// The bytecode sequence itself.
	bytecodes []uint32
}

// NewProgram constructs a new bytecode program with a given entry point.
func NewProgram(entry uint, bytecodes []uint32) Program {
	return Program{
		entry, bytecodes,
	}
}
