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
package rtrace

import (
	"fmt"
	"math"

	ctrace "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FromTrace converts a column-major trace.Trace into a row-major rtrace.Array.
// Each source column becomes one register, and each register is backed by one
// limb using the source column data bitwidth when available.
func FromTrace[T any, M ModuleBuilder[T, M]](tr ctrace.Trace[T]) *Array[T, M] {
	modules := make([]M, tr.Width())
	//
	for mid := range tr.Width() {
		modules[mid] = fromTraceModule[T, M](tr.Module(mid))
	}
	//
	return NewArray(modules)
}

// ToTrace converts a row-major rtrace.Trace into a column-major trace.Trace.
// Each limb becomes one column, named following the register splitting
// convention: a register backed by a single limb keeps its own name, whilst
// the limbs of a subdivided register are named "reg'0", "reg'1", etc.  Observe
// that key columns are not reconstructed (every module reports zero keys), and
// all columns are padded with zero.
func ToTrace[T word.Word[T]](tr Trace[T]) []lt.Module[T] {
	var (
		modules = make([]lt.Module[T], tr.Width())
	)
	//
	for mid := range tr.Width() {
		modules[mid] = tr.Module(mid).ToLtModule()
	}
	//
	return modules
}

func fromTraceModule[T any, M ModuleBuilder[T, M]](module ctrace.Module[T]) M {
	var (
		columns    = make([]ctrace.Column[T], module.Width())
		descriptor = make([]Register, module.Width())
		rows       = make([][]T, module.Height())
		nmod       M
	)
	//
	for cid := range module.Width() {
		col := module.Column(cid)
		//
		columns[cid] = col
		descriptor[cid] = Register{col.Name(), traceColumnLimbWidths(col)}
	}
	//
	for rid := range module.Height() {
		row := make([]T, module.Width())
		//
		for cid, col := range columns {
			row[cid] = col.Get(traceRowIndex(rid))
		}
		//
		rows[rid] = row
	}
	// Create new module
	return nmod.Initialise(module.Name().String(), descriptor, rows...)
}

func traceColumnLimbWidths[T any](col ctrace.Column[T]) util.Option[uint] {
	data := col.Data()
	//
	if data == nil {
		return util.None[uint]()
	} else if bw := data.BitWidth(); bw != math.MaxUint {
		return util.Some(bw)
	}
	// field element
	return util.None[uint]()
}

func traceRowIndex(row uint) int {
	if row > ^uint(0)>>1 {
		panic(fmt.Sprintf("row index %d exceeds int range", row))
	}
	//
	return int(row)
}
