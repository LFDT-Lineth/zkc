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
package assignment

import (
	"encoding/gob"
	"fmt"
	"slices"

	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	tr "github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// NativeComputation currently describes a native computation which accepts a set of
// input columns, and assigns a set of output columns.
type NativeComputation[F field.Element[F]] struct {
	// Name of the function being invoked.
	Function string
	// Target columns declared by this sorted permutation (in the order
	// of declaration).
	Targets []register.Refs
	// Source columns which define the new (sorted) columns.
	Sources []register.Refs
}

// NewNativeComputation defines a set of target columns which are assigned from a
// given set of source columns using a function to multiplex input to output.
func NewNativeComputation[F field.Element[F]](fn string, targets []register.Refs,
	sources []register.Refs) *NativeComputation[F] {
	//
	return &NativeComputation[F]{fn, targets, sources}
}

// ============================================================================
// Assignment Interface
// ============================================================================

// Bounds determines the well-definedness bounds for this assignment for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
func (p *NativeComputation[F]) Bounds(_ sc.ModuleId) util.Bounds {
	return util.EMPTY_BOUND
}

// Compute computes the values of columns defined by this assignment. This
// requires copying the data in the source columns, and sorting that data
// according to the permutation criteria.
func (p *NativeComputation[F]) Compute(trace tr.Trace[F], schema sc.AnySchema[F],
) ([]array.MutArray[F], error) {
	// Identify Computation
	fn := findNative[F](p.Function)
	// Go!
	return computeNative(p.Sources, fn, trace), nil
}

// Consistent performs some simple checks that the given schema is consistent.
// This provides a double check of certain key properties, such as that
// registers used for assignments are large enough, etc.
func (p *NativeComputation[F]) Consistent(_ sc.AnySchema[F]) []error {
	// NOTE: this is where we could (in principle) check the type of the
	// function being defined to ensure it is, for example, typed correctly.
	return nil
}

// RegistersExpanded identifies registers expanded by this assignment.
func (p *NativeComputation[F]) RegistersExpanded() []register.Ref {
	return nil
}

// RegistersRead returns the set of columns that this assignment depends upon.
// That can include both input columns, as well as other computed columns.
func (p *NativeComputation[F]) RegistersRead() []register.Ref {
	return array.FlatMap(p.Sources, register.AsRefArray)
}

// RegistersWritten identifies registers assigned by this assignment.
func (p *NativeComputation[F]) RegistersWritten() []register.Ref {
	return array.FlatMap(p.Targets, register.AsRefArray)
}

// Substitute any matchined labelled constants within this assignment
func (p *NativeComputation[F]) Substitute(map[string]F) {
	// Nothing to do here.
}

// ============================================================================
// Lispify Interface
// ============================================================================

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
func (p *NativeComputation[F]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	var (
		targets = sexp.EmptyList()
		sources = sexp.EmptyList()
	)

	for _, refs := range p.Targets {
		var (
			regs   = sexp.EmptyList()
			module = schema.Module(refs.Module())
		)
		//
		for _, ref := range refs.Registers() {
			ith := module.Register(ref)
			name := sexp.NewSymbol(ith.QualifiedName(module))
			datatype := sexp.NewSymbol(fmt.Sprintf("u%d", ith.Width()))
			def := sexp.NewList([]sexp.SExp{name, datatype})
			regs.Append(def)
		}
		//
		targets.Append(regs)
	}
	//
	for _, refs := range p.Sources {
		var (
			regs   = sexp.EmptyList()
			module = schema.Module(refs.Module())
		)
		//
		for _, ref := range refs.Registers() {
			ith := module.Register(ref)
			name := ith.QualifiedName(module)
			regs.Append(sexp.NewSymbol(name))
		}
		//
		sources.Append(regs)
	}

	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("compute"),
		targets,
		sexp.NewSymbol(p.Function),
		sources,
	})
}

// ============================================================================
// Native Generic Computation
// ============================================================================

