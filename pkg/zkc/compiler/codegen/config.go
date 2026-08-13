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
)

// DEFAULT_MAX_STATIC_HEIGHT is the default maximum height (i.e. number of rows) of
// static tables, used when no override is supplied.  It is 2^16.
const DEFAULT_MAX_STATIC_HEIGHT uint = 65536

// DEFAULT_CONFIG is the configuration used when no overrides are supplied.
// Vectorisation is enabled, which matches the behaviour expected by the
// downstream prover; callers wanting to disable individual passes (for
// debugging, for example) should derive a custom Config via the chainable
// setters below.
var DEFAULT_CONFIG = Config{
	field:           field.KOALABEAR_16,
	maxStaticHeight: DEFAULT_MAX_STATIC_HEIGHT,
	verbose:         false,
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
	// verbose controls whether printf statements are emitted as VM debug
	// instructions during code generation, or skipped (the default).
	verbose bool
	// maxStaticHeight controls the maximum height (i.e. number of rows) of
	// static tables. This is used to limit the size of static tables, as
	// required by the prover. It defaults to 2^16.
	maxStaticHeight uint
}

// Field sets the target field configuration to use for this compiler.
func (p Config) Field(field field.Config) Config {
	p.field = field
	//
	return p
}

// GetField returns the specified field configuration.
func (p Config) GetField() field.Config {
	return p.field
}

// MaxStaticHeight sets the maximum height (ie nb of rows) of static tables.
func (p Config) MaxStaticHeight(height uint) Config {
	var q = p
	//
	q.maxStaticHeight = height
	//
	return q
}

// GetMaxStaticHeight returns the maximum height (ie nb of rows) of static tables.
func (p Config) GetMaxStaticHeight() uint {
	return p.maxStaticHeight
}

// Verbose returns a copy of this Config where printf statements are emitted
// during code generation when flag=true, and skipped otherwise.
func (p Config) Verbose(flag bool) Config {
	p.verbose = flag
	//
	return p
}
