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
package generate

import (
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/util"
)

// Intersect two source modules, producing one source module which contains only
// the items which are present in both.  That is, the returned module captures
// the common functionality across both modules.
func intersectModules(left corset.SourceModule, right corset.SourceModule) *corset.SourceModule {
	// First, some sanity checks
	if left.Name != right.Name || left.Virtual != right.Virtual {
		return nil
	}
	//
	return &corset.SourceModule{
		Name:       left.Name,
		Synthetic:  false,
		Virtual:    left.Virtual,
		Selector:   util.None[string](),
		Submodules: intersectSubmodules(left.Submodules, right.Submodules),
		Columns:    intersectColumns(left.Columns, right.Columns),
		Constants:  intersectConstants(left.Constants, right.Constants),
	}
}

func intersectSubmodules(left []corset.SourceModule, right []corset.SourceModule) []corset.SourceModule {
	var (
		modules []corset.SourceModule
		// Construct suitable name comparator
		cmp = func(l corset.SourceModule, r corset.SourceModule) int { return strings.Compare(l.Name, r.Name) }
		//
		l, r = 0, 0
	)
	//
	slices.SortFunc(left, cmp)
	slices.SortFunc(right, cmp)
	//
	for l < len(left) && r < len(right) {
		leftModule := left[l]
		rightModule := right[r]
		c := strings.Compare(leftModule.Name, rightModule.Name)

		switch {
		case c < 0:
			l++
		case c > 0:
			r++
		case c == 0:
			if mod := intersectModules(leftModule, rightModule); mod != nil {
				modules = append(modules, *mod)
			}

			l++
			r++
		}
	}
	//
	return modules
}

func intersectColumns(left []corset.SourceColumn, right []corset.SourceColumn) []corset.SourceColumn {
	var (
		columns []corset.SourceColumn
		// Construct suitable name comparator
		cmp = func(l corset.SourceColumn, r corset.SourceColumn) int { return strings.Compare(l.Name, r.Name) }
		//
		l, r = 0, 0
	)
	//
	slices.SortFunc(left, cmp)
	slices.SortFunc(right, cmp)
	//
	for l < len(left) && r < len(right) {
		leftColumn := left[l]
		rightColumn := right[r]
		c := strings.Compare(leftColumn.Name, rightColumn.Name)

		switch {
		case c < 0:
			l++
		case c > 0:
			r++
		case c == 0:
			if col := intersectColumn(leftColumn, rightColumn); col != nil {
				columns = append(columns, *col)
			}

			l++
			r++
		}
	}
	//
	return columns
}

func intersectColumn(left corset.SourceColumn, right corset.SourceColumn) *corset.SourceColumn {
	//
	if left.Name != right.Name {
		panic("unreachable")
	} else if left.Multiplier != right.Multiplier {
		return nil
	} else if left.Bitwidth != right.Bitwidth {
		return nil
	} else if left.Computed != right.Computed {
		return nil
	}
	//
	return &left
}

func intersectConstants(left []corset.SourceConstant, right []corset.SourceConstant) []corset.SourceConstant {
	var (
		constants []corset.SourceConstant
		// Construct suitable name comparator
		cmp = func(l corset.SourceConstant, r corset.SourceConstant) int { return strings.Compare(l.Name, r.Name) }
		//
		l, r = 0, 0
	)
	//
	slices.SortFunc(left, cmp)
	slices.SortFunc(right, cmp)
	//
	for l < len(left) && r < len(right) {
		leftColumn := left[l]
		rightColumn := right[r]
		c := strings.Compare(leftColumn.Name, rightColumn.Name)

		switch {
		case c < 0:
			l++
		case c > 0:
			r++
		case c == 0:
			if col := intersectConstant(leftColumn, rightColumn); col != nil {
				constants = append(constants, *col)
			}

			l++
			r++
		}
	}

	return constants
}

func intersectConstant(left corset.SourceConstant, right corset.SourceConstant) *corset.SourceConstant {
	if left.Name != right.Name {
		panic("unreachable")
	} else if left.Bitwidth != right.Bitwidth {
		return nil
	} else if left.Value.Cmp(&right.Value) != 0 {
		return nil
	}
	//
	return &left
}
