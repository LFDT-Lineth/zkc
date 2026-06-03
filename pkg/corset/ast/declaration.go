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
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util"
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
// defalias
// ============================================================================

// DefAliases represents the declaration of one or more aliases.  That is,
// alternate names for existing symbols.
type DefAliases struct {
	// Aliases
	Aliases []*DefAlias
	// Symbols being aliased
	Symbols []Symbol
}

// NewDefAliases constructs a new instance of DefAliases.
func NewDefAliases(aliases []*DefAlias, symbols []Symbol) *DefAliases {
	return &DefAliases{aliases, symbols}
}

// Dependencies needed to signal declaration.
func (p *DefAliases) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator[Symbol](nil)
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefAliases) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefAliases) Defines(symbol Symbol) bool {
	// fine because defaliases gets special treatment.
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefAliases) IsFinalised() bool {
	// Fine because defaliases doesn't really do anything with its symbols.
	return true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
//
//nolint:revive
func (p *DefAliases) Lisp() sexp.SExp {
	pairs := sexp.EmptyList()
	//
	for i, a := range p.Aliases {
		pairs.Append(sexp.NewSymbol(a.Name))
		pairs.Append(p.Symbols[i].Lisp())
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defalias"), pairs,
	})
}

// DefAlias provides a node on which to hang source information to an alias name.
type DefAlias struct {
	// Name of the alias
	Name string
}

// NewDefAlias constructs a new instance of DefAlias.
func NewDefAlias(name string) *DefAlias {
	return &DefAlias{name}
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
//
//nolint:revive
func (p *DefAlias) Lisp() sexp.SExp {
	return sexp.NewSymbol(p.Name)
}

// ============================================================================
// defcall
// ============================================================================

// DefCall captures a function call between a lisp module and an assembly
// function.  A key feature of this is that it triggers trace propagation.
type DefCall struct {
	// Returns for the call
	Returns []Expr
	// Function being called
	Function string
	// Arguments for the call
	Arguments []Expr
	// Optional source selector
	Selector util.Option[Expr]
	// determines whether or not this has been finalised.
	finalised bool
}

// NewDefCall creates a new (unfinalised) function call.
func NewDefCall(returns []Expr, fun string, args []Expr, selector util.Option[Expr]) *DefCall {
	//
	return &DefCall{returns, fun, args, selector, false}
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefCall) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefCall) Dependencies() iter.Iterator[Symbol] {
	var deps []Symbol
	//
	deps = append(deps, DependenciesOfExpressions(p.Arguments)...)
	deps = append(deps, DependenciesOfExpressions(p.Returns)...)
	// Include selector dependencies (if applicable)
	if p.Selector.HasValue() {
		deps = append(deps, p.Selector.Unwrap().Dependencies()...)
	}
	// Combine deps
	return iter.NewArrayIterator(deps)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefCall) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefCall) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, which means that all source and target expressions
// have been resolved.
func (p *DefCall) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefCall) Lisp() sexp.SExp {
	returns := make([]sexp.SExp, len(p.Returns))
	args := make([]sexp.SExp, len(p.Arguments))
	// Returns
	for i, t := range p.Returns {
		returns[i] = t.Lisp()
	}
	// Arguments
	for i, t := range p.Arguments {
		args[i] = t.Lisp()
	}
	//
	list := sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defcall"),
		sexp.NewList(returns),
		sexp.NewSymbol(p.Function),
		sexp.NewList(args),
	})
	// Include selector (if applicable)
	if p.Selector.HasValue() {
		list.Append(p.Selector.Unwrap().Lisp())
	}
	//
	return list
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

