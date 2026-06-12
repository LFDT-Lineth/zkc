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

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/parser"
)

// Link a set of one or more source files together to produce a complete program
// (or one or more errors).  Linking is the process of resolving external
// identifiers used within a source file, or generating errors when this fails.
// For example, if a function in one source file calls another function in a
// different source file, then this linkage needs to be resolved (i.e. checked).
// This can fail for various reasons: for example, if no function of the given
// name can be found in any source file; or, if a function with the correct name
// but incorrect arity (i.e. number of parameters/returns) is found.
func Link(files ...parser.UnlinkedSourceFile) (ast.Program, source.Maps[any], []source.SyntaxError) {
	var (
		program ast.Program
		linker  = NewLinker()
		errors  []source.SyntaxError
	)
	// Construct bus and source mappings
	for _, item := range files {
		linker.Join(item.SourceMap)
		//
		for _, declaration := range item.Components {
			// Check whether component of same name already exists.
			if linker.Exists(declaration.Name()) {
				// Indicates component of same name already exists.
				errors = append(errors, *linker.srcmap.SyntaxError(declaration, "duplicate declaration"))
			} else {
				linker.Register(declaration)
			}
		}
	}
	// Link all assembly items
	var linkErrs []source.SyntaxError

	program, linkErrs = linker.Link()
	errors = append(errors, linkErrs...)
	//
	return program, linker.srcmap, errors
}

// Linker packages together the various bits of information required for linking
// the assembly files.
type Linker struct {
	busmap     map[string]symbol.Resolved
	components []decl.Unresolved
	srcmap     source.Maps[any]
	names      map[string]bool
}

// NewLinker constructs a new linker
func NewLinker() *Linker {
	return &Linker{
		srcmap:     *source.NewSourceMaps[any](),
		busmap:     make(map[string]symbol.Resolved),
		components: nil,
		names:      make(map[string]bool),
	}
}

// Exists checks whether or not a component of the given name already exists.
func (p *Linker) Exists(name string) bool {
	_, ok := p.names[name]
	//
	return ok
}

// Join a source map into this linker
func (p *Linker) Join(srcmap source.Map[any]) {
	p.srcmap.Join(&srcmap)
}

// Register a new components with this linker.
func (p *Linker) Register(component decl.Unresolved) {
	// Ignore any anonymous components (i.e. includes)
	if component.Name() != "" {
		// First, record name
		p.names[component.Name()] = true
		// Second, act on component type
		switch c := component.(type) {
		case decl.Unresolved:
			// Allocate bus entry
			p.busmap[c.Name()] = symbol.Resolved{Index: uint(len(p.busmap))}
			//
			p.components = append(p.components, c)
		default:
			// Should be unreachable
			panic(fmt.Sprintf("unknown component %s", component.Name()))
		}
	}
}

// Link all components register with this linker
func (p *Linker) Link() (ast.Program, []source.SyntaxError) {
	var (
		errors []source.SyntaxError
		decls  []decl.Resolved
	)
	//
	for index := range p.components {
		decl, errs := p.linkDeclaration(uint(index))
		decls = append(decls, decl)
		errors = append(errors, errs...)

		p.srcmap.Copy(p.components[index], decl)
	}
	//
	return ast.NewProgram(decls, p.srcmap), errors
}

// Link all external accessed used within this function to their intended
// targets.  This means, for every bus used locally, settings the global bus
// identifier and also allocated registers for the address/data lines.
func (p *Linker) linkDeclaration(index uint) (decl.Resolved, []source.SyntaxError) {
	switch d := p.components[index].(type) {
	case *decl.UnresolvedConstant:
		return p.linkConstant(*d)
	case *decl.UnresolvedFunction:
		return p.linkFunction(*d)
	case *decl.UnresolvedInclude:
		return decl.NewInclude[symbol.Resolved](d.Pattern()), nil
	case *decl.UnresolvedMemory:
		address, errs1 := p.linkVariableDeclarations(d.Address)
		data, errs2 := p.linkVariableDeclarations(d.Data)

		var (
			contents []expr.Resolved
			errs3    []source.SyntaxError
		)
		if d.Contents != nil {
			contents, errs3 = p.linkExprs(d.Contents...)
		}

		resolved := decl.NewMemory[symbol.Resolved](d.Name(), d.Kind, address, data, contents)
		resolved.SetAnnotations(d.Annotations())

		return resolved, append(append(errs1, errs2...), errs3...)
	case *decl.UnresolvedTypeAlias:
		datatype, errs := p.linkType(d.DataType)
		//
		return decl.NewTypeAlias[symbol.Resolved](d.Name(), datatype), errs
	default:
		panic("unknown declaration")
	}
}

