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
	"fmt"

	"github.com/consensys/go-corset/pkg/util/collection/set"
)

// Symbol represents a reference to some declared entity, such as a function,
// constant, memory or alias.
type Symbol[S any] interface {
	fmt.Stringer
	set.Comparable[S]
	// IsMemory determines whether this refers to a form of memory (or not)
	IsMemory() bool
	// IsFunction determines whether this refers to a function (or not)
	IsFunction() bool
}

// Kind determines the symbol kind (e.g. constant, function, input, output,
// etc).
type Kind uint8

const (
	// UNKNOWN indicates an unidentified external access which results due to some kind of linking failure.
	UNKNOWN = 0
	// READABLE_MEMORY identifies a memory which can be read (i.e. an input memory, or a static memory, etc).
	READABLE_MEMORY = 1
	// WRITEABLE_MEMORY identifies a memory which can be written (i.e. an
	// output memory, or a read/write memory, etc).
	WRITEABLE_MEMORY = 2
	// FUNCTION identifies a function symbol.
	FUNCTION = 3
	// CONSTANT identifies a constant symbol.
	CONSTANT = 4
	// TYPE_ALIAS identifies a alias symbol.
	TYPE_ALIAS = 5
	// MEMORY_EFFECT indicates a declared effect on some memory.
	MEMORY_EFFECT = 6
)