// NewDefComputedColumn constructs a new column declaration for a computed
// column.  Such a column cannot be finalised yet, since its type and multiplier
// remains to be determined, etc.
func NewDefComputedColumn(context file.Path, name file.Path) *DefColumn {
	var padding big.Int
	//
	binding := ColumnBinding{context, name, nil, false, 0, COMPUTED, padding, "hex"}
	//
	return &DefColumn{binding}
}

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (e *DefColumn) Arity() util.Option[uint] {
	return NON_FUNCTION
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

// LengthMultiplier returns the length multiplier of this column (where the
// height of this column is determined as the product of the enclosing module's
// height and this length multiplier).  If this column have not yet been
// finalised, then this will panic.
func (e *DefColumn) LengthMultiplier() uint {
	if !e.binding.IsFinalised() {
		panic("unfinalised column")
	}
	//
	return e.binding.Multiplier
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
	if e.binding.Multiplier != 1 {
		list.Append(sexp.NewSymbol(":multiplier"))
		list.Append(sexp.NewSymbol(fmt.Sprintf("%d", e.binding.Multiplier)))
	}
	//
	if list.Len() == 1 {
		return list.Get(0)
	}
	//
	return list
}

// ============================================================================
// defcompute
// ============================================================================

// DefComputed is an assignment which computes the values for one or more columns
// based (currently) on a chosen internal function.
type DefComputed struct {
	// Columns being assigned by this computation
	Targets []*DefColumn
	// Function being invoked to perform computation
	Function Symbol
	// Source columns as parameters to computation.
	Sources []Symbol
	// Indicates the declaration has been resolved
	Finalised bool
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefComputed) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewArrayIterator(p.Targets)
	return iter.NewCastIterator[*DefColumn, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefComputed) Dependencies() iter.Iterator[Symbol] {
	fn := iter.NewUnitIterator(p.Function)
	sources := iter.NewArrayIterator(p.Sources)
	//
	return fn.Append(sources)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefComputed) Defines(symbol Symbol) bool {
	for _, col := range p.Targets {
		if &col.binding == symbol.Binding() {
			return true
		}
	}
	// Done
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefComputed) IsFinalised() bool {
	return p.Finalised
}

// Finalise this declaration, which means that all source and target expressions
// have been resolved.
func (p *DefComputed) Finalise() {
	p.Finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefComputed) Lisp() sexp.SExp {
	targets := make([]sexp.SExp, len(p.Targets))
	sources := make([]sexp.SExp, len(p.Sources))
	// Targets
	for i, t := range p.Targets {
		targets[i] = t.Lisp()
	}
	// Sources
	for i, t := range p.Sources {
		var sign string
		//
		sources[i] = sexp.NewList([]sexp.SExp{
			sexp.NewSymbol(sign),
			t.Lisp()})
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defcomputed"),
		sexp.NewList(targets),
		sexp.NewSymbol(p.Function.Path().String()),
		sexp.NewList(sources)})
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

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (e *DefConstUnit) Arity() util.Option[uint] {
	return NON_FUNCTION
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
// same height).  Furthermore, within a given module, we require that all
// columns accessed by the constraint have the same length multiplier.
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
	// Perspective identifies the perspective to which this constraint is
	// associated (if any).
	Perspective *PerspectiveName
	// The constraint itself which (when active) should evaluate to zero for the
	// relevant set of rows.
	Constraint Expr
	//
	finalised bool
}

// NewDefConstraint constructs a new (unfinalised) constraint.
func NewDefConstraint(handle string, domain util.Option[int], guard Expr, perspective *PerspectiveName,
	constraint Expr) *DefConstraint {
	return &DefConstraint{handle, domain, guard, perspective, constraint, false}
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
	// Extract perspective (if applicable)
	if p.Perspective != nil {
		deps = append(deps, p.Perspective)
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

// DefInRange restricts all values for a given expression to be within a range
// [0..n) for some bound n.  Any bound is supported, and the system will choose
// the best underlying implementation as needed.
type DefInRange struct {
	// The expression whose values are being constrained to within the given
	// bound.
	Expr Expr
	// Bitwidth determines the bitwidth that this range constraint is enforcing.
	Bitwidth uint
	// Indicates whether or not the expression has been resolved.
	finalised bool
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefInRange) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefInRange) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator[Symbol](p.Expr.Dependencies())
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

// Finalise this declaration, meaning that the expression has been resolved.
func (p *DefInRange) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefInRange) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("definrange"),
		p.Expr.Lisp(),
		sexp.NewSymbol(fmt.Sprintf("u%d", p.Bitwidth)),
	})
}

