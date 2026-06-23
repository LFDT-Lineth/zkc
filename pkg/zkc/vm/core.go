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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
)

// Module identifies a machine module
type Module = machine.Module

// Function contains information about an executable function in the system.  A
// function has one or more registers where: the first n registers are the input
// registers (i.e. parameters); the next m registers are the output registers
// (i.e. returns); and all remaining registers are internal (sometimes also
// referred to as computed registers).  Additionally, a function has some number
// of "instructions" which capture its semantics (i.e. intended behaviour).  The
// notion of an instruction is specifically left undefined by this interface to
// support different levels of the compilation pipeline.  For example, a
// compiled function has instructions which are simply bytes (or words) for
// efficient execution.  However, the instructions of an "assembly" level
// function implement the Instruction interface, which is better suited to
// analysis and/or translation into constraints.
type Function[I instruction.Instruction] = function.Function[I]

// Vector instructions are instructions composed of some number of micro
// instructions which, with restrictions, can be executed by the underlying
// machine "in parallel".  The approach is analoguous to the concept of
// "Very-Long Instruction Words (VLIW)" but taken to more of an extreme ---
// there is no limit on the number of micro-instructions.
//
// To better understand vector instructions, consider two instructions executed
// in sequence (the at pc location 0, the second at pc location 1):
//
// (pc=0) x = y + 1 (pc=1) z = 0
//
// When executing these instructions, there is an intermediate state after the
// first instruction is executed but before the second has been where x has been
// written but z has not.  Alternatively, the two instructions can be composed
// together to form a vector instruction, written like so:
//
// (pc=0) x = y + 1 ; z = 0
//
// In this case, both instructions are executed together and there is no
// intermediate state where x is written but z is not.
//
// To ensure easy translation into polynomial constraints, there are
// restrictions on how vector instructions can be composed.  In particular, no
// variable can be assigned twice on the same execution path.  Thus, for
// example, this is an invalid vector instruction:
//
// (pc=0) x = 0 ; x = 1
//
// These writes are said to be _conflicting_.  In contrast, the following is a
// valid vector instruction:
//
// (pc=0) skip_if x != y 2 ; r = 0 ; ret ; r = 1 ; ret
//
// In this case, whilst there are two assignments to register r, neither are on
// the same path.  These writes are said to be _non-conflicting_.  Finally, we
// should note that register forwarding is applied within vector instructions.
// Thus, for example, the following is allowed:
//
// (pc=0) x = 0; y = x + 1; ret
//
// Here, the value of x written in the instruction is "forwarded" to the
// assignment for y.  This process is, roughly speaking, analoguous to register
// forwarding as found in CPU architectures.
type Vector[I Instruction] = instruction.Vector[I]

// Instruction characterises the kinds of instructions which can be
// vectorized.  They key is that, whilst many instructions are also micro
// instructions, this is not always the case.  Specifically, there are
// instructions which are not valid micro-instructions and, likewise,
// micro-instructions which are not valid instructions.
type Instruction = instruction.Instruction
