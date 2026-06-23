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
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/agnostic"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

const (
	// minParallelHeight is the minimum number of rows before enabling
	// row-parallel computation within a single assignment.
	minParallelHeight = 4096
	// minRowsPerWorker ensures each goroutine processes a meaningful amount
	// of work to avoid goroutine scheduling overhead dominating.
	minRowsPerWorker = 1024
)

// INNER_WORKERS controls the number of goroutines for row-parallel computation
// within a single computed register assignment. A value of 0 (default) means
// auto-detect from runtime.GOMAXPROCS(0). This can be set from the CLI to
// coordinate with outer-level parallelism in ParallelTraceExpansion.
var INNER_WORKERS uint

// innerWorkers returns the effective number of workers, capped to ensure each
// goroutine processes at least minRowsPerWorker rows.
func innerWorkers(height uint) uint {
	workers := INNER_WORKERS
	if workers == 0 {
		workers = uint(runtime.GOMAXPROCS(0))
	}

	if workers > height/minRowsPerWorker {
		workers = height / minRowsPerWorker
	}

	if workers < 1 {
		workers = 1
	}

	return workers
}

// ComputedRegister describes a column whose values are computed on-demand, rather
// than being stored in a data array.  Typically computed columns read values
// from other columns in a trace in order to calculate their value.  There is an
// expectation that this computation is acyclic.  Furthermore, computed columns
// give rise to "trace expansion".  That is where the initial trace provided by
// the user is expanded by determining the value of all computed columns.
type ComputedRegister[F field.Element[F]] struct {
	// Module in which expression is evaluated
	Module schema.ModuleId
	// Target indices for computed column
	Targets []register.Id
	// The computation which accepts a given trace and computes
	// the value of this column at a given row.
	Expr term.Computation[word.BigEndian]
	// Direction in which value is computed (true = forward, false = backward).
	// More specifically, a forwards direction means the computation starts on
	// the first row, whilst a backwards direction means it starts on the last.
	Direction bool
}

// NewComputedRegister constructs a new set of computed column(s) with a given
// determining expression.  More specifically, that expression is used to
// compute the values for the columns during trace expansion.  For each, the
// resulting value is split across the target columns.
func NewComputedRegister[F field.Element[F]](expr term.Computation[word.BigEndian], dir bool, module schema.ModuleId,
	limbs ...register.Id) *ComputedRegister[F] {
	//
	if len(limbs) == 0 {
		panic("computed register requires at least one limb")
	}
	//
	return &ComputedRegister[F]{module, limbs, expr, dir}
}

// Bounds determines the well-definedness bounds for this assignment for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
func (p *ComputedRegister[F]) Bounds(mid sc.ModuleId) util.Bounds {
	if mid == p.Module {
		return p.Expr.Bounds()
	}
	// Not relevant
	return util.EMPTY_BOUND
}

// Compute the values of columns defined by this assignment. Specifically, this
// creates a new column which contains the result of evaluating a given
// expression on each row.
func (p *ComputedRegister[F]) Compute(tr trace.Trace[F], schema schema.AnySchema[F],
) ([]array.MutArray[F], error) {
	var (
		trModule = trace.ModuleAdapter[F, word.BigEndian](tr.Module(p.Module))
		scModule = schema.Module(p.Module)
		err      error
	)
	// Determine multiplied height
	height := trModule.Height()
	bitwidths := make([]uint, len(p.Targets))
	//
	for i, target := range p.Targets {
		bitwidths[i] = scModule.Register(target).Width()
	}
	// Non-recursive computations can be parallelised across rows since each
	// row's evaluation is independent.
	if !p.IsRecursive() {
		data := make([][]F, len(p.Targets))
		for i := range p.Targets {
			data[i] = make([]F, height)
		}

		if polyFil, ok := p.Expr.(*agnostic.PolyFil); ok {
			err = fwdCompiledPolyFilParallel(height, data, bitwidths, newCompiledPolyFil(polyFil, trModule))
		} else {
			err = fwdComputationParallelDirect(height, data, bitwidths, p.Expr, trModule, scModule, p.Module)
		}

		if err != nil {
			return nil, err
		}

		return materializeFieldColumns(data, tr), nil
	}
	// Recursive computations must remain sequential since each row depends on
	// previous rows.
	wrapper := recursiveModule{
		col:      p.Targets,
		data:     make([][]word.BigEndian, len(p.Targets)),
		trModule: trModule,
	}

	for i := range p.Targets {
		wrapper.data[i] = make([]word.BigEndian, height)
	}

	if p.Direction {
		err = fwdComputation(height, wrapper.data, bitwidths, p.Expr, &wrapper, scModule, p.Module)
	} else {
		err = bwdComputation(height, wrapper.data, bitwidths, p.Expr, &wrapper, scModule, p.Module)
	}
	// Sanity check
	if err != nil {
		return nil, err
	}
	// Done
	return concretizeColumns(wrapper.data, tr), err
}