// ============================================================================
// definterleaved
// ============================================================================

// DefInterleaved generates a new column by interleaving two or more existing
// columns.  For example, say Z interleaves X and Y (in that order) and we have
// a trace X=[1,2], Y=[3,4].  Then, the interleaved column Z has the values
// Z=[1,3,2,4].  All columns must be defined within the same context.  Finally,
// the type of the interleaved column is the widest type of any source columns.
// For example, consider an interleaving of two columns X and Y with types i16
// and i8 respectively.  Then, the type of the resulting column is i16 (as this
// is required to hold an element from any source column).
type DefInterleaved struct {
	// The target column being defined
	Target *DefColumn
	// The source columns used to define the interleaved target column.
	Sources []TypedSymbol
	// Indicates the declaration has been resolved
	Finalised bool
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefInterleaved) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewUnitIterator(p.Target)
	return iter.NewCastIterator[*DefColumn, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefInterleaved) Dependencies() iter.Iterator[Symbol] {
	iterator := iter.NewArrayIterator(p.Sources)
	return iter.NewCastIterator[TypedSymbol, Symbol](iterator)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefInterleaved) Defines(symbol Symbol) bool {
	return &p.Target.binding == symbol.Binding()
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefInterleaved) IsFinalised() bool {
	return p.Finalised
}

// Finalise this declaration, which means that all source and target expressions
// have been resolved.
func (p *DefInterleaved) Finalise() {
	p.Finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefInterleaved) Lisp() sexp.SExp {
	sources := make([]sexp.SExp, len(p.Sources))
	// Sources
	for i, t := range p.Sources {
		sources[i] = t.Lisp()
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("definterleaved"),
		p.Target.Lisp(),
		sexp.NewList(sources),
	})
}

// ============================================================================
// deflookup
// ============================================================================

// DefLookup represents a lookup constraint between a set N of source
// expressions and a set of N target expressions.  The source expressions must
// have a single context (i.e. all be in the same module) and likewise for the
// target expressions (though the source and target contexts can differ).  The
// constraint can be viewed as a "subset constraint".  Let the set of "source
// tuples" be those obtained by evaluating the source expressions over all rows
// in the source context, and likewise the "target tuples" those for the target
// expressions in the target context.  Then the lookup constraint holds if the
// set of source tuples is a subset of the target tuples.  This does not need to
// be a strict subset, so the two sets can be identical.  Furthermore, these are
// not treated as multi-sets, hence the number of occurrences of a given tuple
// is not relevant.
type DefLookup struct {
	// Unique handle given to this constraint.  This is primarily useful for
	// debugging (i.e. so we know which constraint failed, etc).
	Handle string
	// Checked determines whether or not "type checking" is enabled for source /
	// target pairs.
	Checked bool
	// Source selector expressions (nil entries mean no selector for corresponding source).
	SourceSelectors []Expr
	// Source expressions for lookup (i.e. these values must all be contained
	// within the targets).
	Sources [][]Expr
	// Target selector expressions (nil entries mean no selector for corresponding source).
	TargetSelectors []Expr
	// Target expressions for lookup (i.e. these values must contain all of the
	// source values, but may contain more).
	Targets [][]Expr
	// Indicates whether or not target and source expressions have been resolved.
	finalised bool
}

