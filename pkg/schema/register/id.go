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
package register

import "github.com/LFDT-Lineth/zkc/pkg/trace"

// Id captures the notion of a register index.  That is, for each
// module, every register is allocated a given index starting from 0.  The
// purpose of the wrapper is to avoid confusion between uint values and things
// which are expected to identify Columns.
type Id = trace.ColumnId

// AccessId is a wrapper around a column Id which adds a "relative shift" and
// "bitwidth window".  That is, it identifies a column on a relative row from
// the given row, and allows only a portion of the original variable to be
// accessed.
type AccessId = trace.ColumnAccessId

// NewId constructs a new register ID from a given raw index.
func NewId(index uint) Id {
	return trace.NewColumnId(index)
}

// UnusedId constructs something akin to a null reference.  This is
// used in some situations where we may (or may not) want to refer to a specific
// register.
func UnusedId() Id {
	return trace.NewUnusedColumnId()
}
