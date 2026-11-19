// Copyright Consensys Incorporated
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
package vm

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/interpreter"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// PreFlightRecorder is used for recording "preflight data".  More specifically,
// the set of all messages sent or received on any bus during execution of the
// entire program.  For example, when a global function is invoked its arguments
// correspond to a message which is both sent and received on the bus of global
// function (recall that global functions have no returns).  Other forms of
// interactions include those required for managing the various forms of
// supported read/write memory.
type PreFlightRecorder[W Word[W]] struct {
	// Program being executed (in fast mode) to record preflight data.
	program Program[W]
	// Handle for bytecode interpreter
	bci *Interpreter[W]
	// Accumulated preflight data
	data []trace.ModuleBuilder[W]
	// Read-write memories whose timestamps prefix each row recorded for a given
	// global function (i.e. those memories in its effects).
	stamps [][]interpreter.ReadWriteMemory[W]
	// Scratch buffer for constructing rows (sized for the widest module).
	row []W
}

// NewPreFlightRecorder constructs a new preflight recorder from the given
// program.  This constructs an appropriate bytecode interpreter for executing
// the underlying program with breakpoints for each global function, thereby
// allowing their bus interactions to be recorded.
func NewPreFlightRecorder[W Word[W]](program Program[W]) PreFlightRecorder[W] {
	var (
		modules = program.Modules()
		data    = make([]trace.ModuleBuilder[W], uint(len(modules)))
		stamps  = make([][]interpreter.ReadWriteMemory[W], uint(len(modules)))
		width   uint
	)
	// Configure breakpoints for all global functions (since only these have bus
	// interactions).
	for mid, m := range modules {
		if f, ok := m.(*descriptor.Function[W]); ok && f.Kind().IsGlobal() {
			var (
				fid = util.Cast[uint16](mid)
			)
			// Mark global function
			program = program.BreakPoint(fid, 0)
			//
			data[fid] = *trace.InitModuleBuilder[W](toModuleDescriptor(modules, f))
			width = max(width, data[fid].Width())
		}
	}
	// Construct interpreter which triggers the breakpoint function on every
	// invocation of any global function, and records the relevant interaction.
	var bci = NewBytecodeInterpreter(program)
	// Resolve the memories whose timestamps are recorded for each global
	// function.
	for mid, m := range modules {
		if f, ok := m.(*descriptor.Function[W]); ok && f.Kind().IsGlobal() {
			stamps[mid] = array.Map(f.Effects(), func(_ uint, e descriptor.ModuleId) interpreter.ReadWriteMemory[W] {
				return bci.Memory(e).(interpreter.ReadWriteMemory[W])
			})
		}
	}
	//
	var pfr = PreFlightRecorder[W]{program, bci, data, stamps, make([]W, width)}
	// Register breakpoint handler
	bci.WithBreakpointHandler(func(_ uint32, entry bool) bool {
		if entry {
			// Record interaction
			pfr.recordFunctionCall()
		}
		// Don't interrupt
		return false
	})
	//
	return pfr
}

// Interpreter gives access the bytecode interpreter used by this preflight
// recorder.
func (p *PreFlightRecorder[W]) Interpreter() *Interpreter[W] {
	return p.bci
}

// CountInteractions simply counts the height of each module in the preflight
// shard being constructed.  Each row, therefore, is counted as a "bus
// interaction".
func (p *PreFlightRecorder[W]) CountInteractions() (n uint) {
	for _, m := range p.data {
		n += m.Height()
	}
	//
	return n
}

// Convert recorded preflight data for the target field.
func (p *PreFlightRecorder[W]) Convert[F Element[F]]() util.Option[trace.Shard[F]] {
	var (
		field   = p.program.Field()
		modules = make([]trace.Module[F], len(p.data))
		tmp     F
	)
	// Sanity check
	util.Assert(field.Modulus().Cmp(tmp.Modulus()) == 0, "mismatched field element and config")
	//
	for i, m := range p.data {
		// Convert ith module
		modules[i] = convertModule[W, F](field, m.Build())
	}
	// Construct final shard
	return util.Some(trace.NewShard(modules))
}

// Record bus interaction for currently executing function.  Each row consists
// of the timestamps for the function's effects, followed by its arguments.
// This matches the signature of the function in the tracing program, where
// the stamps are threaded as additional inputs (see ThreadTimestamps).
func (p *PreFlightRecorder[W]) recordFunctionCall() {
	var (
		// Extract state from the interpreter
		fid, _, frame = interpreter.ExtractExecutingState(p.bci)
		stamps        = p.stamps[fid]
		row           = p.row[:p.data[fid].Width()]
		n             = len(stamps)
	)
	// The stamp on entry to a function is one past the memory's current clock
	// (since timestamp zero is reserved for the initial state of an untouched
	// cell).
	for i, mem := range stamps {
		row[i] = word.Const64[W](mem.Clock() + 1)
	}
	// Arguments follow the stamps
	copy(row[n:], frame)
	// Append data row
	p.data[fid].Append(row...)
}

