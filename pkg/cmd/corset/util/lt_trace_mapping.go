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
package util

import (
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// BIG_WORD is a pretend field configuration matching word.BigEndian.
var BIG_WORD = field.Config{Name: "BigWord", BandWidth: math.MaxUint, RegisterWidth: math.MaxUint}

// BigWordMapping constructs a limbs map for word.BigEndian values.
func BigWordMapping(ltf lt.TraceFile) module.LimbsMap {
	var modules = make([]dummyModule, ltf.Width())
	//
	for i, ith := range ltf.RawModules() {
		modules[i] = newDummyModule(ith)
	}
	//
	return module.NewLimbsMap[word.BigEndian](BIG_WORD, modules...)
}

type dummyModule struct {
	name      trace.ModuleName
	registers []register.Register
}

func newDummyModule(module lt.Module[word.BigEndian]) dummyModule {
	var (
		registers = make([]register.Register, len(module.Columns))
		zero      big.Int
	)
	//
	for i, ith := range module.Columns {
		registers[i] = register.NewComputed(ith.Name(), ith.Data().BitWidth(), zero)
	}
	//
	return dummyModule{module.Name(), registers}
}

func (p dummyModule) Name() trace.ModuleName {
	return p.name
}

func (p dummyModule) HasRegister(name string) (register.Id, bool) {
	for i, ith := range p.registers {
		if name == ith.Name() {
			return register.NewId(uint(i)), true
		}
	}
	//
	return register.UnusedId(), false
}

func (p dummyModule) Register(rid register.Id) register.Register {
	return p.registers[rid.Unwrap()]
}

func (p dummyModule) Registers() []register.Register {
	return p.registers
}

func (p dummyModule) String() string {
	panic("todo")
}