// NativeComputationFn defines the type of a native function for computing a given
// set of output columns as a function of a given set of input columns.
type NativeComputationFn[F any] func([]array.Vector[F], array.Builder[F]) []array.MutVector[F]

func computeNative[F field.Element[F]](sources []register.Refs, fn NativeComputationFn[F], trace tr.Trace[F],
) []array.MutArray[F] {
	// Read inputs
	inputs := ReadRegisterRefs(trace, sources...)
	// Apply native function
	targets := fn(inputs, trace.Builder())
	// Flattern targets
	return array.FlatMap(targets, func(arrs array.MutVector[F]) []array.MutArray[F] {
		return arrs.Unwrap()
	})
}

// ============================================================================
// Native Function Definitions
// ============================================================================

func findNative[F field.Element[F]](name string) NativeComputationFn[F] {
	switch name {
	case "id":
		return idNativeFunction
	case "interleave":
		return interleaveNativeFunction
	case "filter":
		return filterNativeFunction
	case "map-if":
		return mapIfNativeFunction
	case "fwd-changes-within":
		return fwdChangesWithinNativeFunction
	case "fwd-unchanged-within":
		return fwdUnchangedWithinNativeFunction
	case "bwd-changes-within":
		return bwdChangesWithinNativeFunction
	case "fwd-fill-within":
		return fwdFillWithinNativeFunction
	case "bwd-fill-within":
		return bwdFillWithinNativeFunction
	default:
		panic(fmt.Sprintf("unknown native function: %s", name))
	}
}

// id assigns the target column with the corresponding value of the source
// column
func idNativeFunction[F field.Element[F]](sources []array.Vector[F], _ array.Builder[F],
) []array.MutVector[F] {
	if len(sources) != 1 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Clone source vector (that's it)
		target = sources[0].Clone()
	)
	// Done
	return []array.MutVector[F]{target}
}

// interleaving constructs a single interleaved column from a give set of source
// columns.  The assumption is that the height of all columns is the same.
func interleaveNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	var (
		bitwidths  = array.BitwidthOfVectors(sources...)
		values     = make([]F, array.MaxWidthOfVectors(sources...))
		height     = sources[0].Len()
		multiplier = uint(len(sources))
	)
	// Sanity check column heights
	for _, src := range sources {
		if src.Len() != height {
			panic(fmt.Sprintf("inconsistent column height for interleaving (%d v %d)", src.Len(), height))
		}
	}
	// Construct interleaved column
	target := array.NewMutVector(height*multiplier, bitwidths, builder)
	//
	for i := range multiplier {
		source := sources[i]
		//
		for j := range height {
			row := (j * multiplier) + i
			// Read source value
			source.Read(j, values)
			// Write target value
			target.Write(row, values)
		}
	}
	// Done
	return []array.MutVector[F]{target}
}

// filter assigns the target column with the corresponding value of the source
// column *when* a given selector column is non-zero.  Otherwise, the target
// column remains zero at the given position.
func filterNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	if len(sources) != 2 {
		panic("incorrect number of arguments")
	}

	var (
		// Extract input column info
		source   = sources[0]
		selector = sources[1]
		// Create target column
		target = source.EmptyClone(source.Len(), builder)
		// Construct temporary buffer
		values = make([]F, array.MaxWidthOfVectors(sources...))
	)
	//
	for i := uint(0); i < target.Len(); i++ {
		// Check whether selctor non-zero
		if !isZero(i, selector) {
			source.Read(i, values)
			target.Write(i, values)
		}
	}
	// Done
	return []array.MutVector[F]{target}
}