// fieldWordSplitter splits a word.BigEndian value into per-limb field elements
// according to the given bitwidths.
type fieldWordSplitter[F field.Element[F]] struct {
	widths     []uint
	limit      word.BigEndian
	workingSet []byte
	limbBuf    [32]byte
}

func newFieldWordSplitter[F field.Element[F]](widths []uint) fieldWordSplitter[F] {
	totalBits := uint(0)
	for _, width := range widths {
		totalBits += width
	}

	return fieldWordSplitter[F]{
		widths:     widths,
		limit:      field.TwoPowN[word.BigEndian](totalBits),
		workingSet: make([]byte, word.ByteWidth(totalBits)),
	}
}

func (s *fieldWordSplitter[F]) write(row uint, val word.BigEndian, data [][]F) bool {
	if val.Cmp(s.limit) >= 0 {
		return false
	}

	val.PutBytes(s.workingSet)
	array.ReverseInPlace(s.workingSet)

	reader := bit.NewReader(s.workingSet)

	for i, width := range s.widths {
		n := reader.LittleEndianReadInto(width, s.limbBuf[:])
		array.ReverseInPlace(s.limbBuf[:n])

		var element F

		data[i][row] = element.SetBytes(s.limbBuf[:n])
	}

	return true
}

// compiledPolyFil pre-resolves column references for a PolyFil expression,
// avoiding repeated lookups during per-row evaluation.
type compiledPolyFil struct {
	rightShift uint
	terms      []compiledPolyFilTerm
}

type compiledPolyFilTerm struct {
	coefficient word.BigEndian
	accesses    []compiledPolyFilAccess
}

type compiledPolyFilAccess struct {
	column trace.Column[word.BigEndian]
	shift  int
}

func newCompiledPolyFil(polyFil *agnostic.PolyFil, mod trace.Module[word.BigEndian]) compiledPolyFil {
	poly := polyFil.Polynomial()
	terms := make([]compiledPolyFilTerm, poly.Len())

	for i := range poly.Len() {
		monomial := poly.Term(i)
		coefficient := monomial.Coefficient()
		terms[i].coefficient = word.BigEndian{}.SetBytes(coefficient.Bytes())
		terms[i].accesses = make([]compiledPolyFilAccess, monomial.Len())

		for j := range monomial.Len() {
			access := monomial.Nth(j)
			terms[i].accesses[j] = compiledPolyFilAccess{
				column: mod.Column(access.Unwrap()),
				shift:  access.RelativeShift(),
			}
		}
	}

	return compiledPolyFil{
		rightShift: polyFil.RightShift(),
		terms:      terms,
	}
}

func (p compiledPolyFil) evalAt(row uint) word.BigEndian {
	var result word.BigEndian

	for i, term := range p.terms {
		value := term.coefficient
		for _, access := range term.accesses {
			value = value.Mul(access.column.Get(int(row) + access.shift))
		}

		if i == 0 {
			result = value
		} else {
			result = result.Add(value)
		}
	}

	if p.rightShift != 0 {
		return result.Rsh(p.rightShift)
	}

	return result
}

func materializeFieldColumns[F field.Element[F]](data [][]F, tr trace.Trace[F]) []array.MutArray[F] {
	cols := make([]array.MutArray[F], len(data))

	for i, d := range data {
		col := tr.Builder().NewArray(uint(len(d)), math.MaxUint)
		for j, value := range d {
			col = col.Set(uint(j), value)
		}

		cols[i] = col
	}

	return cols
}

