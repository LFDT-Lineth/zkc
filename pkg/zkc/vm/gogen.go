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
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/gogen"
)

// WordToGoSource generates a self-contained Go program (package main) that
// executes the given (u64) word machine natively.  It is an alternative "fast
// executor" to the bytecode interpreter: where WordToBytecodeInterpreter builds a
// decode-dispatch loop, this emits straight-line Go that a compiler can optimise.
// The result exposes run(in) (out, error) plus a JSON stdin/stdout main, so it can
// be compiled and run as a subprocess for differential testing and benchmarking.
func WordToGoSource(wm *WordMachine[Uint64]) (string, error) {
	return gogen.WordToGoSource(wm)
}
