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
package ast

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/iter"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// Circuit represents the root of the Abstract Syntax Tree.  This is also
// referred to as the "prelude".  All modules are contained within the root, and
// declarations can also be declared here as well.
type Circuit struct {
	Modules      []Module
	Declarations []Declaration
}

// Module represents a top-level module declaration.  This corresponds to a
// table in the final constraint set.
type Module struct {
	Name         string
	Declarations []Declaration
}

// Add a new declaration into this module.
func (p *Module) Add(decl Declaration) {
	p.Declarations = append(p.Declarations, decl)
}

// Node provides common functionality across all elements of the Abstract Syntax
// Tree.  For example, it ensures every element can converted back into Lisp
// form for debugging.  Furthermore, it provides a reference point for
// constructing a suitable source map for reporting syntax errors.
type Node interface {
	// Convert this node into its lisp representation.  This is primarily used
	// for debugging purposes.
	Lisp() sexp.SExp
}

// Declaration represents a top-level declaration in a Corset source file (e.g.
// defconstraint, defcolumns, etc).
type Declaration interface {
	Node
	// Returns the set of symbols being defined this declaration.  Observe that
	// these may not yet have been finalised.
	Definitions() iter.Iterator[SymbolDefinition]
	// Return set of columns on which this declaration depends.
	Dependencies() iter.Iterator[Symbol]
	// Check whether this declaration defines a given symbol.  The symbol in
	// question needs to have been resolved already for this to make sense.
	Defines(Symbol) bool
	// Check whether this declaration is finalised already.
	IsFinalised() bool
}

// ============================================================================
// defcolumns
// ============================================================================

// DefColumns captures a set of one or more columns being declared.
type DefColumns struct {
	Columns []*DefColumn
}

// NewDefColumns constructs a new instance of DefColumns.
func NewDefColumns(columns []*DefColumn) *DefColumns {
	return &DefColumns{columns}
}

// Dependencies needed to signal declaration.
func (p *DefColumns) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator[Symbol](nil)
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefColumns) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewArrayIterator(p.Columns)
	return iter.NewCastIterator[*DefColumn, SymbolDefinition](iterator)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefColumns) Defines(symbol Symbol) bool {
	for _, sym := range p.Columns {
		if &sym.binding == symbol.Binding() {
			return true
		}
	}
	//
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefColumns) IsFinalised() bool {
	return true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefColumns) Lisp() sexp.SExp {
	list := sexp.EmptyList()
	list.Append(sexp.NewSymbol("defcolumns"))
	// Add lisp for each individual column
	for _, c := range p.Columns {
		list.Append(c.Lisp())
	}
	// Done
	return list
}

// DefColumn packages together those piece relevant to declaring an individual
// column, such its name and type.
type DefColumn struct {
	// Binding of this column (which may or may not be finalised).
	binding ColumnBinding
}

var _ SymbolDefinition = &DefColumn{}

// NewDefColumn constructs a new (non-computed) column declaration.  Such a
// column is automatically finalised, since all information is provided at the
// point of creation.
func NewDefColumn(binding ColumnBinding) *DefColumn {
	return &DefColumn{binding}
}

// Binding returns the allocated binding for this symbol (which may or may not
// be finalised).
func (e *DefColumn) Binding() Binding {
	return &e.binding
}

// InnerBinding returns the allocated binding for this symbol (which may or may
// not be finalised).
func (e *DefColumn) InnerBinding() ColumnBinding {
	return e.binding
}

