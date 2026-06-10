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

// GoGenConfig controls the shape of the Go source GenerateGo produces.
type GoGenConfig = gogen.Config

// GenerateGo compiles a word machine into native Go source: the "fast
// execution mode" alternative to interpreting the machine.  Where
// WordToBytecodeInterpreter builds a decode-dispatch loop, this emits plain Go
// the compiler can optimise — and it consumes the machine over Uint (the same
// machine the reference executor interprets), so the generated semantics
// mirror the reference executor exactly, with no register splitting or word
// narrowing in the way.
//
// The result exposes Run(inputs) (outputs, error); with cfg.Package == "main"
// (the default) a JSON stdin/stdout harness is appended, so the artefact can
// be compiled and run as a subprocess for differential testing and
// benchmarking.
func GenerateGo(wm *WordMachine[Uint], cfg GoGenConfig) (string, error) {
	return gogen.Generate(wm, cfg)
}
