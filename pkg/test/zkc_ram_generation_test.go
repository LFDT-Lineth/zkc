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
package test

import (
	"testing"

	test_util "github.com/LFDT-Lineth/zkc/pkg/test/util"
)

// ===================================================================
// RAM constraint-generation tests
// ===================================================================
//
// These verify that read-write (RAM) memory programs of various shapes both
// compile to a machine and generate AIR constraints, across every field (so the
// multi-limb timestamp / address machinery is exercised under each register-width
// regime).  They use no trace files: the RAM trace observer is not yet complete,
// so full accept/reject tracing tests cannot run — but schema generation should
// still hold across these shapes.

// Several distinct reads and writes to a read-write memory.
func Test_ZkcUnit_RamConstraintGeneration_01(t *testing.T) {
	test_util.CheckConstraintGeneration(t, "zkc/unit/ram_constraint_generation_01")
}

// Read-write memory with a multi-lane (tuple) value.
func Test_ZkcUnit_RamConstraintGeneration_02(t *testing.T) {
	test_util.CheckConstraintGeneration(t, "zkc/unit/ram_constraint_generation_02")
}

// Read-write memory with a field-element value lane.
func Test_ZkcUnit_RamConstraintGeneration_03(t *testing.T) {
	test_util.CheckConstraintGeneration(t, "zkc/unit/ram_constraint_generation_03")
}

// Read-write memory with a mixed (integer + field element) tuple value.
func Test_ZkcUnit_RamConstraintGeneration_04(t *testing.T) {
	test_util.CheckConstraintGeneration(t, "zkc/unit/ram_constraint_generation_04")
}
