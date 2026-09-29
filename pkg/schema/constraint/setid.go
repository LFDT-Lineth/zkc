// Copyright Consensys Software Inc.
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
package constraint

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// SetId provides a generic mechanism for referring to a particular set of data
// required for constraint checking.
type SetId struct {
	mid module.Id
	sel util.Option[register.Id]
	row []register.Id
}

// NewSetId constructs a new set id.
func NewSetId(mid module.Id, selector util.Option[register.Id], row []register.Id) SetId {
	return SetId{mid, selector, row}
}

// Cmp implementation for set.Comparable inteface.
func (p SetId) Cmp(o SetId) int {
	if c := cmp.Compare(p.mid, o.mid); c != 0 {
		return c
	} else if c := array.Compare(p.row, o.row); c != 0 {
		return c
	}
	//
	return compareSelectors(p.sel, o.sel)
}

// Module returns the module to which this identifier refers.
func (p SetId) Module() module.Id {
	return p.mid
}

// HasSelector determines whether or not this identifier has a selector line.
func (p SetId) HasSelector() bool {
	return p.sel.HasValue()
}

// Selector returns the selector line for this identifier, or panics if it has
// none.
func (p SetId) Selector() register.Id {
	return p.sel.Unwrap()
}

// Width returns the number of data lines for this identifier.
func (p SetId) Width() uint {
	return uint(len(p.row))
}

// Ith returns the ith data line for this identifier.
func (p SetId) Ith(index uint) register.Id {
	return p.row[index]
}

// String returns a (unique) string representation for this set.
func (p SetId) String() string {
	var builder strings.Builder
	//
	if p.HasSelector() {
		fmt.Fprintf(&builder, "%x:%x", p.mid, p.sel.Unwrap())
	} else {
		fmt.Fprintf(&builder, "%x", p.mid)
	}
	//
	for _, r := range p.row {
		fmt.Fprintf(&builder, ";%x", r.Unwrap())
	}
	//
	return builder.String()
}

func compareSelectors(l, r util.Option[register.Id]) int {
	switch {
	case l.IsEmpty() && r.IsEmpty():
		return 0
	case l.IsEmpty():
		return -1
	case r.IsEmpty():
		return 1
	default:
		return l.Unwrap().Cmp(r.Unwrap())
	}
}