func (p *Linker) linkConstant(fn decl.UnresolvedConstant) (decl.Resolved, []source.SyntaxError) {
	expr, errs1 := p.linkExpr(fn.ConstExpr)
	datatype, errs2 := p.linkType(fn.DataType)
	//
	return decl.NewConstant[symbol.Resolved](fn.Name(), datatype, expr), append(errs1, errs2...)
}

func (p *Linker) linkFunction(fn decl.UnresolvedFunction) (decl.Resolved, []source.SyntaxError) {
	var (
		effects = make([]*symbol.Resolved, len(fn.Effects))
		codes   = make([]stmt.Resolved, len(fn.Code))
		errs1   []source.SyntaxError
	)
	// link effects
	for i, e := range fn.Effects {
		var es []source.SyntaxError
		//
		effects[i], es = p.linkEffect(e)
		//
		errs1 = append(errs1, es...)
	}
	// link code
	for i, c := range fn.Code {
		var es []source.SyntaxError
		//
		codes[i], es = p.linkStatement(c)
		//
		errs1 = append(errs1, es...)
	}
	//
	vars, errs2 := p.linkVariableDeclarations(fn.Variables)
	//
	resolved := decl.NewFunction(fn.Name(), effects, vars, codes)
	resolved.SetAnnotations(fn.Annotations())
	//
	return resolved, append(errs1, errs2...)
}

func (p *Linker) linkEffect(effect *symbol.Unresolved,
) (*symbol.Resolved, []source.SyntaxError) {
	// resolve this effect
	ne, err := p.resolve(*effect, effect)
	// take its address
	neffect := &ne
	// copy over source map info
	p.srcmap.Copy(effect, neffect)
	//
	return neffect, err
}

func (p *Linker) linkVariableDeclarations(decls []variable.UnresolvedDescriptor,
) ([]variable.ResolvedDescriptor, []source.SyntaxError) {
	var (
		ndecls = make([]variable.ResolvedDescriptor, len(decls))
		errors []source.SyntaxError
	)
	//
	for i, d := range decls {
		var datatype, errs = p.linkType(d.DataType)
		//
		ndecls[i] = variable.New(d.Kind, d.Name, datatype)
		//
		errors = append(errors, errs...)
	}
	//
	return ndecls, errors
}

