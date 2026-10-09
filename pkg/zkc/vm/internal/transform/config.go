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
package transform

// DEFAULT_CONFIG provides default configuration settings for the
// pipeline transformations.
var DEFAULT_CONFIG = Config{
	Validation: true,
	Ignores:    nil,
}

// Config embodies relevant configuration settings for pipeline transformations.
type Config struct {
	// Validation ensures bytecode validation is performed after each transform
	// is applied.
	Validation bool
	// Ignores identifies pipeline stages which should be ignored.
	Ignores []string
}

// Validate enables / disables bytecode validation.
func (p Config) Validate(flag bool) Config {
	p.Validation = flag
	return p
}

// Ignore marks a given pipeline stage to be ignored.
func (p Config) Ignore(stage string) Config {
	p.Ignores = append(p.Ignores, stage)
	return p
}
