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
	"github.com/consensys/go-corset/pkg/zkc/vm/internal/trace"
)

// Observer is a generic interface for extract information before and after an
// execution step of the VM.  For example, to generate debugging information.
type Observer[W MachineWord[W], I Instruction, M Machine[W, I]] = trace.Observer[W, I, M]

// BaseObserver is an observer for a base machin
type BaseObserver[W Word[W]] = trace.Observer[W, WordInstruction, *WordMachine[W]]

// EmptyBaseObserver is an empty observer for a base machine.
type EmptyBaseObserver = trace.EmptyObserver[Uint, WordInstruction, *WordMachine[Uint]]

// TraceObserver is an observer which can be used to extract a full trace.
type TraceObserver[W Word[W], I Instruction, M Machine[W, I]] = trace.FullObserver[W, I, M]