// Construct the module descriptor for a given global function, consisting of
// a stamp column for each memory in its effects followed by its signature.
// This matches the layout of the function's inputs after timestamp threading.
func toModuleDescriptor[W word.Word[W]](modules []descriptor.Module[W], f *descriptor.Function[W],
) trace.ModuleDescriptor {
	var (
		signatureWidth = f.NumInputs() + f.NumOutputs()
		// Construct stamp columns for each memory effect
		stamps = array.Map(f.Effects(), func(_ uint, e descriptor.ModuleId) trace.ColumnDescriptor {
			var mem = modules[e].(*descriptor.Memory[W])
			//
			return trace.NewColumnDescriptor(descriptor.StampName(mem.Name()), mem.StampWidth())
		})
		// Convert registers which make up the signature of this function into
		// column descriptors.
		columns = array.Map(f.Registers()[:signatureWidth], func(_ uint, r Register[W]) trace.ColumnDescriptor {
			return trace.NewColumnDescriptor(r.Name(), r.Bitwidth())
		})
	)
	//
	return trace.NewModuleDescriptor(f.Name(), append(stamps, columns...))
}

func convertModule[W Word[W], F Element[F]](field field.Config, mod trace.Module[W]) trace.Module[F] {
	var (
		columns []trace.ColumnDescriptor
		data    []array.Array[F]
	)
	//
	for i, col := range mod.Descriptor().Columns {
		var (
			nCols = splitColumnDescriptor(field, col)
			nData = splitColumnData[W, F](nCols, mod.Column(uint(i)))
		)
		// Reverse in place so as to ensure that most-significant limb comes first.
		array.ReverseInPlace(nCols)
		array.ReverseInPlace(nData)
		//
		columns = append(columns, nCols...)
		data = append(data, nData...)
	}
	//
	return trace.NewModule(trace.NewModuleDescriptor(mod.Name(), columns), data...)
}

// Split given column descriptor into limb descriptors organised with the least
// significant limb coming first.
func splitColumnDescriptor(field field.Config, desc trace.ColumnDescriptor) []trace.ColumnDescriptor {
	var limbWidths []uint
	// Handle native field case first
	if desc.Bitwidth.IsEmpty() {
		return []trace.ColumnDescriptor{desc}
	}
	// Apply splitting algorithm
	limbWidths = descriptor.LimbWidths(field.RegisterWidth, desc.Bitwidth.Unwrap())
	// Reconstruct descriptors
	if len(limbWidths) <= 1 {
		// NOTE: to be consistent with register splitting elsewhere, naming of
		// limbs only uses the extended notation (e.g. "X'1") when there is more
		// than one limb.  This includes zero-width columns, which have no limbs.
		return []trace.ColumnDescriptor{desc}
	}
	//
	return array.Map(limbWidths, func(i uint, bitwidth uint) trace.ColumnDescriptor {
		var name = fmt.Sprintf("%s'%d", desc.Name, i)
		//
		return trace.NewColumnDescriptor(name, util.Some(bitwidth))
	})
}

// Split given data according to the given column descriptors (which are
// organised in with the least significant limb coming first).  A native column
// (i.e. one without a bitwidth) is never split, hence is converted directly.
func splitColumnData[W Word[W], F Element[F]](cols []trace.ColumnDescriptor, data array.Array[W]) []array.Array[F] {
	var native = len(cols) == 1 && cols[0].Bitwidth.IsEmpty()
	// Determine whether native or non-native case.
	if native {
		return splitNativeColumnData[W, F](data)
	}
	//
	return splitFixedColumnData[W, F](cols, data)
}

// Convert native column data directly into a single column of field elements.
func splitNativeColumnData[W Word[W], F Element[F]](data array.Array[W]) []array.Array[F] {
	var (
		limb = array.Alloc[F](math.MaxUint)
	)
	// Convert each row directly
	for row := range data.Len() {
		limb.Append(wordToElement[W, F](data.Get(row)))
	}
	//
	return []array.Array[F]{limb.Build()}
}

// Split fixed-width column data into limbs, with the least significant limb
// first.
func splitFixedColumnData[W Word[W], F Element[F]](cols []trace.ColumnDescriptor, arr array.Array[W]) []array.Array[F] {
	var (
		limbs  = make([]array.MutArray[F], len(cols))
		result = make([]array.Array[F], len(cols))
	)
	// Allocate limb arrays
	for i, col := range cols {
		limbs[i] = array.Alloc[F](col.Bitwidth.Unwrap())
	}
	// Convert over all data
	for row := range arr.Len() {
		var value = arr.Get(row)
		//
		for i, limb := range limbs {
			var width = cols[i].Bitwidth.Unwrap()
			limb.Append(wordToElement[W, F](value.Slice(width)))
			value = value.Shr64(uint64(width))
		}
	}
	// Finalise limb arrays
	for i, limb := range limbs {
		result[i] = limb.Build()
	}
	//
	return result
}

// Convert a word into a field element, assuming its value is below the field
// modulus.  Values which fit within 64 bits avoid the (allocating) byte
// conversion.
func wordToElement[W Word[W], F Element[F]](value W) F {
	var element F
	//
	if value.FitsWithin(64) {
		return element.SetUint64(value.Uint64())
	}
	//
	return element.SetBytes(value.Bytes())
}
