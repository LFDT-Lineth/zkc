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
package compiler

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
)

// Scope represents a region of code in which an expression can be evaluated.
// The purpose of a scope is to assist with determining what, exactly, a given
// variable used within an expression refers to.  For example, a variable can
// refer to a column, or a parameter, etc.
type Scope interface {
	// Attempt to bind a given symbol within this scope.  If successful, the
	// symbol is then resolved with the appropriate binding.  Return value
	// indicates whether successful or not.
	Bind(ast.Symbol) bool

	// Check whether a given path is within the enclosing module, or not.
	IsWithin(file.Path) bool

	// Check whether the given symbol is in the process of being defined in the
	// enclose scope.  Thus, if we encounter an this symbol within e.g. some
	// expression whilst it is being defined ... we know its a recursive access.
	// Depending on how the symbol is defined this may (or may not) result in an
	// error.
	IsVisible(ast.Symbol) bool
}

// BindingId identifies a binding (e.g. a column or constant) by its
// (unqualified) name within the scope in which it is declared.
type BindingId = string

// =============================================================================
// Module Scope
// =============================================================================

// ModuleScope defines recursive tree of scopes where symbols can be resolved
// and bound.  The primary goal is to handle the various ways in which a
// symbol's qualified name (i.e. path) can be expressed.  For example, a symbol
// can be given an absolute name (which is resolved from the root of the scope
// tree), or it can be relative (in which case it is resolved relative to a
// given module).
type ModuleScope struct {
	// public determines whether or not this module is externally visible
	public bool
	// Selector determining when this module is active.
	selector util.Option[string]
	// Absolute path
	path file.Path
	// Map identifiers to indices within the bindings array.
	ids map[BindingId]uint
	// The set of bindings in the order of declaration.
	bindings []boxedBinding
	// Enclosing scope
	parent *ModuleScope
	// Submodules in a map (for efficient lookup)
	submodmap map[string]*ModuleScope
	// Submodules in the order of declaration (for determinism).
	submodules []*ModuleScope
}

// NewModuleScope constructs an initially empty top-level scope.
func NewModuleScope(public bool) *ModuleScope {
	return &ModuleScope{
		public,
		util.None[string](),
		file.NewAbsolutePath(),
		make(map[BindingId]uint),
		nil,
		nil,
		make(map[string]*ModuleScope),
		nil,
	}
}

// Path returns the absolute path of this module.
func (p *ModuleScope) Path() *file.Path {
	return &p.path
}

// Name returns the name of the given module.
func (p *ModuleScope) Name() string {
	if p.path.Depth() > 0 {
		return p.path.Tail()
	}
	//
	return ""
}

// IsPublic determines whether or not this is an externally visible module.
func (p *ModuleScope) IsPublic() bool {
	return p.public
}

// Virtual identifies whether or not this is a virtual module.
func (p *ModuleScope) Virtual() bool {
	return p.selector.HasValue()
}

// IsWithin checks whether a given path is local to the enclosing module, or not.
func (p *ModuleScope) IsWithin(path file.Path) bool {
	return !path.IsAbsolute() || (p.parent != nil && p.path.PrefixOf(path))
}

// IsVisible implemention for Scope interface.
func (p *ModuleScope) IsVisible(symbol ast.Symbol) bool {
	// Split the two cases: absolute versus relative.
	if symbol.Path().IsAbsolute() && p.parent != nil {
		// Absolute path, and this is not the root scope.  Therefore, simply
		// pass this up to the root scope for further processing.
		return p.parent.IsVisible(symbol)
	}
	// Relative path from this scope, or possibly an absolute path if this is
	// the root scope.
	found, visible := p.isVisibleInner(*symbol.Path())
	// If not found, traverse upwards.
	if !found && p.parent != nil {
		return p.parent.IsVisible(symbol)
	}
	// Given up
	return visible
}

