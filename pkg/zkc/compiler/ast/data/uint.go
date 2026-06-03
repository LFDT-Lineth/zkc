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
package data

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
)

// UnsignedInt captures the fundamental data type of the language.
type UnsignedInt[S symbol.Symbol[S]] struct {
	bitwidth uint
	open     bool
}

// NewUnsignedInt constructs an unsigned int type of a given width.
func NewUnsignedInt[S symbol.Symbol[S]](bitwidth uint, open bool) *UnsignedInt[S] {
	return &UnsignedInt[S]{bitwidth, open}
}

// AsUint implementation for Type interface
func (p *UnsignedInt[S]) AsUint(Environment[S]) *UnsignedInt[S] {
	return p
}

// IsOpen determines whether or not this is an "open type" or not.
func (p *UnsignedInt[S]) IsOpen() bool {
	return p.open
}

// BitWidth returns the width of this unsigned int type (e.g. 8 for u8, etc)
func (p *UnsignedInt[S]) BitWidth() uint {
	return p.bitwidth
}

// Flattern implementation for Type interface
func (p *UnsignedInt[S]) Flattern(prefix string, env Environment[S], constructor func(name string, bitwidth uint)) {

}

// AsTuple implementation for Type interface
func (p *UnsignedInt[S]) AsTuple(Environment[S]) *Tuple[S] {
	return nil
}

// Join combines to uint types together
func (p *UnsignedInt[S]) Join(q *UnsignedInt[S]) *UnsignedInt[S] {
	if p.open && q.open {
		return &UnsignedInt[S]{max(p.bitwidth, q.bitwidth), true}
	} else if p.open {
		return q
	} else if q.open {
		return p
	} else if p.bitwidth != q.bitwidth {
		panic(fmt.Sprintf("cannot join u%d ⊔ u%d", p.bitwidth, q.bitwidth))
	}
	//
	return p
}

// AsAlias implementation for Type interface
func (p *UnsignedInt[S]) AsAlias(Environment[S]) *Alias[S] {
	return nil
}

// AsField implementation for Type interface
func (p *UnsignedInt[S]) AsField(Environment[S]) *FieldElement[S] {
	return nil
}

// AsFixedArray implementation for Type interface
func (p *UnsignedInt[S]) AsFixedArray(Environment[S]) *FixedArray[S] {
	return nil
}

func (p *UnsignedInt[S]) String(_ Environment[S]) string {
	if p.open {
		return fmt.Sprintf("u%d+", p.bitwidth)
	}
	//
	return fmt.Sprintf("u%d", p.bitwidth)
}