// NewDefLookup creates a new (unfinalised) lookup constraint.
func NewDefLookup(handle string, checked bool, sourceSelectors []Expr, sources [][]Expr,
	targetSelectors []Expr, targets [][]Expr) *DefLookup {
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
	var deps []Symbol
	//
	for i, sources := range p.Sources {
		ith_selector := p.SourceSelectors[i]
		//
		if ith_selector != nil {
			deps = append(deps, ith_selector.Dependencies()...)
		}
		//
		deps = append(deps, DependenciesOfExpressions(sources)...)
	}
	//
	for i, targets := range p.Targets {
		ith_selector := p.TargetSelectors[i]
		//
		if ith_selector != nil {
			deps = append(deps, ith_selector.Dependencies()...)
		}
		//
		deps = append(deps, DependenciesOfExpressions(targets)...)
	}
	// Combine deps
	return iter.NewArrayIterator(deps)
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
// defpermutation
// ============================================================================

// DefPermutation represents a (lexicographically sorted) permutation of a set
// of source columns in a given source context, manifested as an assignment to a
// corresponding set of target columns.  The sort direction for each of the
// source columns can be specified as increasing or decreasing.
type DefPermutation struct {
	Targets []*DefColumn
	Sources []Symbol
	Signs   []bool
}

// NewDefPermutation constructs a new (unfinalised) sorted permutation assignment.
func NewDefPermutation(targets []*DefColumn, sources []Symbol, signs []bool) *DefPermutation {
	return &DefPermutation{targets, sources, signs}
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefPermutation) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewArrayIterator(p.Targets)
	return iter.NewCastIterator[*DefColumn, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefPermutation) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator(p.Sources)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefPermutation) Defines(symbol Symbol) bool {
	for _, col := range p.Targets {
		if &col.binding == symbol.Binding() {
			return true
		}
	}
	// Done
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefPermutation) IsFinalised() bool {
	for _, col := range p.Targets {
		if !col.binding.IsFinalised() {
			return false
		}
	}
	// Done
	return true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefPermutation) Lisp() sexp.SExp {
	targets := make([]sexp.SExp, len(p.Targets))
	sources := make([]sexp.SExp, len(p.Sources))
	// Targets
	for i, t := range p.Targets {
		targets[i] = t.Lisp()
	}
	// Sources
	for i, t := range p.Sources {
		var sign string
		//
		if i >= len(p.Signs) {
			sources[i] = t.Lisp()
			continue
		} else if p.Signs[i] {
			sign = "+"
		} else {
			sign = "-"
		}
		//
		sources[i] = sexp.NewList([]sexp.SExp{
			sexp.NewSymbol(sign),
			t.Lisp()})
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defpermutation"),
		sexp.NewList(targets),
		sexp.NewList(sources)})
}

// ============================================================================
// defperspective
// ============================================================================

// DefPerspective captures the definition of a perspective, its selector and  a
// set of one or more columns being declared within.
type DefPerspective struct {
	// Name of the perspective.
	symbol *PerspectiveName
	// Selector for the perspective.
	Selector Expr
	// Columns defined in this perspective.
	Columns []*DefColumn
}

// NewDefPerspective constructs a new (unfinalised) perspective declaration.
func NewDefPerspective(name *PerspectiveName, selector Expr, columns []*DefColumn) *DefPerspective {
	return &DefPerspective{name, selector, columns}
}

// Name returns the (unqualified) name of this symbol.  For example, "X" for
// a column X defined in a module m1.
func (p *DefPerspective) Name() string {
	return p.symbol.Path().Tail()
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (p *DefPerspective) Path() *file.Path {
	return &p.symbol.path
}

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (p *DefPerspective) Arity() util.Option[uint] {
	return NON_FUNCTION
}

// Finalise this perspective, which indicates the selector expression has been
// finalised.
func (p *DefPerspective) Finalise() {
	p.symbol.binding.Finalise()
}

// Binding returns the allocated binding for this symbol (which may or may not
// be finalised).
func (p *DefPerspective) Binding() Binding {
	return p.symbol.binding
}

// Dependencies needed to signal declaration.
func (p *DefPerspective) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator(p.Selector.Dependencies())
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefPerspective) Definitions() iter.Iterator[SymbolDefinition] {
	iter1 := iter.NewArrayIterator(p.Columns)
	iter2 := iter.NewCastIterator[*DefColumn, SymbolDefinition](iter1)
	iter3 := iter.NewUnitIterator[SymbolDefinition](p)
	// Construct casting iterator
	return iter2.Append(iter3)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefPerspective) Defines(symbol Symbol) bool {
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
func (p *DefPerspective) IsFinalised() bool {
	return p.symbol.binding.IsFinalised()
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefPerspective) Lisp() sexp.SExp {
	columns := make([]sexp.SExp, len(p.Columns))
	//
	for i := range columns {
		columns[i] = p.Columns[i].Lisp()
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defperspective"),
		p.Selector.Lisp(),
		sexp.NewList(columns),
	})
}