func (p *ModuleScope) isVisibleInner(path file.Path) (found, visible bool) {
	var id = path.Tail()
	//
	if submod, ok := p.submodmap[path.Head()]; ok && path.Depth() > 1 {
		// Indicates the symbol could be within submodule, so go look in there.
		return submod.isVisibleInner(*path.Dehead())
	} else if index, ok := p.ids[id]; ok && path.Depth() == 1 {
		box := p.bindings[index]
		return true, !box.open || box.binding.IsRecursive()
	}
	// give up
	return false, false
}

// IsRoot checks whether or not this is the root of the module tree.
func (p *ModuleScope) IsRoot() bool {
	return p.parent == nil
}

// Children returns the set of submodules defined within this module.
func (p *ModuleScope) Children() []*ModuleScope {
	return p.submodules
}

// Selector gets an MIR unit expression which evaluates to a non-zero value when
// this module is active.  This can be nil if there is no selector (i.e. this is
// a non-virtual module).
func (p *ModuleScope) Selector() util.Option[string] {
	return p.selector
}

// DestructuredColumns returns the set of (destructured) columns defined within
// this module scope.  That is, source-level columns which are broken down into
// their atomic components.
func (p *ModuleScope) DestructuredColumns() []Register {
	var (
		sources []Register
		owner   file.Path = p.Owner().path
	)
	//
	for _, b := range p.bindings {
		if binding, ok := b.binding.(*ast.ColumnBinding); ok {
			cols := p.destructureColumn(binding, owner, binding.Path, binding.DataType)
			sources = append(sources, cols...)
		}
	}
	//
	return sources
}

// DestructuredConstants returns the set of (destructured) constant definitions
// within this module scope.
func (p *ModuleScope) DestructuredConstants() []ast.ConstantBinding {
	var constants []ast.ConstantBinding

	for _, b := range p.bindings {
		if binding, ok := b.binding.(*ast.ConstantBinding); ok {
			constants = append(constants, *binding)
		}
	}

	return constants
}

// Owner returns the enclosing non-virtual module of this module.  Observe
// that, if this is a non-virtual module, then it is returned.
func (p *ModuleScope) Owner() *ModuleScope {
	if p.selector.IsEmpty() {
		return p
	} else if p.parent != nil {
		return p.parent.Owner()
	}
	// Should be unreachable
	panic("invalid module tree")
}

// Declare a new submodule at the given (absolute) path within this tree scope.
// Submodules can be declared as "virtual" which indicates the submodule is
// simply a subset of rows of its enclosing module.  A virtual module is
// indicated by a non-zero selector, which signals when the virtual module is
// active.  This returns true if this succeeds, otherwise returns false (i.e. a
// matching submodule already exists).
func (p *ModuleScope) Declare(submodule string, selector util.Option[string], public bool) bool {
	if _, ok := p.submodmap[submodule]; ok {
		return false
	}
	// Construct suitable child scope
	scope := &ModuleScope{
		public,
		selector,
		*p.path.Extend(submodule),
		make(map[BindingId]uint),
		nil,
		p,
		make(map[string]*ModuleScope),
		nil,
	}
	// Update records
	p.submodmap[submodule] = scope
	p.submodules = append(p.submodules, scope)
	// Done
	return true
}

// Bind looks up a given variable being referenced within a given module.  For a
// root context, this is either a column or a constant declaration.
func (p *ModuleScope) Bind(symbol ast.Symbol) bool {
	// Split the two cases: absolute versus relative.
	if symbol.Path().IsAbsolute() && p.parent != nil {
		// Absolute path, and this is not the root scope.  Therefore, simply
		// pass this up to the root scope for further processing.
		return p.parent.Bind(symbol)
	}
	// Relative path from this scope, or possibly an absolute path if this is
	// the root scope.
	found := p.innerBind(symbol.Path(), symbol)
	// If not found, traverse upwards.
	if !found && p.parent != nil {
		return p.parent.Bind(symbol)
	}
	//
	return found
}

