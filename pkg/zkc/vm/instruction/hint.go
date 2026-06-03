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
package instruction

import (
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	finsn "github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/field"
)

// FieldHint is a non-deterministic register assignment with no polynomial
// constraint.  It marks the target registers as defined for the constancy
// analysis without generating any equality.
type FieldHint = finsn.Hint

// NewFieldHint constructs a new hint instruction.
func NewFieldHint(targets, sources []register.Id) *FieldHint {
	return &FieldHint{Targets: targets, Sources: sources}
}