// apply a key-value map conditionally.
func mapIfNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	n := len(sources) - 3
	if n%2 != 0 {
		panic(fmt.Sprintf("map-if expects 3 + 2*n columns (given %d)", len(sources)))
	}
	//
	n = n / 2
	// Setup what we need
	sourceSelector := sources[1+n]
	sourceKeys := make([]array.Vector[F], n)
	sourceValue := sources[2+n+n]
	sourceMap := hash.NewMap[hash.Array[F], []F](sourceValue.Len())
	targetSelector := sources[0]
	targetKeys := make([]array.Vector[F], n)
	targetValue := sourceValue.EmptyClone(targetSelector.Len(), builder)
	// Construct temporary buffer
	tmpBuffer := make([]F, array.WidthOfVectors(sourceValue))
	// Initialise source / target keys
	for i := 0; i < n; i++ {
		targetKeys[i] = sources[1+i]
		sourceKeys[i] = sources[2+n+i]
	}
	// Build source map
	for i := uint(0); i < sourceValue.Len(); i++ {
		// Check whether selector non-zero
		if !isZero(i, sourceSelector) {
			// Extract ith key
			ithKey := extractIthKey(i, sourceKeys)
			// Read ith value
			sourceValue.Read(i, tmpBuffer)
			//
			if val, ok := sourceMap.Get(ithKey); ok && array.Compare(val, tmpBuffer) != 0 {
				// Conflicting item already in map, so fail with useful error.
				ithRow := extractIthColumns(i, sourceKeys, nil)
				lhs := fmt.Sprintf("%v=>%v", ithRow, tmpBuffer)
				rhs := fmt.Sprintf("%v=>%v", ithRow, val)
				panic(fmt.Sprintf("conflicting values in source map (row %d): %s vs %s", i, lhs, rhs))
			} else if !ok {
				// Item not previously in map
				sourceMap.Insert(ithKey, slices.Clone(tmpBuffer))
			}
		}
	}
	// Construct target value column
	for i := uint(0); i < targetValue.Len(); i++ {
		if !isZero(i, targetSelector) {
			ithKey := extractIthKey(i, targetKeys)
			//nolint:revive
			if val, ok := sourceMap.Get(ithKey); !ok {
				// Couldn't find key in source map, so fail with useful error.
				ith_row := extractIthColumns(i, targetKeys, nil)
				panic(fmt.Sprintf("target key (%v) missing from source map (row %d)", ith_row, i))
			} else {
				// Assign target value
				targetValue.Write(i, val)
			}
		}
	}
	// Done
	return []array.MutVector[F]{targetValue}
}

func extractIthKey[F field.Element[F]](index uint, cols []array.Vector[F]) hash.Array[F] {
	var (
		count uint
		// Each column has 1 x 64bit hash
		buffer = make([]F, array.WidthOfVectors(cols...))
	)
	// Evaluate each expression in turn
	for _, col := range cols {
		for i := range col.Width() {
			buffer[count] = col.Limb(i).Get(index)
			count++
		}
	}
	// Done
	return hash.NewArray(buffer)
}

// determines changes of a given set of columns within a given region.
func fwdChangesWithinNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	if len(sources) < 2 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Useful constant
		one = field.One[F]()
		// Extract input column info
		selector = sources[0]
		// Construct (binary) output column
		data    = builder.NewArray(selector.Len(), 1)
		row     []F
		started = false
	)
	// Trim off selector
	sources = sources[1:]
	// Set current value
	current := make([]F, array.WidthOfVectors(sources...))
	//
	for i := uint(0); i < selector.Len(); i++ {
		// Check whether within region or not.
		if !isZero(i, selector) {
			//
			row := extractIthColumns(i, sources, row)
			// Trigger required?
			if !started || array.Compare(current, row) != 0 {
				started = true
				current = slices.Clone(row)
				//
				data = data.Set(i, one)
			}
		}
	}
	// Done
	return []array.MutVector[F]{array.MutVectorOf(data)}
}

func fwdUnchangedWithinNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	if len(sources) < 2 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Useful constant
		one  = field.One[F]()
		zero = field.Zero[F]()
		// Extract input column info
		selector = sources[0]
		// Construct (binary) output column
		data    = builder.NewArray(selector.Len(), 1)
		row     []F
		started = false
	)
	// Trim off selector
	sources = sources[1:]
	// Set current value
	current := make([]F, array.WidthOfVectors(sources...))
	//
	for i := uint(0); i < selector.Len(); i++ {
		// Check whether within region or not.
		if !isZero(i, selector) {
			//
			row = extractIthColumns(i, sources, row)
			// Trigger required?
			if !started || array.Compare(current, row) != 0 {
				started = true
				current = slices.Clone(row)
				//
				data = data.Set(i, zero)
			} else {
				data = data.Set(i, one)
			}
		}
	}
	// Done
	return []array.MutVector[F]{array.MutVectorOf(data)}
}

// determines changes of a given set of columns within a given region.
func bwdChangesWithinNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	if len(sources) < 2 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Useful constant
		one = field.One[F]()
		// Extract input column info
		selector = sources[0]
		// Construct (binary) output column
		data    = builder.NewArray(selector.Len(), 1)
		row     []F
		started = false
	)
	// Trim off selector
	sources = sources[1:]
	// Set current value
	current := make([]F, array.WidthOfVectors(sources...))
	//
	for i := selector.Len(); i > 0; i-- {
		// Check whether within region or not.
		if !isZero(i-1, selector) {
			//
			row = extractIthColumns(i-1, sources, row)
			// Trigger required?
			if !started || array.Compare(current, row) != 0 {
				started = true
				current = slices.Clone(row)
				//
				data = data.Set(i-1, one)
			}
		}
	}
	// Done
	return []array.MutVector[F]{array.MutVectorOf(data)}
}

func fwdFillWithinNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	if len(sources) != 3 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Extract input column info
		selector = sources[0]
		first    = sources[1]
		source   = sources[2]
		// Construct output column
		data = source.EmptyClone(source.Len(), builder)
		// Initialise current value
		current = make([]F, source.Width())
	)
	//
	for i := uint(0); i < selector.Len(); i++ {
		// Check whether within region or not.
		if !isZero(i, selector) {
			//
			if !isZero(i, first) {
				source.Read(i, current)
			}
			//
			data.Write(i, current)
		}
	}
	// Done
	return []array.MutVector[F]{data}
}

func bwdFillWithinNativeFunction[F field.Element[F]](sources []array.Vector[F], builder array.Builder[F],
) []array.MutVector[F] {
	//
	if len(sources) != 3 {
		panic("incorrect number of arguments")
	}
	//
	var (
		// Extract input column info
		source    = sources[0]
		first     = sources[1]
		sourceCol = sources[2]
		// Construct output column
		target = sourceCol.EmptyClone(source.Len(), builder)
		// Initialise current value
		current = make([]F, sourceCol.Width())
	)
	//
	for i := source.Len(); i > 0; i-- {
		// Check whether within region or not.
		if !isZero(i-1, source) {
			//
			if !isZero(i-1, first) {
				sourceCol.Read(i-1, current)
			}
			//
			target.Write(i-1, current)
		}
	}
	// Done
	return []array.MutVector[F]{target}
}

func extractIthColumns[F any](index uint, cols []array.Vector[F], buffer []F) []F {
	var count uint
	// Sanity check buffer is big enough, or not.
	if buffer == nil {
		buffer = make([]F, array.WidthOfVectors(cols...))
	}
	//
	for _, ith := range cols {
		for j := range ith.Width() {
			buffer[count] = ith.Limb(j).Get(index)
			count++
		}
	}
	//
	return buffer
}

// ============================================================================
// Helpers
// ============================================================================

// Check whether all limbs within a given vector are zero, or not.
func isZero[F field.Element[F]](index uint, vec array.Vector[F]) bool {
	return vec.All(index, func(f F) bool { return f.IsZero() })
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

func init() {
	gob.Register(sc.Assignment[word.BigEndian](&NativeComputation[word.BigEndian]{}))
}
