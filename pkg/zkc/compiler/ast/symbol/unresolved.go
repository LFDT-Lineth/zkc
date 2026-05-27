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
package symbol

import (
	"cmp"
	"math"
	"strings"
)

// ANY_INPUTS is used as a wildcard for the number of inputs expected.
const ANY_INPUTS uint = math.MaxUint

// Unresolved symbols represented expected records in the symbol table.  For
// functions, this includes their "arity" --- that is, the number of expected
// inputs and outputs.
type Unresolved struct {
	Name   string
	Kind   Kind
	Inputs uint
}

// NewUnresolved constructs a new unresolved symbol with a given arity.
func NewUnresolved(name string, kind Kind, inputs uint) Unresolved {
	return Unresolved{name, kind, inputs}
}

// HasAnyArity checks whether or not this symbol will match any arity or not.
func (p Unresolved) HasAnyArity() bool {
	return p.Inputs == ANY_INPUTS
}

// IsMemory implementation for Symbol interface
func (p Unresolved) IsMemory() bool {
	return p.Kind == READABLE_MEMORY || p.Kind == WRITEABLE_MEMORY
}

// IsFunction implementation for Symbol interface
func (p Unresolved) IsFunction() bool {
	return p.Kind == FUNCTION
}

// Cmp implementation for set.Comparable interface
func (p Unresolved) Cmp(o Unresolved) int {
	if c := strings.Compare(p.Name, o.Name); c != 0 {
		return c
	} else if c := cmp.Compare(p.Inputs, o.Inputs); c != 0 {
		return c
	}
	//
	return cmp.Compare(p.Kind, o.Kind)
}

func (p Unresolved) String() string {
	return p.Name
}