func concretizeColumns[F field.Element[F]](data [][]word.BigEndian, tr trace.Trace[F]) []array.MutArray[F] {
	var cols = make([]array.MutArray[F], len(data))
	//
	for i, d := range data {
		cols[i] = concretizeColumn(d, tr)
	}
	//
	return cols
}

func concretizeColumn[F field.Element[F]](data []word.BigEndian, tr trace.Trace[F]) array.MutArray[F] {
	var (
		// FIXME: using a large bitwidth here ensures the underlying data is
		// represented using a full field element, rather than e.g. some smaller
		// number of bytes.  This is needed to handle reject tests which can produce
		// values outside the range of the computed register, but which we still
		// want to check are actually rejected (i.e. since they are simulating what
		// an attacker might do).
		col = tr.Builder().NewArray(uint(len(data)), math.MaxUint)
	)
	//
	for i, word := range data {
		var element F
		// Assign value
		col.Set(uint(i), element.SetBytes(word.Bytes()))
	}
	//
	return col
}

// Consistent performs some simple checks that the given assignment is
// consistent with its enclosing schema This provides a double check of certain
// key properties, such as that registers used for assignments are valid,
// etc.
func (p *ComputedRegister[F]) Consistent(schema sc.AnySchema[F]) []error {
	return nil
}

// IsRecursive checks whether or not this computation is recursive (i.e. the
// target column is defined in terms of itself).
func (p *ComputedRegister[F]) IsRecursive() bool {
	var regs = p.Expr.RequiredRegisters()
	// Walk through registers accessed by the computation and see whether target
	// register is amongst them.
	for i, iter := 0, regs.Iter(); iter.HasNext(); i++ {
		rid := register.NewId(iter.Next())
		// Did we find it?
		if slices.Contains(p.Targets, rid) {
			// Yes!
			return true
		}
	}
	//
	return false
}

// RegistersExpanded identifies registers expanded by this assignment.
func (p *ComputedRegister[F]) RegistersExpanded() []register.Ref {
	return nil
}

// RegistersRead returns the set of columns that this assignment depends upon.
// That can include both input columns, as well as other computed columns.
func (p *ComputedRegister[F]) RegistersRead() []register.Ref {
	var (
		module = p.Module
		regs   = p.Expr.RequiredRegisters()
		rids   = make([]register.Ref, regs.Iter().Count())
	)
	//
	for i, iter := 0, regs.Iter(); iter.HasNext(); i++ {
		rid := register.NewId(iter.Next())
		rids[i] = register.NewRef(module, rid)
	}
	// Remove target to allow recursive definitions.  Observe this does not
	// guarantee they make sense!
	return array.RemoveMatching(rids, func(r register.Ref) bool {
		if r.Module() == p.Module {
			return slices.Contains(p.Targets, r.Column())
		}
		//
		return false
	})
}

// RegistersWritten identifies registers assigned by this assignment.
func (p *ComputedRegister[F]) RegistersWritten() []register.Ref {
	var written = make([]register.Ref, len(p.Targets))
	//
	for i, r := range p.Targets {
		written[i] = register.NewRef(p.Module, r)
	}
	//
	return written
}

// Substitute any matchined labelled constants within this assignment
func (p *ComputedRegister[F]) Substitute(mapping map[string]F) {
	var tmp any = mapping
	// NOTE: this is the only scenario under which this method can be called.
	w, ok := tmp.(map[string]word.BigEndian)
	// sanity check
	if !ok {
		panic("unreachable")
	}
	//
	p.Expr.Substitute(w)
}