// ============================================================================
// defproperty
// ============================================================================

// DefProperty represents an assertion to be used only for debugging / testing /
// verification.  Unlike vanishing constraints, property assertions do not
// represent something that the prover can enforce.  Rather, they represent
// properties which are expected to hold for every valid trace. That is, they
// should be implied by the actual constraints.  Thus, whilst the prover cannot
// enforce such properties, external tools (such as for formal verification) can
// attempt to ensure they do indeed always hold.
type DefProperty struct {
	// Unique handle given to this constraint.  This is primarily useful for
	// debugging (i.e. so we know which constaint failed, etc).
	Handle string
	// Domain of this property which, if empty, indicates the property applies
	// to all rows. Otherwise, a given value indicates a single row on which
	// this property should apply (where negative values are taken from the end,
	// meaning that -1 represents the last row of a given module).
	Domain util.Option[int]
	// The assertion itself which (when active) should evaluate to zero for the
	// relevant set of rows.
	Assertion Expr
	// Indicates whether or not the assertion has been resolved.
	finalised bool
}

// NewDefProperty constructs a new (unfinalised) property assertion.
func NewDefProperty(handle string, domain util.Option[int], assertion Expr) *DefProperty {
	return &DefProperty{handle, domain, assertion, false}
}

// Definitions returns the set of symbols defined by this declaration.  Observe that
// these may not yet have been finalised.
func (p *DefProperty) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Dependencies needed to signal declaration.
func (p *DefProperty) Dependencies() iter.Iterator[Symbol] {
	return iter.NewArrayIterator(p.Assertion.Dependencies())
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefProperty) Defines(symbol Symbol) bool {
	return false
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefProperty) IsFinalised() bool {
	return p.finalised
}

// Finalise this property, meaning that the assertion has been resolved.
func (p *DefProperty) Finalise() {
	p.finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefProperty) Lisp() sexp.SExp {
	var (
		list = sexp.NewList([]sexp.SExp{
			sexp.NewSymbol("defproperty"),
			sexp.NewSymbol(p.Handle)})
		//

	)
	// domain
	if p.Domain.HasValue() {
		var (
			domain    = fmt.Sprintf("{%d}", p.Domain.Unwrap())
			modifiers = sexp.EmptyList()
		)
		//
		modifiers.Append(sexp.NewSymbol(":domain"))
		modifiers.Append(sexp.NewSymbol(domain))
		//
		list.Append(modifiers)
	}
	//
	list.Append(p.Assertion.Lisp())
	//
	return list
}

// ============================================================================
// defpurefun & defun
// ============================================================================

// DefFun represents defines a (possibly pure) "function" (which, in actuality,
// is more like a macro).  Specifically, whenever an invocation of this function
// is encountered we can imagine that, in the final constraint set, the body of
// the function is inlined at the point of the call.  A pure function is not
// permitted to access any columns in scope (i.e. it can only manipulate its
// parameters).  In contrast, an impure function can access those columns
// defined within its enclosing context.
type DefFun struct {
	symbol *FunctionName
	// Parameters
	parameters []*DefParameter
	// Return
	ret Type
}

