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

import "cmp"

// Resolved provides linkage information about the given component being
// referenced.  Each component is referred to by its kind (function, RAM, ROM,
// etc) and its index of that kind.
type Resolved struct {
	Name  string
	Kind  Kind
	Index uint
}

// NewResolved constructs a new resolved symbol
func NewResolved(name string, kind Kind, index uint) Resolved {
	return Resolved{name, kind, index}
}

// IsMemory implementation for Symbol interface
func (p Resolved) IsMemory() bool {
	return p.Kind == READABLE_MEMORY || p.Kind == WRITEABLE_MEMORY
}

// IsFunction implementation for Symbol interface
func (p Resolved) IsFunction() bool {
	return p.Kind == FUNCTION
}

// IsUnknown implementation
func (p Resolved) IsUnknown() bool {
	return p.Kind == UNKNOWN
}

// Cmp implementation for set.Comparable interface
func (p Resolved) Cmp(o Resolved) int {
	return cmp.Compare(p.Index, o.Index)
}

func (p Resolved) String() string {
	return p.Name
}