// Lisp converts this constraint into an S-Expression.
//
//nolint:revive
func (p *ComputedRegister[F]) Lisp(schema sc.AnySchema[F]) sexp.SExp {
	var (
		module  = schema.Module(p.Module)
		targets = make([]sexp.SExp, len(p.Targets))
	)
	//
	for i, t := range p.Targets {
		var (
			datatype = "𝔽"
			ith      = module.Register(t)
		)
		if !ith.IsNative() {
			datatype = fmt.Sprintf("u%d", ith.Width())
		}

		targets[i] = sexp.NewList([]sexp.SExp{
			sexp.NewSymbol(ith.QualifiedName(module)), sexp.NewSymbol(datatype),
		})
	}
	//
	return sexp.NewList(
		[]sexp.SExp{sexp.NewSymbol("compute"),
			sexp.NewList(targets),
			p.Expr.Lisp(false, module),
		})
}

func fwdComputation(height uint, data [][]word.BigEndian, widths []uint, expr term.Evaluable[word.BigEndian],
	trMod trace.Module[word.BigEndian], scMod register.Map, ctx schema.ModuleId) error {
	// Forwards computation
	for i := range height {
		val, err := expr.EvalAt(int(i), trMod, scMod)
		// error check
		if err != nil {
			e := fmt.Sprintf("%s for %s", err.Error(), expr.Lisp(false, scMod).String(true))
			return constraint.NewInternalFailure[word.BigEndian](scMod.Name().String(), ctx, i, expr, e)
		}
		// Write data across limbs
		write(i, val, data, widths)
	}
	//
	return nil
}

// fwdComputationParallelDirect splits the row range into chunks and evaluates
// them concurrently, writing results directly to field elements. This is safe
// for non-recursive computations where each row's evaluation is independent.
func fwdComputationParallelDirect[F field.Element[F]](height uint, data [][]F, widths []uint,
	expr term.Evaluable[word.BigEndian], trMod trace.Module[word.BigEndian], scMod register.Map,
	ctx schema.ModuleId) error {
	//
	if height < minParallelHeight {
		return fwdComputationDirect(height, data, widths, expr, trMod, scMod, ctx)
	}

	numWorkers := innerWorkers(height)
	chunkSize := (height + numWorkers - 1) / numWorkers

	var (
		firstErr error
		mu       sync.Mutex
		wg       sync.WaitGroup
	)

	for w := uint(0); w < numWorkers; w++ {
		start := w * chunkSize
		end := min(start+chunkSize, height)

		wg.Add(1)

		go func(start, end uint) {
			defer wg.Done()

			splitter := newFieldWordSplitter[F](widths)

			for i := start; i < end; i++ {
				val, err := expr.EvalAt(int(i), trMod, scMod)
				if err != nil {
					mu.Lock()

					if firstErr == nil {
						e := fmt.Sprintf("%s for %s", err.Error(), expr.Lisp(false, scMod).String(true))
						firstErr = constraint.NewInternalFailure[word.BigEndian](scMod.Name().String(), ctx, i, expr, e)
					}

					mu.Unlock()

					return
				}

				splitter.write(i, val, data)
			}
		}(start, end)
	}

	wg.Wait()

	return firstErr
}

