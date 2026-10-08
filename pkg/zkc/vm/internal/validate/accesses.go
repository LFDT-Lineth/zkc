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
package validate

import (
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// RegisterAccess represents a register read with a possible bitwidth
// expectation.  For example, for a function f(u16), the register x is read with
// an expected u16 bitwidth in a function call f(x).  Thus, if the register X
// has a bitwidth at that point of e.g. u17, then an error should be reported.
type RegisterAccess struct {
	// Id of register being accessed.
	id RegisterId
	// Check indicates whether or not to perform a bounds check
	check bool
	// Expected bitwidth of register (where none indicates native register).
	expected RegWidth
}

// accessesOf returns the set of register accesses for a given bytecode.  The
// purpose of this is to provide a generic mechanism for argument checking.
func accessesOf[W word.Word[W]](bc Bytecode[W], f *descriptor.Function[W], env Environment[W]) []RegisterAccess {
	switch bc := bc.(type) {
	case *bytecode.CheckCast[W]:
		// Return access whilst skipping the bounds check so as to simply ensure
		// the register is defined.
		return []RegisterAccess{{bc.Target, false, util.None[uint]()}}
	case *bytecode.Call[W]:
		var m = env.Module(bc.Target).Unwrap().(*descriptor.Function[W])
		// For an unsafe call, we don't check the arguments
		if m.HasUnsafeArgs() {
			return nil
		}
		//
		return reads(bc.Arguments, f, m.Inputs()...)
	case *bytecode.ReadWrite[W]:
		var (
			m      = env.Module(bc.Id).Unwrap().(*descriptor.Memory[W])
			stamps = reads(bc.Stamp, f)
			rds    = reads(bc.Address, f, m.Inputs()...)
		)
		// Include data lines for writes
		if bc.Write {
			var writes = reads(bc.Data, f, m.Outputs()...)
			//
			rds = append(rds, writes...)
		}
		//
		return append(stamps, rds...)
	case *bytecode.Ret[W]:
		// For a return bytecode, force the output registers of the enclosing
		// function to be checked.
		var rds = make([]RegisterAccess, f.NumOutputs())
		//
		for i := range f.NumOutputs() {
			id := RegisterId(i + f.NumInputs())
			rds[i] = RegisterAccess{id, true, f.Register(id).Bitwidth()}
		}
		//
		return rds
	}
	//
	return reads(bc.Uses(), f)
}

// Construct appropriate register accesses for a given set of arguments, each of
// which may have an optional target.  If the target exists, the resulting
// access has least bitwidth of either.  Otherwise, it has the declared bitwidth.
func reads[W word.Word[W]](args []RegisterId, m descriptor.Module[W], targets ...descriptor.Register[W],
) []RegisterAccess {
	var accesses = make([]RegisterAccess, len(args))
	//
	for i, arg := range args {
		var expected = m.Register(arg).Bitwidth()
		// Check for matching target
		if i < len(targets) {
			expected = minWidth(expected, targets[i].Bitwidth())
		}
		//
		accesses[i] = RegisterAccess{arg, true, expected}
	}
	//
	return accesses
}
