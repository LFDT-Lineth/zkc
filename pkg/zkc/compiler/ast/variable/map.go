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
package variable

import "github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"

// Map defines an abstract notion of mapping a variable identifier to a
// variable description.
type Map[S symbol.Symbol[S]] interface {
	// Get the descriptor for a given variable.
	Variable(Id) Descriptor[S]
}

// ArrayMap constructs a variable map from an array of variables.
func ArrayMap[S symbol.Symbol[S]](vars ...Descriptor[S]) Map[S] {
	return &arrayMap[S]{vars}
}

// arrayMap constructs a variable map from an array of variable declarations.
type arrayMap[S symbol.Symbol[S]] struct {
	vars []Descriptor[S]
}

// Variable implementation for Map interface
func (p arrayMap[S]) Variable(id Id) Descriptor[S] {
	return p.vars[id]
}