var _ SymbolDefinition = &DefFun{}

// NewDefFun constructs a new (unfinalised) function declaration.
func NewDefFun(name *FunctionName, parameters []*DefParameter, ret Type) *DefFun {
	return &DefFun{name, parameters, ret}
}

// Arity indicates whether or not this is a function and, if so, what arity
// (i.e. how many arguments) the function has.
func (p *DefFun) Arity() util.Option[uint] {
	return util.Some(uint(len(p.parameters)))
}

// IsPure indicates whether or not this is a pure function.  That is, a function
// which is not permitted to access any columns from the enclosing environment
// (either directly itself, or indirectly via functions it calls).
func (p *DefFun) IsPure() bool {
	return p.symbol.binding.Pure
}

// Parameters returns information about the parameters defined by this
// declaration.
func (p *DefFun) Parameters() []*DefParameter {
	return p.parameters
}

// Return returns the return type of this declaration, which can be nil if no
// return type was given explicitly.
func (p *DefFun) Return() Type {
	return p.ret
}

// Body Access information about the parameters defined by this declaration.
func (p *DefFun) Body() Expr {
	return p.symbol.binding.Body
}

// Binding returns the allocated binding for this symbol (which may or may not
// be finalised).
func (p *DefFun) Binding() Binding {
	return p.symbol.binding
}

// Name returns the (unqualified) name of this symbol.  For example, "X" for
// a column X defined in a module m1.
func (p *DefFun) Name() string {
	return p.symbol.Path().Tail()
}

// Path returns the qualified name (i.e. absolute path) of this symbol.  For
// example, "m1.X" for a column X defined in module m1.
func (p *DefFun) Path() *file.Path {
	return &p.symbol.path
}

// Finalise this declaration
func (p *DefFun) Finalise() {
	p.symbol.binding.Finalise()
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefFun) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewUnitIterator(p.symbol)
	return iter.NewCastIterator[*FunctionName, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefFun) Dependencies() iter.Iterator[Symbol] {
	deps := p.symbol.binding.Body.Dependencies()
	ndeps := make([]Symbol, 0)
	// Filter out all parameters declared in this function, since these are not
	// external dependencies.
	for _, d := range deps {
		n := d.Path()
		if n.IsAbsolute() || d.Arity().HasValue() || n.Depth() > 1 || !p.hasParameter(n.Head()) {
			ndeps = append(ndeps, d)
		}
	}
	// Done
	return iter.NewArrayIterator(ndeps)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefFun) Defines(symbol Symbol) bool {
	return p.symbol.binding == symbol.Binding()
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefFun) IsFinalised() bool {
	return p.symbol.binding.IsFinalised()
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefFun) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defun"),
		sexp.NewSymbol(p.symbol.path.Tail()),
		sexp.NewSymbol("..."), // todo
	})
}

// hasParameter checks whether this function has a parameter with the given
// name, or not.
func (p *DefFun) hasParameter(name string) bool {
	for _, v := range p.parameters {
		if v.Binding.Name == name {
			return true
		}
	}
	//
	return false
}

// DefParameter packages together those piece relevant to declaring an individual
// parameter, such its name and type.
type DefParameter struct {
	Binding LocalVariableBinding
}

// NewDefParameter constructs a new parameter declaration.
func NewDefParameter(name string, datatype Type) *DefParameter {
	binding := NewLocalVariableBinding(name, datatype)
	return &DefParameter{binding}
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefParameter) Lisp() sexp.SExp {
	return sexp.NewSymbol(p.Binding.Name)
}

// ============================================================================
// defsorted
// ============================================================================

// DefSorted ensures that a given set of columns are lexicographically sorted.
// The sort direction for each of the source columns can be specified as
// increasing or decreasing.
type DefSorted struct {
	// Unique handle given to this constraint.  This is primarily useful for
	// debugging (i.e. so we know which constraint failed, etc).
	Handle string
	// Optional selector expression which determines when a sorted constraint is active.
	Selector util.Option[Expr]
	// Source expressions for lookup (i.e. these values must all be contained
	// within the targets).
	Sources []Expr
	// Sorting signs
	Signs []bool
	// Indicates whether sorting constraint is strict (or not).
	Strict bool
	// Indicates whether or not source expressions have been resolved.
	finalised bool
}

