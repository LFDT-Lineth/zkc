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

// Debug is currently a bytecode no-op used to preserve executable DEBUG sites.
type Debug struct{}

func (p *Debug) String(mapping SystemMap) string {
	return "debug"
}

// Codes implementation for Bytecode interface.
func (p *Debug) Codes(_ uint32) []uint32 {
	return []uint32{DEBUG}
}

// NewDebug constructs a no-op debug bytecode.
func NewDebug() *Debug {
	return &Debug{}
}
