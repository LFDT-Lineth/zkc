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
package codegen

import "github.com/LFDT-Lineth/zkc/pkg/util/field"

// DEFAULT_CONFIG is the configuration used when no overrides are supplied.
// Vectorisation is enabled, which matches the behaviour expected by the
// downstream prover; callers wanting to disable individual passes (for
// debugging, for example) should derive a custom Config via the chainable
// setters below.
var DEFAULT_CONFIG = Config{
	field:          field.KOALABEAR_16,
	lowerZkcNative: false,
	quiet:          false,
	vectorize:      true,
	splitting:      false,
}

// Config captures the tunable aspects of the ZkC code generator.  Instances
// are immutable: each setter (e.g. Vectorize) returns a new Config rather
// than mutating the receiver, so a Config can be safely shared between
// concurrent compilations.
type Config struct {
	// field provides information about the target field.  There must always be
	// a target field in order to correctly evaluate native expressions, and
	// sanity check native initialisers, etc.
	field field.Config
	// lower ZkC native functions (such as bitwise ops) into arithmetic instructions.
	// This is required to generate arithmetic constraints. It happens before vectorization and register splitting.
	lowerZkcNative bool
	// quiet controls whether printf statements are emitted as VM debug
	// instructions or skipped during code generation.
	quiet bool
	// vectorize controls whether the codegen pipeline runs the
	// instruction-vectorisation pass in pkg/zkc/compiler/codegen/vectorize.go.
	// Vectorisation merges sequences of micro-instructions that have no
	// register conflicts into single (vector) macro-instructions, allowing
	// the prover to handle them in one step.  Disabling it produces a less
	// compact program but leaves the macro instruction stream identical to
	// the codegen output, which is useful when debugging the codegen or
	// inspecting the un-merged IR.
	vectorize bool
	// splitting controls whether or not register splitting is enabled.
	splitting bool
}

// Field sets the target field configuration to use for this compiler.
func (p Config) Field(field field.Config) Config {
	var q = p
	//
	q.field = field
	//
	return q
}

// GetField returns the specified field configuration.
func (p Config) GetField() field.Config {
	return p.field
}

// SplitRegisters returns a copy of this Config in which register splitting is
// either enabled (flag=true) or disabled (flag=false).
func (p Config) SplitRegisters(flag bool) Config {
	var q = p
	//
	q.splitting = flag
	//
	return q
}

// Vectorize returns a copy of this Config in which the vectorisation pass is
// either enabled (flag=true) or disabled (flag=false).  The receiver is left
// unchanged, so this can be chained: codegen.DEFAULT_CONFIG.Vectorize(false).
func (p Config) Vectorize(flag bool) Config {
	var q = p
	//
	q.vectorize = flag
	//
	return q
}

// LowerNatives returns a copy of this Config with VM-level bitwise lowering
// enabled (flag=true) or disabled (flag=false).
func (p Config) LowerNatives(flag bool) Config {
	var q = p
	//
	q.lowerZkcNative = flag
	//
	return q
}

// Quiet returns a copy of this Config where printf statements are skipped
// during code generation when flag=true.
func (p Config) Quiet(flag bool) Config {
	var q = p
	//
	q.quiet = flag
	//
	return q
}
