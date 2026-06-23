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
package widget

import (
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
)

// Separator is intended to be something like a horizontal rule, where the
// separator character can be specified.
type Separator struct {
	separator string
}

// NewSeparator constructs a new separator with a given separator character.
func NewSeparator(separator string) termio.Widget {
	return &Separator{separator}
}

// GetHeight of this widget, where MaxUint indicates widget expands to take as
// much as it can.
func (p *Separator) GetHeight() uint {
	return 1
}

// Render this widget on the given canvas.
func (p *Separator) Render(canvas termio.Canvas) {
	w, _ := canvas.GetDimensions()
	//
	var builder strings.Builder
	//
	for i := uint(0); i < w; i++ {
		builder.WriteString(p.separator)
	}
	//
	canvas.Write(0, 0, termio.NewText(builder.String()))
}
