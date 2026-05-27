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
package trace

import (
	"fmt"

	"github.com/consensys/go-corset/pkg/util/collection/array"
	"github.com/consensys/go-corset/pkg/util/field"
)

// QualifiedColumnName returns the fully qualified name of a given column.
func QualifiedColumnName(module ModuleName, column string) string {
	var (
		name = module.String()
	)
	//
	if name == "" {
		return column
	}

	return fmt.Sprintf("%s.%s", name, column)
}

// NumberOfColumns returns the total number of all columns in the given trace.
func NumberOfColumns[F any](tr Trace[F]) uint {
	var count = uint(0)
	//
	for i := range tr.Width() {
		ith := tr.Module(i)
		count += ith.Width()
	}
	//
	return count
}

// ModuleAdapter provides a generic mechanism for making a trace module in one
// field look like a trace module in another field.  Whether or not this is safe
// to do depends upon the fields in question, and is the caller's
// responsibility.
func ModuleAdapter[F1 field.Element[F1], F2 field.Element[F2]](module Module[F1]) Module[F2] {
	return &moduleAdapter[F1, F2]{module}
}

type moduleAdapter[F1 field.Element[F1], F2 field.Element[F2]] struct {
	module Module[F1]
}

// Module implementation for trace.Module interface.
func (p *moduleAdapter[F1, F2]) Name() ModuleName {
	return p.module.Name()
}

// Column implementation for trace.Module interface.
func (p *moduleAdapter[F1, F2]) Column(index uint) Column[F2] {
	return &columnAdapter[F1, F2]{p.module.Column(index)}
}

// ColumnOf implementation for trace.Module interface.
func (p *moduleAdapter[F1, F2]) ColumnOf(string) Column[F2] {
	// NOTE: this is marked unreachable because, as it stands, expression
	// evaluation never calls this method.
	panic("unreachable")
}

// FindLast implementation for the trace.Module interface.
func (p *moduleAdapter[F1, F2]) FindLast(...F2) uint {
	panic("unsupported operation")
}

// Keys implementation for the trace.Module interface.
func (p *moduleAdapter[F1, F2]) Keys() uint {
	panic("unsupported operation")
}

// Width implementation for trace.Module interface.
func (p *moduleAdapter[F1, F2]) Width() uint {
	return p.module.Width()
}

// Height implementation for trace.Module interface.
func (p *moduleAdapter[F1, F2]) Height() uint {
	return p.module.Height()
}

// RecColumn is a wrapper which enables the array being computed to be accessed
// during its own computation.
type columnAdapter[F1 field.Element[F1], F2 field.Element[F2]] struct {
	col Column[F1]
}

// Holds the name of this column
func (p *columnAdapter[F1, F2]) Name() string {
	return p.col.Name()
}

// Get implementation for trace.Column interface.
func (p *columnAdapter[F1, F2]) Get(row int) F2 {
	var (
		from  = p.col.Get(row)
		to    F2
		check F1
	)
	// Fast path: avoid expensive SetBytes round-trip for small values.
	u := from.Uint64()
	if from.Equals(check.SetUint64(u)) {
		return to.SetUint64(u)
	}

	return to.SetBytes(from.Bytes())
}

// Data implementation for trace.Column interface.
func (p *columnAdapter[F1, F2]) Data() array.Array[F2] {
	panic("unreachable")
}

// Padding implementation for trace.Column interface.
func (p *columnAdapter[F1, F2]) Padding() F2 {
	panic("unreachable")
}