// Name returns the (unqualified) name of this symbol.  For example, "X" for
// a column X defined in a module m1.
func (e *DefColumn) Name() string {
	return e.binding.Path.Tail()
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (e *DefColumn) Path() *file.Path {
	return &e.binding.Path
}

// DataType returns the type of this column.  If this column have not yet been
// finalised, then this will panic.
func (e *DefColumn) DataType() Type {
	if !e.binding.IsFinalised() {
		panic("unfinalised column")
	}
	//
	return e.binding.DataType
}

// MustProve determines whether or not the type of this column must be
// established by the prover (e.g. a range constraint or similar).
func (e *DefColumn) MustProve() bool {
	if !e.binding.IsFinalised() {
		panic("unfinalised column")
	}
	//
	return e.binding.MustProve
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (e *DefColumn) Lisp() sexp.SExp {
	list := sexp.EmptyList()
	list.Append(sexp.NewSymbol(e.Name()))
	//
	if e.binding.DataType != nil {
		datatype := e.binding.DataType.String()
		if e.binding.MustProve {
			datatype = fmt.Sprintf(":%s@prove", datatype)
		}

		list.Append(sexp.NewSymbol(datatype))
	}
	//
	if list.Len() == 1 {
		return list.Get(0)
	}
	//
	return list
}

// ============================================================================
// defconst
// ============================================================================

// DefConst represents the declaration of one of more constant values which can
// be used within expressions to improve readability.
type DefConst struct {
	// List of constant pairs.  Observe that every expression in this list must
	// be constant (i.e. it cannot refer to column values or call impure
	// functions, etc).
	Constants []*DefConstUnit
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefConst) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewArrayIterator[*DefConstUnit](p.Constants)
	return iter.NewCastIterator[*DefConstUnit, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefConst) Dependencies() iter.Iterator[Symbol] {
	var deps []Symbol
	// Combine dependencies from all constants defined within.
	for _, d := range p.Constants {
		deps = append(deps, d.ConstBinding.Value.Dependencies()...)
	}
	// Done
	return iter.NewArrayIterator[Symbol](deps)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefConst) Defines(symbol Symbol) bool {
	for _, sym := range p.Constants {
		if &sym.ConstBinding == symbol.Binding() {
			return true
		}
	}
	//
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefConst) IsFinalised() bool {
	for _, c := range p.Constants {
		if !c.ConstBinding.IsFinalised() {
			return false
		}
	}
	//
	return true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefConst) Lisp() sexp.SExp {
	def := sexp.EmptyList()
	def.Append(sexp.NewSymbol("defconst"))
	//
	for _, c := range p.Constants {
		def.Append(sexp.NewSymbol(c.Name()))
		def.Append(c.ConstBinding.Value.Lisp())
	}
	// Done
	return def
}

// DefConstUnit represents the definition of exactly one constant value.  As
// such, this is an instance of SymbolDefinition and provides a binding.
type DefConstUnit struct {
	// Binding for this constant.
	ConstBinding ConstantBinding
}

// Binding returns the allocated binding for this symbol (which may or may not
// be finalised).
func (e *DefConstUnit) Binding() Binding {
	return &e.ConstBinding
}

// Name returns the (unqualified) name of this symbol.  For example, "X" for
// a column X defined in a module m1.
func (e *DefConstUnit) Name() string {
	return e.ConstBinding.Path.Tail()
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (e *DefConstUnit) Path() *file.Path {
	return &e.ConstBinding.Path
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
//
//nolint:revive
func (e *DefConstUnit) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(e.Name()),
		e.ConstBinding.Value.Lisp()})
}

// ============================================================================
// defconstraint
// ============================================================================

// DefConstraint represents a vanishing constraint, which is either "local" or
// "global".  A local constraint applies either to the first or last rows,
// whilst a global constraint applies to all rows.  For a constraint to hold,
// its expression must evaluate to zero for the rows on which it is active.  A
// constraint may also have a "guard" which is an expression that must evaluate
// to a non-zero value for the constraint to be considered active.  The
// expression for a constraint must have a single context.  That is, it can only
// be applied to columns within the same module (i.e. to ensure they have the
// same height).
type DefConstraint struct {
	// Unique handle given to this constraint.  This is primarily useful for
	// debugging (i.e. so we know which constraint failed, etc).
	Handle string
	// Domain of this constraint which, if empty, indicates a global constraint.
	// Otherwise, a given value indicates a single row on which this constraint
	// should apply (where negative values are taken from the end, meaning that
	// -1 represents the last row of a given module).
	Domain util.Option[int]
	// A selector which determines for which rows this constraint is active.
	// Specifically, when the expression evaluates to a non-zero value then the
	// constraint is active; otherwise, its inactive. Nil is permitted to
	// indicate no guard is present.
	Guard Expr
	// The constraint itself which (when active) should evaluate to zero for the
	// relevant set of rows.
	Constraint Expr
	//
	finalised bool
}

// NewDefConstraint constructs a new (unfinalised) constraint.
func NewDefConstraint(handle string, domain util.Option[int], guard Expr, constraint Expr) *DefConstraint {
	return &DefConstraint{handle, domain, guard, constraint, false}
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefConstraint) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefConstraint) Dependencies() iter.Iterator[Symbol] {
	var deps []Symbol
	// Extract guard's dependencies (if applicable)
	if p.Guard != nil {
		deps = p.Guard.Dependencies()
	}
	// Extract bodies dependencies
	deps = append(deps, p.Constraint.Dependencies()...)
	// Done
	return iter.NewArrayIterator[Symbol](deps)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefConstraint) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefConstraint) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, which means that its guard (if applicable) and
