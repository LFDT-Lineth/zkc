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
package memory

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// StaticReadOnly is a ReadOnly memory whose contents are fixed at construction
// time and are never overwritten by Boot.  Specifically, its Initialise method
// is a no-op, so the pre-loaded data survives across multiple executions of the
// same machine.
type StaticReadOnly[W util.Uinter64] struct {
	ReadOnly[W]
}

// Initialise is a no-op for static read-only memory: contents are fixed at
// construction time and must not be overwritten between executions.
func (p *StaticReadOnly[W]) Initialise(contents []W) {
	if len(contents) > 0 {
		panic("cannot initialise static read only memory")
	}
}

// NewStatic constructs a static read-only memory pre-loaded with the
// given values.
func NewStatic[W util.Uinter64](name string, public bool, registers []register.Register, init ...W,
) *StaticReadOnly[W] {
	//
	var kind Kind
	//
	if public {
		kind = PUBLIC_STATIC_MEMORY
	} else {
		kind = PRIVATE_STATIC_MEMORY
	}
	//
	return &StaticReadOnly[W]{
		ReadOnly: ReadOnly[W]{
			StaticArray: NewStaticArray(name, kind, registers, init...),
		},
	}
}
