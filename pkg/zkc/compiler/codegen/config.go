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

import (
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// DEFAULT_MAX_STATIC_DEPTH is the default maximum depth (i.e. number of rows) of
// static tables, used when no override is supplied.  It is 2^16.
const DEFAULT_MAX_STATIC_DEPTH uint = 65536

// DEFAULT_CONFIG is the configuration used when no overrides are supplied.
// Vectorisation is enabled, which matches the behaviour expected by the
// downstream prover; callers wanting to disable individual passes (for
// debugging, for example) should derive a custom Config via the chainable
// setters below.
var DEFAULT_CONFIG = Config{
	field: field.KOALABEAR_16,
	// NOTE: this should be deprecated to u64 at some point.
	word:           vm.WORD_UINT128,
	fastMode:       false,
	inlining:       true,
	quiet:          true,
	splitting:      true,
	maxStaticDepth: DEFAULT_MAX_STATIC_DEPTH,
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
	// word provides information about the target word.  That is, in fast mode,
	// the machine word to be used for execution.  This must be larger than the
	// field (for now).
	word vm.WordConfig
	// fastMode execution is useful to harvest partial trace information,
	// such as memory access, module call, etc..., but can't be used
	// to generate the trace witness nor to generate arithmetic constraints.
	// It is defaulted to false.
	fastMode bool
	// inlining controls whether functions marked with the #[inline] annotation
	// are inlined at their call sites (and removed).  This happens before
	// native lowering and vectorisation.
	inlining bool
	// quiet controls whether printf statements are emitted as VM debug
	// instructions or skipped during code generation.
	quiet bool
	// splitting controls whether or not register splitting is enabled.
	splitting bool
	// maxStaticDepth controls the maximum depth (i.e. number of rows) of static tables.
	// This is used to limit the size of static tables, as required by the prover.
	// It defaults to 2^16.
	maxStaticDepth uint
}

// Field sets the target field configuration to use for this compiler.
func (p Config) Field(field field.Config) Config {
	var q = p
	//
	q.field = field
	//
	return q
}

// Word sets the target word configuration to use for this compiler.
func (p Config) Word(word vm.WordConfig) Config {
	var q = p
	//
	p.word = word
	//
	return q
}

// GetField returns the specified field configuration.
func (p Config) GetField() field.Config {
	return p.field
}

// MaxStaticDepth sets the maximum depth (ie nb of rows) of static tables.
func (p Config) MaxStaticDepth(depth uint) Config {
	var q = p
	//
	q.maxStaticDepth = depth
	//
	return q
}

// GetMaxStaticDepth returns the maximum depth (ie nb of rows) of static tables.
func (p Config) GetMaxStaticDepth() uint {
	return p.maxStaticDepth
}

// Inlining returns a copy of this Config in which function inlining is either
// enabled (flag=true) or disabled (flag=false).  When enabled, functions
// marked with the #[inline] annotation are inlined at their call sites.
func (p Config) Inlining(flag bool) Config {
	var q = p
	//
	q.inlining = flag
	//
	return q
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

// FastMode returns a copy of this Config with fast-mode execution enabled (flag=true) or disabled (flag=false).
func (p Config) FastMode(flag bool) Config {
	var q = p
	//
	q.fastMode = flag
	//
	return q
}

// IsFastMode determines whether or not fast mode is enabled.
func (p Config) IsFastMode() bool {
	return p.fastMode
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
