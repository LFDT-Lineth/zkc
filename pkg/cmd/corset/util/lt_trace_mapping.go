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
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// IdentityMapping constructs a trivial (unsplit) limbs map for a set of
// modules, where every register maps to exactly itself.  This is used in
// place of a genuine subdivision mapping now that registers are never split
// into limbs.
func IdentityMapping[F field.Element[F], M register.Map](name string, modules ...M) module.LimbsMap {
	var cfg = field.Config{Name: name, BandWidth: math.MaxUint, RegisterWidth: math.MaxUint}
	return module.NewLimbsMap[F](cfg, modules...)
}
