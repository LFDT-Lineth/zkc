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
package vm

import (
	"github.com/consensys/go-corset/pkg/schema/module"
	"github.com/consensys/go-corset/pkg/schema/register"
	"github.com/consensys/go-corset/pkg/trace"
	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/field"
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/machine"
)

// Subdivide all modules to meet a given bandwidth and maximum register width.
// This will split all registers wider than the maximum permitted width into two
// or more "limbs" (i.e. subregisters which do not exceeded the permitted
// width). For example, consider a register "r" of width u32. Subdividing this
// register into registers of at most 8bits will result in four limbs: r'0, r'1,
// r'2 and r'3 where (by convention) r'0 is the least significant.
func Subdivide[W Word[W]](cfg field.Config, wm *WordMachine[W]) *WordMachine[W] {
	// Construct a suitable limbs mapping
	var limbsMap = newLimbsMap(cfg, wm.Modules()...)
	// Invoke subdivision algorithm
	return machine.Subdivide(limbsMap, wm)
}

func newLimbsMap(config field.Config, modules ...Module) module.LimbsMap {
	var ms []register.Map = array.Map(modules, func(_ uint, m Module) register.Map {
		name := trace.ModuleName{Name: m.Name(), Multiplier: 1}
		return register.ArrayMap(name, m.Registers()...)
	})
	// NOTE: generic parameter is meaningless, and only retained for backwards
	// compatibility.
	return module.NewLimbsMap[uint](config, ms...)
}
