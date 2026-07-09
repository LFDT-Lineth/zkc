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
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FromTrace converts a column-major trace.Trace into a row-major rtrace.Array.
// Each source column becomes one register, and each register is backed by one
// limb using the source column data bitwidth when available.
func FromTrace[T any](tr ctrace.Trace[T]) *Array[T] {
	modules := make([]ArrayModule[T], tr.Width())
	//
	for mid := range tr.Width() {
		modules[mid] = fromTraceModule(tr.Module(mid))
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
		builder = array.NewStaticBuilder[T]()
		modules = make([]lt.Module[T], tr.Width())
	)
	//
	for mid := range tr.Width() {
		modules[mid] = toTraceModule(tr.Module(mid), builder)
	}
	//
	return modules
}

func toTraceModule[T word.Word[T]](module Module[T], builder array.Builder[T]) lt.Module[T] {
	var (
		names   = limbColumnNames(module)
		columns = make([]lt.Column[T], module.Width())
	)
	//
	for cid := range module.Width() {
		columns[cid] = toTraceColumn(module, cid, names[cid], builder)
	}
	//
	return lt.NewModule(ctrace.ParseModuleName(module.Name()), columns)
}

func toTraceColumn[T word.Word[T]](module Module[T], cid uint, name string, builder array.Builder[T],
) lt.Column[T] {
	var data = builder.NewArray(module.Height(), limbBitwidth(module.LimbAt(cid)))
	//
	for rid := range module.Height() {
		data = data.Set(rid, module.Row(rid).Get(cid))
	}
	//
	return lt.NewColumn(name, data)
}

// limbColumnNames determines the column name for each limb in a module,
// following the naming convention used by register splitting (see
// register.SplitIntoLimbs).
func limbColumnNames[T any](module Module[T]) []string {
	names := make([]string, 0, module.Width())
	//
	for iter := module.Descriptor(); iter.HasNext(); {
		var (
			reg   = iter.Next()
			limbs = reg.Limbs().Collect()
		)
		//
		if len(limbs) == 1 {
			names = append(names, reg.Name())
		} else {
			for i := range limbs {
				names = append(names, fmt.Sprintf("%s'%d", reg.Name(), i))
			}
		}
	}
	//
	return names
}

// limbBitwidth returns the bitwidth of a given limb, using math.MaxUint for
// native limbs (i.e. those backed by field elements), as per
// register.WidthOrNative.
func limbBitwidth(l Limb) uint {
	if bitwidth := l.Bitwidth(); bitwidth.HasValue() {
		return bitwidth.Unwrap()
	}
	//
	return math.MaxUint
}

func fromTraceModule[T any](module ctrace.Module[T]) ArrayModule[T] {
	var (
		columns    = make([]ctrace.Column[T], module.Width())
		descriptor = make([]Register, module.Width())
		rows       = make([][]T, module.Height())
	)
	//
	for cid := range module.Width() {
		col := module.Column(cid)
		//
		columns[cid] = col
		descriptor[cid] = NewRegister(col.Name(), traceColumnLimbWidths(col))
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
	//
	return NewArrayModule(module.Name().String(), descriptor, rows...)
}

func traceColumnLimbWidths[T any](col ctrace.Column[T]) util.Option[[]uint] {
	data := col.Data()
	//
	if data == nil {
		return util.None[[]uint]()
	}
	//
	return util.Some([]uint{data.BitWidth()})
}

func traceRowIndex(row uint) int {
	if row > ^uint(0)>>1 {
		panic(fmt.Sprintf("row index %d exceeds int range", row))
	}
	//
	return int(row)
}