func (p *Linker) linkStatement(s stmt.Unresolved) (stmt.Resolved, []source.SyntaxError) {
	var (
		ninsn  stmt.Resolved
		errors []source.SyntaxError
	)
	//
	switch s := s.(type) {
	case *stmt.Assign[symbol.Unresolved]:
		// Link the left-hand side
		lhs, errs1 := p.linkLVals(s.Targets)
		// Link the right-hand side
		rhs, errs2 := p.linkExpr(s.Source)
		//
		ninsn = &stmt.Assign[symbol.Resolved]{Targets: lhs, Source: rhs}
		//
		errors = append(errs1, errs2...)
	case *stmt.Break[symbol.Unresolved]:
		ninsn = &stmt.Break[symbol.Resolved]{}
	case *stmt.Continue[symbol.Unresolved]:
		ninsn = &stmt.Continue[symbol.Resolved]{}
	case *stmt.Cost[symbol.Unresolved]:
		body, errs := p.linkStatement(s.Body)
		ninsn = &stmt.Cost[symbol.Resolved]{Label: s.Label, Body: body}
		errors = errs
	case *stmt.Fail[symbol.Unresolved]:
		var args []expr.Expr[symbol.Resolved]
		//
		args, errors = p.linkExprs(s.Arguments...)
		//
		ninsn = &stmt.Fail[symbol.Resolved]{Chunks: s.Chunks, Arguments: args}
	case *stmt.For[symbol.Unresolved]:
		init, errs1 := p.linkStatement(s.Init)
		cond, errs2 := p.linkExpr(s.Cond)
		post, errs3 := p.linkStatement(s.Post)
		body, errs4 := p.linkStatements(s.Body)
		ninsn = &stmt.For[symbol.Resolved]{Init: init, Cond: cond, Post: post, Body: body}
		//
		errors = append(append(append(errs1, errs2...), errs3...), errs4...)
	case *stmt.IfElse[symbol.Unresolved]:
		cond, errs1 := p.linkExpr(s.Cond)
		trueBranch, errs2 := p.linkStatements(s.TrueBranch)
		falseBranch, errs3 := p.linkStatements(s.FalseBranch)
		ninsn = &stmt.IfElse[symbol.Resolved]{Cond: cond, TrueBranch: trueBranch, FalseBranch: falseBranch}
		//
		errors = append(append(errs1, errs2...), errs3...)
	case *stmt.Switch[symbol.Unresolved]:
		discriminant, errsDisc := p.linkExpr(s.Discriminant)

		var (
			branches  []stmt.SwitchBranch[symbol.Resolved]
			errsCases []source.SyntaxError
			errsBody  []source.SyntaxError
		)

		for _, branch := range s.Branches {
			var (
				labels []expr.Expr[symbol.Resolved]
				body   []stmt.Stmt[symbol.Resolved]
			)

			for _, caseConstant := range branch.Labels {
				exprCase, errCase := p.linkExpr(caseConstant)
				errsCases = append(errsCases, errCase...)
				labels = append(labels, exprCase)
			}

			for _, statement := range branch.Body {
				exprBody, errBody := p.linkStatement(statement)
				errsBody = append(errsBody, errBody...)
				body = append(body, exprBody)
			}

			branches = append(branches, stmt.SwitchBranch[symbol.Resolved]{
				IsDefault: branch.IsDefault,
				Labels:    labels,
				Body:      body,
			})
		}

		errors = append(errors, errsDisc...)
		errors = append(errors, errsCases...)
		errors = append(errors, errsBody...)
		ninsn = &stmt.Switch[symbol.Resolved]{
			Discriminant: discriminant,
			Branches:     branches,
		}
	case *stmt.Printf[symbol.Unresolved]:
		var args []expr.Expr[symbol.Resolved]
		//
		args, errors = p.linkExprs(s.Arguments...)
		//
		ninsn = &stmt.Printf[symbol.Resolved]{Chunks: s.Chunks, Arguments: args}
	case *stmt.Return[symbol.Unresolved]:
		ninsn = &stmt.Return[symbol.Resolved]{}
	case *stmt.VarDecl[symbol.Unresolved]:
		if s.Init.IsEmpty() {
			ninsn = &stmt.VarDecl[symbol.Resolved]{
				Variables: s.Variables,
				Init:      util.None[expr.Resolved](),
			}
		} else {
			rhs, errs := p.linkExpr(s.Init.Unwrap())
			ninsn = &stmt.VarDecl[symbol.Resolved]{
				Variables: s.Variables,
				Init:      util.Some[expr.Resolved](rhs),
			}
			errors = errs
		}
	case *stmt.While[symbol.Unresolved]:
		cond, errs1 := p.linkExpr(s.Cond)
		body, errs2 := p.linkStatements(s.Body)
		ninsn = &stmt.While[symbol.Resolved]{Cond: cond, Body: body}
		//
		errors = append(errs1, errs2...)
	default:
		return nil, p.srcmap.SyntaxErrors(s, "invalid statement")
	}
	//
	if ninsn != nil {
		p.srcmap.Copy(s, ninsn)
	}
	//
	return ninsn, errors
}

func (p *Linker) linkStatements(stmts []stmt.Unresolved) ([]stmt.Resolved, []source.SyntaxError) {
	var (
		result = make([]stmt.Resolved, len(stmts))
		errors []source.SyntaxError
	)
	//
	for i, insn := range stmts {
		var errs []source.SyntaxError

		result[i], errs = p.linkStatement(insn)
		errors = append(errors, errs...)
	}

	return result, errors
}

func (p *Linker) linkLVals(lvals []lval.Unresolved) ([]lval.Resolved, []source.SyntaxError) {
	var (
		llvals = make([]lval.Resolved, len(lvals))
		errors []source.SyntaxError
	)
	//
	for i, lval := range lvals {
		var errs []source.SyntaxError
		//
		llvals[i], errs = p.linkLVal(lval)
		//
		errors = append(errors, errs...)
	}
	//
	return llvals, errors
}

func (p *Linker) linkLVal(lv lval.Unresolved) (lval.Resolved, []source.SyntaxError) {
	var (
		nlval lval.Resolved
		errs  []source.SyntaxError
	)
	//
	switch lv := lv.(type) {
	case *lval.Variable[symbol.Unresolved]:
		nlval = lval.NewVariable[symbol.Resolved](lv.Ids...)
	case *lval.Array[symbol.Unresolved]:
		index, errs1 := p.linkExpr(lv.Arg)
		nlval = lval.NewArray[symbol.Resolved](lv.Id, index)
		//
		errs = append(errs, errs1...)
	case *lval.MemAccess[symbol.Unresolved]:
		// resolve symbols in memory name
		name, errs1 := p.resolve(lv.Name, lv)
		// resolve symbols in index expression
		index, errs2 := p.linkExprs(lv.Args...)
		//
		nlval = lval.NewMemAccess(name, index)
		//
		errs = append(errs1, errs2...)
	default:
		return nil, p.srcmap.SyntaxErrors(lv, "unknown lval encountered")
	}
	//
	if nlval != nil {
		p.srcmap.Copy(lv, nlval)
	}
	//
	return nlval, errs
}

