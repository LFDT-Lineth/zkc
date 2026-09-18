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
package util

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/ir"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// TestCase represents a single test case for a ZkC program with a given test
// vector, and one or more configuration options.
type TestCase struct {
	test string
	// vector which underpins this test
	vector TestVector
	// build config
	build codegen.Config
	// program to execute
	program vm.Program[vm.Uint]
	// padding strategy to use for this test
	padding util.Pair[string, ir.PaddingStrategy]
	// check fastMode
	fastMode bool
	// check constraints (which requires tracing)
	constraints bool
	// check marshalling
	marshal bool
	// sharding configuration
	sharding util.Option[vm.ShardingStrategy]
	// check gogen
	gogen util.Option[string]
}

// NewTestCase constructs a new test case
func NewTestCase(test string, vector TestVector, build codegen.Config, prog vm.Program[vm.Uint]) TestCase {
	return TestCase{test: test, vector: vector, build: build, program: prog, padding: DEFAULT_PADDING}
}

// Constraints indicates this is a tracinbg / constraint checking.
func (p TestCase) Constraints() TestCase {
	p.constraints = true
	return p
}

// FastMode indicates this is a fast-mode only test
func (p TestCase) FastMode() TestCase {
	p.fastMode = true
	return p
}

// Handle returns a suitable descriptor for this test case.
func (p TestCase) Handle() string {
	var (
		filename = strings.TrimPrefix(strings.TrimPrefix(p.vector.filename, TestDir), "/")
		handle   = fmt.Sprintf("%s.zkc(%s:%d);%s", p.test, filename, p.vector.line, p.build.GetField().Name)
	)
	//
	if p.fastMode {
		handle = fmt.Sprintf("%s;fastmode", handle)
	}
	//
	if p.gogen.HasValue() {
		handle = fmt.Sprintf("%s;gogen", handle)
	}
	//
	if p.constraints {
		handle = fmt.Sprintf("%s;constraints", handle)
	}
	//
	if p.marshal {
		handle = fmt.Sprintf("%s;marshal", handle)
	}
	//
	handle = fmt.Sprintf("%s;height=%d", handle, p.build.GetMaxStaticHeight())
	//
	handle = fmt.Sprintf("%s;%s", handle, p.padding.Left)
	//
	return handle
}

// GoGen turns on gogen with the given binary
func (p TestCase) GoGen(binary string) TestCase {
	p.gogen = util.Some(binary)
	return p
}

// Marshalling indicates binary marshalling / unmarshalling should be tested.
func (p TestCase) Marshalling() TestCase {
	p.marshal = true
	return p
}

// Padding sets a specific padding strategy to use.
func (p TestCase) Padding(padding util.Pair[string, ir.PaddingStrategy]) TestCase {
	p.padding = padding
	return p
}

// Sharding sets a specific sharding strategy to use.
func (p TestCase) Sharding(sharding util.Option[vm.ShardingStrategy]) TestCase {
	p.sharding = sharding
	return p
}
