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
package assignment

import (
	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/narray"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ReadRegistersRef reads the values for a given set of registers from a trace.
func ReadRegistersRef[F field.Element[F]](trace rtrace.Trace[F], regs ...register.Ref) []narray.Array[F] {
	var (
		targets = make([]narray.Array[F], len(regs))
	)
	// Read registers
	for i, ref := range regs {
		mid, rid := ref.Module(), ref.Register().Unwrap()
		targets[i] = trace.Module(mid).Column(rid)
	}
	//
	return targets
}
