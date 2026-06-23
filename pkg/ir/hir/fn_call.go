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
package hir

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FunctionCall represents a special kind of lookup constraint which triggers
// trace propagation.
type FunctionCall struct {
	Handle         string
	Callee, Caller module.Id
	Returns        []Term
	Arguments      []Term
	Selector       util.Option[LogicalTerm]
}

// Consistent applies a number of internal consistency checks.  Whilst not
// strictly necessary, these can highlight otherwise hidden problems as an aid
// to debugging.
func (p FunctionCall) Consistent(schema schema.AnySchema[word.BigEndian]) []error {
	var (
		errors []error
		nargs  = uint(len(p.Arguments))
		nrets  = uint(len(p.Returns))
		n      = nargs + nrets
		mod    = schema.Module(p.Callee)
	)
	//
	if mod.Width() < n {
		errors = append(errors,
			fmt.Errorf("incorrect number of arguments / returns (%d vs %d)", n, mod.Width()))
	} else {
		//
		for i := range n {
			var (
				id  = register.NewId(i)
				reg = mod.Register(id)
			)
			if i < nargs && !reg.IsInput() {
				errors = append(errors,
					fmt.Errorf("inconsistent number of arguments (%d vs %d)", nargs, i))
				//
				break
			} else if i >= nargs && !reg.IsOutput() {
				errors = append(errors,
					fmt.Errorf("inconsistent number of returns (%d vs %d)", nrets, i-nargs))
				//
				break
			}
		}
	}
	//
	return errors
}

// Name returns a unique name for a given constraint.  This is useful
// purely for identifying constraints in reports, etc.
func (p FunctionCall) Name() string {
	return p.Handle
}

// Contexts returns the evaluation contexts (i.e. enclosing module + length
// multiplier) for this constraint.  Most constraints have only a single
// evaluation context, though some (e.g. lookups) have more.  Note that all
// constraints have at least one context (which we can call the "primary"
// context).
func (p FunctionCall) Contexts() []schema.ModuleId {
	return []schema.ModuleId{
		p.Callee, p.Caller,
	}
}

// Complexity implementation for constraint interface
func (p FunctionCall) Complexity() uint {
	return 0
}

// Bounds determines the well-definedness bounds for this constraint for both
// the negative (left) or positive (right) directions.  For example, consider an
// expression such as "(shift X -1)".  This is technically undefined for the
// first row of any trace and, by association, any constraint evaluating this
// expression on that first row is also undefined (and hence must pass).
//
//nolint:revive
func (p FunctionCall) Bounds(module uint) util.Bounds {
	var bound util.Bounds
	//
	if module == p.Caller {
		// Include bounds for arguments
		for _, e := range p.Arguments {
			eth := e.Bounds()
			bound.Union(&eth)
		}
		// Include bounds for returns
		for _, e := range p.Returns {
			eth := e.Bounds()
			bound.Union(&eth)
		}
		// Bound selector (if applicable)
		if p.Selector.HasValue() {
			eth := p.Selector.Unwrap().Bounds()
			bound.Union(&eth)
		}
	}
	//
	return bound
}

// Accepts checks whether a lookup constraint into the target columns holds for
// all rows of the source columns.
//
//nolint:revive
func (p FunctionCall) Accepts(_ trace.Trace[word.BigEndian], _ schema.AnySchema[word.BigEndian],
) (bit.Set, schema.Failure) {
	// Currently this is unreachable.
	panic("unreachable")
}

// Lisp converts this schema element into a simple S-Expression, for example
// so it can be printed.
//
//nolint:revive
func (p FunctionCall) Lisp(mapping schema.AnySchema[word.BigEndian]) sexp.SExp {
	var (
		module = mapping.Module(p.Caller)
		callee = mapping.Module(p.Callee)
		args   = sexp.EmptyList()
		rets   = sexp.EmptyList()
	)
	//
	// Iterate arguments
	for _, ith := range p.Arguments {
		args.Append(ith.Lisp(true, module))
	}
	// Iterate returns
	for _, ith := range p.Returns {
		rets.Append(ith.Lisp(true, module))
	}
	// Done
	list := sexp.NewList([]sexp.SExp{
		sexp.NewSymbol("call"),
		rets,
		sexp.NewSymbol(callee.Name().Name),
		args,
	})
	//
	if p.Selector.HasValue() {
		list.Append(p.Selector.Unwrap().Lisp(true, module))
	}
	//
	return list
}

// Substitute any matchined labelled constants within this constraint
func (p FunctionCall) Substitute(mapping map[string]word.BigEndian) {
	//nolint
	for _, ith := range p.Returns {
		ith.Substitute(mapping)
	}

	for _, ith := range p.Arguments {
		ith.Substitute(mapping)
	}
	// Substitute through selector (if applicable)
	if p.Selector.HasValue() {
		p.Selector.Unwrap().Substitute(mapping)
	}
}
