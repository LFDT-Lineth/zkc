// Copyright Consensys Incorporated
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
	"github.com/LFDT-Lineth/zkc/pkg/util"
	log "github.com/sirupsen/logrus"
)

// TestMain manages resources for all tests
func TestMain(m *testing.M) {
	// Access default executor
	var executor = test_util.GOGEN_MANAGER
	// Register proper exit handler
	util.RegisterExitHandler(executor.Close)
	// Run all tests in the package
	exitCode := m.Run()
	// Exit with the test suite's status code
	log.Exit(exitCode)
}