// fwdCompiledPolyFilParallel evaluates a pre-compiled PolyFil expression in
// parallel across row chunks.
func fwdCompiledPolyFilParallel[F field.Element[F]](height uint, data [][]F, widths []uint,
	poly compiledPolyFil) error {
	//
	if height < minParallelHeight {
		return fwdCompiledPolyFil(height, data, widths, poly)
	}

	numWorkers := innerWorkers(height)
	chunkSize := (height + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup

	for w := uint(0); w < numWorkers; w++ {
		start := w * chunkSize
		end := min(start+chunkSize, height)

		wg.Add(1)

		go func(start, end uint) {
			defer wg.Done()

			splitter := newFieldWordSplitter[F](widths)

			for i := start; i < end; i++ {
				splitter.write(i, poly.evalAt(i), data)
			}
		}(start, end)
	}

	wg.Wait()

	return nil
}

func fwdCompiledPolyFil[F field.Element[F]](height uint, data [][]F, widths []uint,
	poly compiledPolyFil) error {
	splitter := newFieldWordSplitter[F](widths)

	for i := range height {
		splitter.write(i, poly.evalAt(i), data)
	}

	return nil
}

func fwdComputationDirect[F field.Element[F]](height uint, data [][]F, widths []uint,
	expr term.Evaluable[word.BigEndian], trMod trace.Module[word.BigEndian], scMod register.Map,
	ctx schema.ModuleId) error {
	splitter := newFieldWordSplitter[F](widths)

	for i := range height {
		val, err := expr.EvalAt(int(i), trMod, scMod)
		if err != nil {
			e := fmt.Sprintf("%s for %s", err.Error(), expr.Lisp(false, scMod).String(true))
			return constraint.NewInternalFailure[word.BigEndian](scMod.Name().String(), ctx, i, expr, e)
		}

		splitter.write(i, val, data)
	}

	return nil
}

func bwdComputation(height uint, data [][]word.BigEndian, widths []uint, expr term.Evaluable[word.BigEndian],
	trMod trace.Module[word.BigEndian], scMod register.Map, ctx schema.ModuleId) error {
	// Backwards computation
	for i := height; i > 0; i-- {
		val, err := expr.EvalAt(int(i-1), trMod, scMod)
		// error check
		if err != nil {
			e := fmt.Sprintf("%s for %s", err.Error(), expr.Lisp(false, scMod).String(true))
			return constraint.NewInternalFailure[word.BigEndian](scMod.Name().String(), ctx, i-1, expr, e)
		}
		// Write data across limbs
		write(i-1, val, data, widths)
	}
	//
	return nil
}

func write(row uint, val word.BigEndian, data [][]word.BigEndian, bitwidths []uint) {
	var elements, ok = field.SplitWord[word.BigEndian](val, bitwidths)
	// Only write data if value was within bounds; otherwise, leave as zero
	// (which should be caught as a constraint failure if its invalid).
	if ok {
		for i := range data {
			data[i][row] = elements[i]
		}
	}
}

// RecModule is a wrapper which enables a computation to be recursive.
// Specifically, it allows the expression being evaluated to access as it is
// being generated.
type recursiveModule struct {
	col      []register.Id
	data     [][]word.BigEndian
	trModule trace.Module[word.BigEndian]
}

// Module implementation for trace.Module interface.
func (p *recursiveModule) Name() module.Name {
	return p.trModule.Name()
}

// Column implementation for trace.Module interface.
func (p *recursiveModule) Column(index uint) trace.Column[word.BigEndian] {
	for i, cid := range p.col {
		if cid.Unwrap() == index {
			return &recursiveColumn{p.data[i]}
		}
	}

	return p.trModule.Column(index)
}

// ColumnOf implementation for trace.Module interface.
func (p *recursiveModule) ColumnOf(string) trace.Column[word.BigEndian] {
	// NOTE: this is marked unreachable because, as it stands, expression
	// evaluation never calls this method.
	panic("unreachable")
}

// FindLast implementation for the trace.Module interface.
func (p *recursiveModule) FindLast(...word.BigEndian) uint {
	panic("unsupported operation")
}

// Keys implementation for the trace.Module interface.
func (p *recursiveModule) Keys() uint {
	panic("unsupported operation")
}

// Width implementation for trace.Module interface.
func (p *recursiveModule) Width() uint {
	return p.trModule.Width()
}

// Height implementation for trace.Module interface.
func (p *recursiveModule) Height() uint {
	return p.trModule.Height()
}

// RecColumn is a wrapper which enables the array being computed to be accessed
// during its own computation.
type recursiveColumn struct {
	data []word.BigEndian
}

// Holds the name of this column
func (p *recursiveColumn) Name() string {
	panic("unreachable")
}

// Get implementation for trace.Column interface.
func (p *recursiveColumn) Get(row int) word.BigEndian {
	if row < 0 || row >= len(p.data) {
		// out-of-bounds access
		return field.Zero[word.BigEndian]()
	}
	//
	return p.data[row]
}

// Data implementation for trace.Column interface.
func (p *recursiveColumn) Data() array.Array[word.BigEndian] {
	panic("unreachable")
}

// Padding implementation for trace.Column interface.
func (p *recursiveColumn) Padding() word.BigEndian {
	panic("unreachable")
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

func init() {
	gob.Register(sc.Assignment[word.BigEndian](&ComputedRegister[word.BigEndian]{}))
}