// InnerBind is really a helper which allows us to split out the symbol path
// from the symbol itself.  This then lets us "traverse" the path as we go
// looking through submodules, etc.
func (p *ModuleScope) innerBind(path *file.Path, symbol ast.Symbol) bool {
	// Relative path.  Then, either it refers to something in this scope, or
	// something in a subscope.
	if path.Depth() == 1 {
		// Must be something in this scope,.
		id := symbol.Path().Tail()
		// Look for it.
		if bid, ok := p.ids[id]; ok {
			// Extract binding
			box := p.bindings[bid]
			// Resolve symbol
			return symbol.Resolve(box.binding)
		}
	} else if submod, ok := p.submodmap[path.Head()]; ok {
		// Looks like this could be in the child scope, so continue searching there.
		return submod.innerBind(path.Dehead(), symbol)
	}
	// Otherwise, try traversing upwards.
	return false
}

// Enter returns a given submodule within this module.
func (p *ModuleScope) Enter(submodule string) *ModuleScope {
	if child, ok := p.submodmap[submodule]; ok {
		// Looks like this is in the child scope, so continue searching there.
		return child
	}
	// Should be unreachable.
	panic("unknown submodule")
}

// Define a new symbol within this scope.
func (p *ModuleScope) Define(symbol ast.SymbolDefinition) bool {
	// Sanity checks
	if !symbol.Path().IsAbsolute() {
		// Definitely should be unreachable.
		panic("symbol definition cannot have relative path!")
	} else if !p.path.PrefixOf(*symbol.Path()) {
		// Should be unreachable.
		err := fmt.Sprintf("invalid symbol definition (%s not prefix of %s)", p.path.String(), symbol.Path().String())
		panic(err)
	} else if !symbol.Path().Parent().Equals(p.path) {
		name := symbol.Path().Get(p.path.Depth())
		// Looks like this definition is for a submodule.  Therefore, attempt to
		// find it and then define it there.
		if mod, ok := p.submodmap[name]; ok {
			// Found it, so attempt definition.
			return mod.Define(symbol)
		}
		// Failed
		return false
	}
	// construct binding identifier
	id := symbol.Name()
	// Sanity check not already declared
	if _, ok := p.ids[id]; ok {
		// Yes, already declared.
		return false
	}
	// ast.Symbol not previously declared.
	bid := uint(len(p.bindings))
	p.bindings = append(p.bindings, boxedBinding{false, symbol.Binding()})
	p.ids[id] = bid
	//
	return true
}

// OpenDefinition indicates that the given symbol is in the process of being
// defined.  This allows us to identify recursive uses of the given symbol (i.e.
// which arise during the period in which it being defined).
func (p *ModuleScope) OpenDefinition(symbol ast.SymbolDefinition) {
	p.setDefinition(true, symbol)
}

// CloseDefinition indicates that the given symbol has now been defined.
func (p *ModuleScope) CloseDefinition(symbol ast.SymbolDefinition) {
	p.setDefinition(false, symbol)
}

func (p *ModuleScope) setDefinition(status bool, symbol ast.SymbolDefinition) {
	if !symbol.Path().IsAbsolute() && !p.IsWithin(*symbol.Path()) {
		panic("symbol definition not permitted")
	}
	// construct binding identifier
	id := symbol.Name()
	// Sanity check not already declared
	if index, ok := p.ids[id]; ok {
		p.bindings[index].open = status
		return
	}
	//
	panic(fmt.Sprintf("unknown symbol definition \"%s\"", symbol.Path().String()))
}

// Flatten flattens the tree into a flat array of modules, such that a module
// always comes before its own submodules.
func (p *ModuleScope) Flatten() []*ModuleScope {
	modules := []*ModuleScope{p}
	//
	for _, m := range p.submodules {
		modules = append(modules, m.Flatten()...)
	}
	//
	return modules
}

