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
package json

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ToJsonString converts a trace into a JSON string.
func ToJsonString[F field.Element[F]](tr trace.Trace[F]) string {
	var (
		builder strings.Builder
		first   = true
	)
	//
	builder.WriteString("{")
	//
	for _, ith := range tr.Modules().Collect() {
		for j := range ith.Width() {
			var (
				name = ith.Descriptor().Name
				data = ith.Column(j)
			)
			//
			if !first {
				builder.WriteString(", ")
			}
			//
			first = false
			//
			builder.WriteString("\"")
			// Construct qualified column qual_name
			qual_name := trace.QualifiedColumnName(ith.Name(), name)
			// Apply bitwidth restrictions (if applicable)
			if bitwidth := data.BitWidth(); bitwidth < 256 {
				// For now, always assume unsigned int.
				qual_name = fmt.Sprintf("%s@u%d", qual_name, bitwidth)
			}
			// Write out column name
			builder.WriteString(qual_name)
			//
			builder.WriteString("\": [")

			for j := range data.Len() {
				if j != 0 {
					builder.WriteString(", ")
				}

				jth := data.Get(j)
				builder.WriteString(jth.String())
			}

			builder.WriteString("]")
		}
	}
	//
	builder.WriteString("}")
	// Done
	return builder.String()
}
