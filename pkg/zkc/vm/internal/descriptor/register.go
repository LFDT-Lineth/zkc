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
package descriptor

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// BitwidthOf returns the accumulated bitwidth of the given set of registers, or
// none if there exists a native register.
func BitwidthOf[W word.Word[W]](regmap RegisterMap[W], regs ...RegisterId) util.Option[uint] {
	var bitwidth uint
	//
	for _, r := range regs {
		bw := regmap.Register(r).Bitwidth()
		//
		if bw.IsEmpty() {
			return bw
		}
		//
		bitwidth += bw.Unwrap()
	}
	//
	return util.Some(bitwidth)
}

// BitwidthOfRegisters returns the accumulated bitwidth of the given set of
// registers, or none if there exists a native register.
func BitwidthOfRegisters[W word.Word[W]](regs ...Register[W]) util.Option[uint] {
	var bitwidth uint
	//
	for _, r := range regs {
		bw := r.Bitwidth()
		//
		if bw.IsEmpty() {
			return bw
		}
		//
		bitwidth += bw.Unwrap()
	}
	//
	return util.Some(bitwidth)
}

// ToRegisters converts an array of register descriptors into an array of scheme
// registers.
func ToRegisters[W word.Word[W]](registers ...Register[W]) []register.Register {
	return array.Map(registers,
		func(_ uint, r Register[W]) register.Register { return r.ToRawRegister() })
}

// RegisterMap provides a generic interface for entities which hold information
// about registers.
type RegisterMap[W word.Word[W]] = bytecode.RegisterMap[W]

// Register represents an individual register in a module that, eventually, will
// be mapped to one (or more) columns in the trace.  Likewise, a single register
// can end up being mapped across multiple columns as a result of register
// splitting  to ensure field agnosticity. Hence, why they are referred to as
// registers rather than columns --- they are similar, but not identical,
// concepts.
type Register[W word.Word[W]] = bytecode.Register[W]

// NewInputRegister constructs a new input register descriptor.
func NewInputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	var safe = bitwidth.HasValue()
	return bytecode.NewRegister(bytecode.INPUT_REGISTER.WithSafety(safe), name, bitwidth, padding)
}

// NewOutputRegister constructs a new output register descriptor.
func NewOutputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	var safe = bitwidth.HasValue()
	return bytecode.NewRegister(bytecode.OUTPUT_REGISTER.WithSafety(safe), name, bitwidth, padding)
}

// NewStampInputRegister constructs a new (stamp) input register descriptor.
func NewStampInputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	var safe = bitwidth.HasValue()
	return bytecode.NewRegister(bytecode.INPUT_REGISTER.WithSafety(safe).WithStamp(true), name, bitwidth, padding)
}

// NewStampOutputRegister constructs a new (stamp) output register descriptor.
func NewStampOutputRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	var safe = bitwidth.HasValue()
	return bytecode.NewRegister(bytecode.OUTPUT_REGISTER.WithSafety(safe).WithStamp(true), name, bitwidth, padding)
}

// NewComputedRegister constructs a new computed (i.e. internal) register descriptor.
func NewComputedRegister[W word.Word[W]](name string, bitwidth util.Option[uint], padding W) Register[W] {
	var safe = bitwidth.HasValue()
	return bytecode.NewRegister(bytecode.COMPUTED_REGISTER.WithSafety(safe), name, bitwidth, padding)
}