// body have been resolved.
func (p *DefConstraint) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefConstraint) Lisp() sexp.SExp {
	modifiers := sexp.EmptyList()
	// domain
	if p.Domain.HasValue() {
		domain := fmt.Sprintf("{%d}", p.Domain.Unwrap())
		//
		modifiers.Append(sexp.NewSymbol(":domain"))
		modifiers.Append(sexp.NewSymbol(domain))
	}
	//
	if p.Guard != nil {
		modifiers.Append(sexp.NewSymbol(":guard"))
		modifiers.Append(p.Guard.Lisp())
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defconstraint"),
		sexp.NewSymbol(p.Handle),
		modifiers,
		p.Constraint.Lisp()})
}

// ============================================================================
// definrange
// ============================================================================

// DefInRange restricts all values for a given column to be within a range
// [0..n) for some bound n.  Any bound is supported, and the system will choose
// the best underlying implementation as needed.
//
// NOTE: as for a lookup, the constrained column is a column access rather than
// an arbitrary expression.  This reflects what the underlying constraint system
// supports.
type DefInRange struct {
	// The column whose values are being constrained to within the given bound.
	Column TypedSymbol
	// Bitwidth determines the bitwidth that this range constraint is enforcing.
	Bitwidth uint
	// Indicates whether or not the column access has been resolved.
	finalised bool
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefInRange) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefInRange) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator[Symbol]([]Symbol{p.Column})
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefInRange) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefInRange) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, meaning that the column access has been resolved.
func (p *DefInRange) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefInRange) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("definrange"),
		p.Column.Lisp(),
		sexp.NewSymbol(fmt.Sprintf("u%d", p.Bitwidth)),
	})
}

// ============================================================================
// deflookup
// ============================================================================

// DefLookup represents a lookup constraint between a set N of source columns
// and a set of N target columns.  The source columns must have a single context
// (i.e. all be in the same module) and likewise for the target columns (though
// the source and target contexts can differ).  The constraint can be viewed as
// a "subset constraint".  Let the set of "source tuples" be those obtained by
// reading the source columns over all rows in the source context, and likewise
// the "target tuples" those for the target columns in the target context.  Then
// the lookup constraint holds if the set of source tuples is a subset of the
// target tuples.  This does not need to be a strict subset, so the two sets can
// be identical.  Furthermore, these are not treated as multi-sets, hence the
// number of occurrences of a given tuple is not relevant.
//
// NOTE: sources and targets (along with their selectors) are column accesses
// rather than arbitrary expressions.  This reflects what the underlying
// constraint system supports.
type DefLookup struct {
	// Unique handle given to this constraint.  This is primarily useful for
	// debugging (i.e. so we know which constraint failed, etc).
	Handle string
	// Checked determines whether or not "type checking" is enabled for source /
	// target pairs.
	Checked bool
	// Source selectors (nil entries mean no selector for corresponding source).
	SourceSelectors []TypedSymbol
	// Source columns for lookup (i.e. these values must all be contained within
	// the targets).
	Sources [][]TypedSymbol
	// Target selectors (nil entries mean no selector for corresponding source).
	TargetSelectors []TypedSymbol
	// Target columns for lookup (i.e. these values must contain all of the
	// source values, but may contain more).
	Targets [][]TypedSymbol
	// Indicates whether or not target and source columns have been resolved.
	finalised bool
}

// NewDefLookup creates a new (unfinalised) lookup constraint.
func NewDefLookup(handle string, checked bool, sourceSelectors []TypedSymbol, sources [][]TypedSymbol,
	targetSelectors []TypedSymbol, targets [][]TypedSymbol) *DefLookup {
	//
	return &DefLookup{handle, checked, sourceSelectors, sources, targetSelectors, targets, false}
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefLookup) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefLookup) Dependencies() iter.Iterator[Symbol] {
	var (
		sources = dependenciesOfLookupVectors(p.SourceSelectors, p.Sources)
		targets = dependenciesOfLookupVectors(p.TargetSelectors, p.Targets)
		viter   = iter.NewArrayIterator(append(sources, targets...))
	)
	// put it altogether
	return iter.NewCastIterator[TypedSymbol, Symbol](viter)
}