func (p *Linker) linkExpr(e expr.Unresolved) (expr.Resolved, []source.SyntaxError) {
	var (
		arg    expr.Resolved
		args   []expr.Resolved
		errors []source.SyntaxError
		nexpr  expr.Resolved
	)
	//
	switch e := e.(type) {
	case *expr.Cmp[symbol.Unresolved]:
		lhs, lerrs := p.linkExpr(e.Left)
		rhs, rerrs := p.linkExpr(e.Right)
		nexpr = expr.NewCmp(e.Operator, lhs, rhs)

		errors = append(lerrs, rerrs...)
	case *expr.Add[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewAdd[symbol.Resolved](args...)
	case *expr.BitwiseAnd[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewBitwiseAnd[symbol.Resolved](args...)
	case *expr.Const[symbol.Unresolved]:
		nexpr = expr.NewUntypedConstant[symbol.Resolved](*e.Constant(), e.Base())
	case *expr.ExternAccess[symbol.Unresolved]:
		// resolve arguments
		args, errors = p.linkExprs(e.Args...)
		// attempt to resolve this non-local access
		sym, errs := p.resolve(e.Name, e)
		// combine errors
		errors = append(errors, errs...)
		//
		nexpr = expr.NewExternAccess(sym, args...)
	case *expr.Mul[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewMul[symbol.Resolved](args...)
	case *expr.Cast[symbol.Unresolved]:
		var castType data.ResolvedType
		//
		arg, errors = p.linkExpr(e.Expr)
		//
		if len(errors) == 0 {
			castType, errors = p.linkType(e.CastType)
		}
		//
		nexpr = expr.NewCast[symbol.Resolved](arg, castType)
	case *expr.Concat[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewConcat[symbol.Resolved](args...)
	case *expr.BitwiseNot[symbol.Unresolved]:
		arg, errors = p.linkExpr(e.Expr)
		nexpr = expr.NewBitwiseNot[symbol.Resolved](arg)
	case *expr.BitwiseOr[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewBitwiseOr[symbol.Resolved](args...)
	case *expr.Shl[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewShl[symbol.Resolved](args...)
	case *expr.Shr[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewShr[symbol.Resolved](args...)
	case *expr.LocalAccess[symbol.Unresolved]:
		nexpr = expr.NewLocalAccess[symbol.Resolved](e.Variable)
	case *expr.LogicalAnd[symbol.Unresolved]:
		args, errs := p.linkExprs(e.Exprs...)
		nexpr = expr.NewLogicalAnd[symbol.Resolved](args...)
		errors = errs
	case *expr.LogicalOr[symbol.Unresolved]:
		args, errs := p.linkExprs(e.Exprs...)
		nexpr = expr.NewLogicalOr[symbol.Resolved](args...)
		errors = errs
	case *expr.LogicalNot[symbol.Unresolved]:
		inner, errs := p.linkExpr(e.Expr)
		nexpr = expr.NewLogicalNot[symbol.Resolved](inner)
		errors = errs
	case *expr.ArrayAccess[symbol.Unresolved]:
		// resolve arguments
		arg, errors = p.linkExpr(e.Arg)
		nexpr = expr.NewArrayAccess[symbol.Resolved](e.Id, arg)
	case *expr.Div[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewDiv[symbol.Resolved](args...)
	case *expr.Rem[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewRem[symbol.Resolved](args...)
	case *expr.Sub[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewSub[symbol.Resolved](args...)
	case *expr.Xor[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewXor[symbol.Resolved](args...)

	case *expr.Ternary[symbol.Unresolved]:
		cond, cerrs := p.linkExpr(e.Cond)
		ifTrue, terrs := p.linkExpr(e.IfTrue)
		ifFalse, ferrs := p.linkExpr(e.IfFalse)
		nexpr = expr.NewTernary[symbol.Resolved](cond, ifTrue, ifFalse)

		errors = append(append(append(errors, cerrs...), terrs...), ferrs...)

	case *expr.TupleInitialiser[symbol.Unresolved]:
		args, errors = p.linkExprs(e.Exprs...)
		nexpr = expr.NewTupleInitialiser[symbol.Resolved](args...)
	default:
		return nil, p.srcmap.SyntaxErrors(e, "invalid expression")
	}
	//
	if nexpr != nil {
		p.srcmap.Copy(e, nexpr)
	}
	//
	return nexpr, errors
}

func (p *Linker) linkExprs(exprs ...expr.Unresolved) ([]expr.Resolved, []source.SyntaxError) {
	var (
		nexprs = make([]expr.Resolved, len(exprs))
		errors []source.SyntaxError
	)
	//
	for i, e := range exprs {
		ne, errs := p.linkExpr(e)
		nexprs[i] = ne
		//
		errors = append(errors, errs...)
	}
	//
	return nexprs, errors
}

func (p *Linker) linkType(datatype data.UnresolvedType) (data.ResolvedType, []source.SyntaxError) {
	switch t := datatype.(type) {
	case *data.UnsignedInt[symbol.Unresolved]:
		return data.NewUnsignedInt[symbol.Resolved](t.BitWidth(), t.IsOpen()), nil
	case *data.FixedArray[symbol.Unresolved]:
		var (
			// Link data type
			datatype, errors = p.linkType(t.DataType)
			//
			size util.Union[uint, symbol.Resolved]
		)
		// resolve size symbol (if applicable)
		if t.Size.HasFirst() {
			size = util.Union1[uint, symbol.Resolved](t.Size.First())
		} else {
			var sym, errs = p.resolve(t.Size.Second(), t)
			//
			if len(errs) > 0 {
				// include all errors
				return nil, append(errors, errs...)
			}
			//
			size = util.Union2[uint](sym)
		}
		//
		return data.NewFixedArray(datatype, size), errors
	case *data.Alias[symbol.Unresolved]:
		// resolve symbol
		name, err := p.resolve(t.Name, t)
		//
		if err != nil {
			return nil, p.srcmap.SyntaxErrors(datatype, "unknown type alias")
		}

		return data.NewAlias[symbol.Resolved](name), nil
	case *data.FieldElement[symbol.Unresolved]:
		return data.NewFieldElement[symbol.Resolved](), nil
	default:
		return nil, p.srcmap.SyntaxErrors(datatype, "unknown type encountered")
	}
}

// Resolve the symbol referred to by an external access into a resolved symbol,
// or return an error if there is some issue matching the symbol.
func (p *Linker) resolve(name symbol.Unresolved, node any) (symbol.Resolved, []source.SyntaxError) {
	var sym symbol.Resolved
	//
	for i, c := range p.components {
		nIns, _ := c.Arity()
		// first, check whether name matches
		if c.Name() == name.Name {
			// now, check arity
			if nIns > name.Inputs {
				return sym, p.srcmap.SyntaxErrors(node, fmt.Sprintf("insufficient arguments (expected %d)", nIns))
			} else if nIns < name.Inputs && !name.HasAnyArity() {
				return sym, p.srcmap.SyntaxErrors(node, fmt.Sprintf("too many arguments (expected %d)", nIns))
			} else if msg, err := checkSymbolKind(c, name); err {
				return sym, p.srcmap.SyntaxErrors(node, msg)
			}
			// hit
			return symbol.NewResolved(c.Name(), name.Kind, uint(i)), nil
		}
	}
	// fail
	return sym, p.srcmap.SyntaxErrors(node, "unknown symbol")
}

// Attempt to determine whether or not the given symbol kind matches the
// declaration.
func checkSymbolKind(d decl.Unresolved, sym symbol.Unresolved) (msg string, err bool) {
	var nIns, _ = d.Arity()
	//
	switch sym.Kind {
	case symbol.MEMORY_EFFECT:
		if mem, ok := d.(*decl.UnresolvedMemory); ok && mem.IsReadable() && mem.IsWriteable() {
			return "", false
		}
		//
		return "invalid read/write memory", true
	case symbol.READABLE_MEMORY:
		if mem, ok := d.(*decl.UnresolvedMemory); ok && mem.IsReadable() {
			return "", false
		}
		//
		return "invalid memory read", true
	case symbol.WRITEABLE_MEMORY:
		if mem, ok := d.(*decl.UnresolvedMemory); ok && mem.IsWriteable() {
			return "", false
		}
		//
		return "invalid memory write", true
	case symbol.FUNCTION:
	case symbol.CONSTANT:
	case symbol.TYPE_ALIAS:
		if _, ok := d.(*decl.UnresolvedTypeAlias); ok {
			return "", false
		}
		//
		return "invalid type alias", true
	}
	// Final arity check
	if nIns != sym.Inputs {
		return fmt.Sprintf("incorrect number of arguments (expected %d)", nIns), true
	}
	// No mismatch
	return "", false
}