// NewDefSorted constructs a new (unfinalised) sorted constraint which can
// (optionally) be controlled by a given selector expression, and may be strict
// or non-strict.  Observe that, for strict sorting, a selector is always needed
// (i.e. because within padding we cannot guarantee strictness).
func NewDefSorted(handle string, selector util.Option[Expr], sources []Expr, signs []bool, strict bool) *DefSorted {
	return &DefSorted{handle, selector, sources, signs, strict, false}
}

// Dependencies needed to signal declaration.
func (p *DefSorted) Dependencies() iter.Iterator[Symbol] {
	sourceDeps := DependenciesOfExpressions(p.Sources)
	// Combine deps
	return iter.NewArrayIterator(sourceDeps)
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefSorted) IsFinalised() bool {
	return p.finalised
}

// Finalise this declaration, which means that all source and target expressions
// have been resolved.
func (p *DefSorted) Finalise() {
	p.finalised = true
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefSorted) Defines(symbol Symbol) bool {
	return false
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefSorted) Definitions() iter.Iterator[SymbolDefinition] {
	return iter.NewArrayIterator[SymbolDefinition](nil)
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefSorted) Lisp() sexp.SExp {
	sources := make([]sexp.SExp, len(p.Sources))
	// Sources
	for i, t := range p.Sources {
		var sign string
		if p.Signs[i] {
			sign = "+"
		} else {
			sign = "-"
		}
		//
		sources[i] = sexp.NewList([]sexp.SExp{
			sexp.NewSymbol(sign),
			t.Lisp()})
	}
	//
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defsorted"),
		sexp.NewList(sources)})
}

// ============================================================================
// defcomputedcolumn
// ============================================================================

// DefComputedColumn is an assignment which computes the values for one column based on a chosen internal function.
type DefComputedColumn struct {
	// Column being assigned by this computation.
	Target *DefColumn
	// The formula to get the target column from the source columns.
	Computation Expr
	// Indicates the computation has been resolved
	Finalised bool
}

// Definitions returns the set of symbols defined by this declaration.  Observe
// that these may not yet have been finalised.
func (p *DefComputedColumn) Definitions() iter.Iterator[SymbolDefinition] {
	iterator := iter.NewArrayIterator([]*DefColumn{p.Target})
	return iter.NewCastIterator[*DefColumn, SymbolDefinition](iterator)
}

// Dependencies needed to signal declaration.
func (p *DefComputedColumn) Dependencies() iter.Iterator[Symbol] {
	var deps []Symbol
	// Extract bodies dependencies
	deps = append(deps, p.Computation.Dependencies()...)
	// Done
	return iter.NewArrayIterator[Symbol](deps)
}

// Defines checks whether this declaration defines the given symbol.  The symbol
// in question needs to have been resolved already for this to make sense.
func (p *DefComputedColumn) Defines(symbol Symbol) bool {
	return &p.Target.binding == symbol.Binding()
}

// IsFinalised checks whether this declaration has already been finalised.  If
// so, then we don't need to finalise it again.
func (p *DefComputedColumn) IsFinalised() bool {
	return p.Finalised && p.Target.binding.IsFinalised()
}

// Finalise this computed column, which means the target binding and the
// comptuation itself have been finalised.
func (p *DefComputedColumn) Finalise() {
	p.Finalised = true
}

// Lisp converts this node into its lisp representation.  This is primarily used
// for debugging purposes.
func (p *DefComputedColumn) Lisp() sexp.SExp {
	return sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("defcomputedcolumn"),
		p.Target.Lisp(),
		p.Computation.Lisp()})
}