// dependenciesOfLookupVectors returns the given symbols as dependencies.  That
// is, each symbol depends only upon itself.
func dependenciesOfLookupVectors(selectors []TypedSymbol, targets [][]TypedSymbol) []TypedSymbol {
	// Filter out all empty selectors
	selectors = array.Filter(selectors, func(t TypedSymbol) bool { return t != nil })
	// Flattern targets
	flat := array.FlatMap(targets, func(f []TypedSymbol) []TypedSymbol {
		return f
	})
	//
	return append(selectors, flat...)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefLookup) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefLookup) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, which means that all source and target expressions
// have been resolved.
func (p *DefLookup) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefLookup) Lisp() sexp.SExp {
	targets := make([]sexp.SExp, len(p.Targets))
	targetSelectors := make([]sexp.SExp, len(p.Targets))
	sources := make([]sexp.SExp, len(p.Sources))
	sourceSelectors := make([]sexp.SExp, len(p.Targets))
	// Targets
	for i, target := range p.Targets {
		ith := make([]sexp.SExp, len(target))
		//
		for j, t := range target {
			ith[j] = t.Lisp()
		}
		//
		targets[i] = sexp.NewList(ith)
		//
		if p.TargetSelectors[i] != nil {
			targetSelectors[i] = p.TargetSelectors[i].Lisp()
		} else {
			targetSelectors[i] = sexp.NewSymbol("_")
		}
	}
	// Targets
	for i, source := range p.Sources {
		ith := make([]sexp.SExp, len(source))
		//
		for j, t := range source {
			ith[j] = t.Lisp()
		}
		//
		sources[i] = sexp.NewList(ith)
		//
		if p.SourceSelectors[i] != nil {
			sourceSelectors[i] = p.SourceSelectors[i].Lisp()
		} else {
			sourceSelectors[i] = sexp.NewSymbol("_")
		}
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("deflookup"),
		sexp.NewSymbol(p.Handle),
		sexp.NewList(targetSelectors),
		sexp.NewList(targets),
		sexp.NewList(sourceSelectors),
		sexp.NewList(sources),
	})
}

// ============================================================================
// defsend / defrecv
// ============================================================================

// DefSendReceive represents one port of a bus.  That is, a declaration that,
// on every row where the selector is non-zero, the given columns form a
// message sent (or received) on the named bus.  All ports sharing a bus name
// are merged into a single bus constraint during translation, which requires
// everything sent on a bus to also be received on it (and vice versa).
//
// NOTE: the selector and columns are column accesses rather than arbitrary
// expressions, and the selector is mandatory (padding rows must stay off the
// bus).
type DefSendReceive struct {
	// Unique handle given to this declaration, useful for debugging.
	Handle string
	// Bus is the name of the bus.  Bus names form a global namespace, and a
	// bus exists simply by virtue of being named by some port.
	Bus string
	// IsSend distinguishes a send port (true) from a receive port (false).
	IsSend bool
	// Selector gates which rows contribute a message.
	Selector TypedSymbol
	// Columns making up the message.
	Columns []TypedSymbol
	// Indicates whether or not the selector and columns have been resolved.
	finalised bool
}

// NewDefSendReceive creates a new (unfinalised) send / receive declaration.
func NewDefSendReceive(handle string, bus string, isSend bool, selector TypedSymbol,
	columns []TypedSymbol) *DefSendReceive {
	//
	return &DefSendReceive{handle, bus, isSend, selector, columns, false}
}

// Definitions returns the set of symbols defined by this declaration.
func (p *DefSendReceive) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefSendReceive) Dependencies() iter.Iterator[Symbol] {
	var (
		symbols = append([]TypedSymbol{p.Selector}, p.Columns...)
		viter   = iter.NewArrayIterator(symbols)
	)
	//
	return iter.NewCastIterator[TypedSymbol, Symbol](viter)
}

// Defines checks whether this declaration defines the given symbol.
func (p *DefSendReceive) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.
func (p *DefSendReceive) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, meaning its selector and columns have been
// resolved.
func (p *DefSendReceive) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily
// used for debugging purposes.
func (p *DefSendReceive) Lisp() sexp.SExp {
	var (
		keyword = "defsend"
		columns = make([]sexp.SExp, len(p.Columns))
	)
	//
	if !p.IsSend {
		keyword = "defrecv"
	}
	//
	for i, c := range p.Columns {
		columns[i] = c.Lisp()
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol(keyword),
		sexp.NewSymbol(p.Handle),
		sexp.NewSymbol(p.Bus),
		p.Selector.Lisp(),
		sexp.NewList(columns),
	})
}