func (p *ModuleScope) destructureColumn(column *ast.ColumnBinding, ctx file.Path, path file.Path,
	datatype ast.Type) []Register {
	// Check for base base
	if int_t, ok := datatype.(*ast.IntType); ok {
		return p.destructureAtomicColumn(column, ctx, path, int_t.BitWidth())
	} else if arraytype, ok := datatype.(*ast.ArrayType); ok {
		// For now, assume must be an array
		return p.destructureArrayColumn(column, ctx, path, arraytype)
	} else {
		panic(fmt.Sprintf("unknown type encountered: %v", datatype))
	}
}

// Allocate an array type
func (p *ModuleScope) destructureArrayColumn(col *ast.ColumnBinding, ctx file.Path, path file.Path,
	arrtype *ast.ArrayType) []Register {
	//
	var sources []Register
	// Allocate n columns
	for i := arrtype.MinIndex(); i <= arrtype.MaxIndex(); i++ {
		ith_name := fmt.Sprintf("%s_%d", path.Tail(), i)
		ith_path := path.Parent().Extend(ith_name)
		sources = append(sources, p.destructureColumn(col, ctx, *ith_path, arrtype.Element())...)
	}
	//
	return sources
}

// Destructure atomic column
func (p *ModuleScope) destructureAtomicColumn(column *ast.ColumnBinding, ctx file.Path, path file.Path,
	bitwidth uint) []Register {
	// Construct register source.
	source := Register{
		ast.NewContext(ctx.String()),
		path,
		bitwidth,
		column.MustProve,
		column.IsComputed(),
		column.Display,
		nil}
	//
	return []Register{source}
}

// BoxedBinding simply wraps a given binding with a boolean used to indicate
// whether its definition is "open" or not.  This is used to detect recursive
// symbol accesses and (depending on the exact symbol definition) to report
// errors.
type boxedBinding struct {
	open    bool
	binding ast.Binding
}

// =============================================================================
// Local Scope
// =============================================================================

// LocalScope represents a simple implementation of scope used when resolving
// expressions within a constraint, lookup or range declaration.  A local scope
// must have a single context associated with it, and this will be inferred by
// resolving those expressions which must be evaluated within.
type LocalScope struct {
	global bool
	// Determines whether or not this scope is "pure" (i.e. whether or not
	// columns can be accessed, etc).
	pure bool
	// Represents the enclosing scope
	enclosing Scope
	// Context for this scope
	context *ast.Context
}

// NewLocalScope constructs a new local scope within a given enclosing scope.
// A local scope can also be "global" in the sense that accessing symbols from
// other modules is permitted.
func NewLocalScope(enclosing Scope, global bool, pure bool) LocalScope {
	context := ast.VoidContext()
	//
	return LocalScope{global, pure, enclosing, &context}
}

// NestedConstScope creates a nested scope within this local scope which, in
// addition, is always pure.
func (p LocalScope) NestedConstScope() LocalScope {
	return LocalScope{p.global, true, p, p.context}
}

// IsVisible implemention for Scope interface.
func (p LocalScope) IsVisible(symbol ast.Symbol) bool {
	return p.enclosing.IsVisible(symbol)
}

// IsGlobal determines whether symbols can be accessed in modules other than the
// enclosing module.
func (p LocalScope) IsGlobal() bool {
	return p.global
}

// IsPure determines whether or not this scope is pure.  That is, whether or not
// expressions in this scope are permitted to access columns.
func (p LocalScope) IsPure() bool {
	return p.pure
}

// IsWithin checks whether a given path is local to the enclosing module, or not.
func (p LocalScope) IsWithin(path file.Path) bool {
	return p.enclosing.IsWithin(path)
}

// FixContext fixes the context for this scope.  Since every scope requires
// exactly one context, this fails if we fix it to incompatible contexts.
func (p LocalScope) FixContext(context ast.Context) bool {
	// Join contexts together
	*p.context = p.context.Join(context)
	// Check they were compatible
	return !p.context.IsConflicted()
}

// Bind looks up a given variable being referenced either within the enclosing
// scope (module==nil) or within a specified module.
func (p LocalScope) Bind(symbol ast.Symbol) bool {
	return p.enclosing.Bind(symbol)
}
