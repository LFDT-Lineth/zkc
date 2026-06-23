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
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/LFDT-Lineth/zkc/pkg/corset/ast"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// ===================================================================
// Public
// ===================================================================

// ParseSourceFiles parses zero or more source files producing zero or more
// modules.  Observe that, since a given module can be spread over multiple
// files, there can be far few modules created than there are source files. This
// function does more than just parse the individual files, because it
// additional combines all fragments of the same module together into one place.
// Thus, you should never expect to see duplicate module names in the returned
// array.
func ParseSourceFiles(files []source.File, config Config) (ast.Circuit, *source.Maps[ast.Node], []SyntaxError) {
	var (
		circuit ast.Circuit
		// (for now) at most one error per source file is supported.
		errors []SyntaxError
		// Construct an initially empty source map
		srcmaps = source.NewSourceMaps[ast.Node]()
		// Contents map holds the combined fragments of each module.
		contents = make(map[string]ast.Module, 0)
		// Names identifies the names of each unique module.
		names = make([]string, 0)
		// Handles used for detecting duplicate constraint handles
		handles = make(map[string]bool)
	)
	//
	for _, file := range files {
		c, srcmap, errs := parseSourceFile(file, config, handles)
		// Handle errors
		if len(errs) > 0 {
			// Report any errors encountered
			errors = append(errors, errs...)
		} else {
			// Combine source maps
			srcmaps.Join(srcmap)
		}
		// Update top-level declarations
		circuit.Declarations = append(circuit.Declarations, c.Declarations...)
		// Allocate any module fragments
		for _, m := range c.Modules {
			if om, ok := contents[m.Name]; !ok {
				contents[m.Name] = m
				names = append(names, m.Name)
			} else {
				om.Declarations = append(om.Declarations, m.Declarations...)
				//
				contents[m.Name] = om
			}
		}
	}
	// Bring all fragments together
	circuit.Modules = make([]ast.Module, len(names))
	// Sort module names to ensure that compilation is always deterministic.
	sort.Strings(names)
	// Finalise every module
	for i, n := range names {
		// Assume this cannot fail as every module in names has been assigned at
		// least one fragment.
		circuit.Modules[i] = contents[n]
	}
	// Done
	if len(errors) > 0 {
		return circuit, srcmaps, errors
	}
	// no errors
	return circuit, srcmaps, nil
}

func parseSourceFile(srcfile source.File, config Config,
	handles map[string]bool) (ast.Circuit, *source.Map[ast.Node], []SyntaxError) {
	//
	//
	var (
		circuit ast.Circuit
		errors  []SyntaxError
		path    file.Path = file.NewAbsolutePath()
	)
	// Parse bytes into an S-Expression
	terms, srcmap, err := sexp.ParseAll(&srcfile)
	// Check test file parsed ok
	if err != nil {
		return circuit, nil, []SyntaxError{*err}
	}
	// Construct parser for corset syntax
	p := NewParser(srcfile, srcmap, config, handles)
	// Parse whatever is declared at the beginning of the file before the first
	// module declaration.  These declarations form part of the "prelude".
	if circuit.Declarations, terms, errors = p.parseModuleContents(path, terms); len(errors) > 0 {
		return circuit, nil, errors
	}
	// Continue parsing string until nothing remains.
	for len(terms) != 0 {
		var (
			name  string
			decls []ast.Declaration
		)
		// Extract module name
		if name, errors = p.parseModuleStart(terms[0]); len(errors) > 0 {
			return circuit, nil, errors
		}
		// Parse module contents
		path = file.NewAbsolutePath(name)
		if decls, terms, errors = p.parseModuleContents(path, terms[1:]); len(errors) > 0 {
			return circuit, nil, errors
		} else if len(decls) != 0 {
			circuit.Modules = append(circuit.Modules, ast.Module{
				Name:         name,
				Declarations: decls,
			})
		}
	}
	// Done
	return circuit, p.NodeMap(), nil
}

// Parser implements a simple parser for the Corset language.  The parser itself
// is relatively simplistic and simply packages up the relevant lisp constructs
// into their corresponding AST forms.  This can fail in various ways, such as
// e.g. a "defconstraint" not having exactly three arguments, etc.  However, the
// parser does not attempt to perform more complex forms of validation (e.g.
// ensuring that expressions are well-typed, etc) --- that is left up to the
// compiler.
type Parser struct {
	// Handles map used to detect duplicate handles
	handles map[string]bool
	// Translator used for recursive expressions.
	translator *sexp.Translator[ast.Expr]
	// Mapping from constructed S-Expressions to their spans in the original text.
	nodemap *source.Map[ast.Node]
	// configuration options
	config Config
}

// NewParser constructs a new parser using a given mapping from S-Expressions to
// spans in the underlying source file.
func NewParser(srcfile source.File, srcmap *source.Map[sexp.SExp], config Config, handles map[string]bool,
) *Parser {
	//
	p := sexp.NewTranslator[ast.Expr](&srcfile, srcmap)
	// Construct (initially empty) node map
	nodemap := source.NewSourceMap[ast.Node](srcmap.Source())
	// Construct parser
	parser := &Parser{handles, p, nodemap, config}
	// Configure expression translator
	p.AddSymbolRule(constantParserRule)
	p.AddSymbolRule(varAccessParserRule)
	p.AddRecursiveListRule("+", addParserRule)
	p.AddRecursiveListRule("-", subParserRule)
	p.AddRecursiveListRule("*", mulParserRule)
	p.AddRecursiveListRule("~", normParserRule)
	p.AddRecursiveListRule("^", powParserRule)
	p.AddRecursiveListRule("¬", logicalNegationRule)
	p.AddRecursiveListRule("∨", logicalParserRule)
	p.AddRecursiveListRule("∧", logicalParserRule)
	p.AddRecursiveListRule("==", eqParserRule)
	p.AddRecursiveListRule("!=", eqParserRule)
	p.AddRecursiveListRule("::", concatParserRule)
	p.AddRecursiveListRule("begin", beginParserRule)
	p.AddRecursiveListRule("debug", debugParserRule)
	p.AddListRule("for", forParserRule(parser))
	p.AddListRule("let", letParserRule(parser))
	p.AddListRule("reduce", reduceParserRule(parser))
	p.AddListRule("if", ifParserRule(parser))
	p.AddRecursiveListRule("shift", shiftParserRule)
	p.AddDefaultListRule(invokeParserRule(parser))
	p.AddDefaultRecursiveArrayRule(arrayAccessParserRule)
	//
	return parser
}

// NodeMap extract the node map constructed by this parser.  A key task here is
// to copy all mappings from the expression translator, which maintains its own
// map.
func (p *Parser) NodeMap() *source.Map[ast.Node] {
	// Copy all mappings from translator's source map into this map.  A mapping
	// function is required to coerce the types.
	source.JoinMaps(p.nodemap, p.translator.SourceMap(), func(e ast.Expr) ast.Node { return e })
	// Done
	return p.nodemap
}

// Register a source mapping from a given S-Expression to a given target node.
func (p *Parser) mapSourceNode(from sexp.SExp, to ast.Node) {
	span := p.translator.SpanOf(from)
	p.nodemap.Put(to, span)
}

// Extract all declarations associated with a given module and package them up.
func (p *Parser) parseModuleContents(path file.Path, terms []sexp.SExp) ([]ast.Declaration, []sexp.SExp,
	[]SyntaxError) {
	//
	var (
		errors []SyntaxError
		decls  = make([]ast.Declaration, 0)
	)
	//
	for i, s := range terms {
		e, ok := s.(*sexp.List)
		// Check for error
		if !ok {
			err := p.translator.SyntaxError(s, "unexpected or malformed declaration")
			errors = append(errors, *err)
		} else if e.MatchSymbols(2, "module") {
			return decls, terms[i:], errors
		} else if decl, errs := p.parseDeclaration(path, e); len(errs) > 0 {
			errors = append(errors, errs...)
		} else {
			// Continue accumulating declarations for this module.
			decls = append(decls, decl)
		}
	}
	// Sanity check errors
	if len(errors) > 0 {
		return nil, nil, errors
	}
	// End-of-file signals end-of-module.
	return decls, make([]sexp.SExp, 0), nil
}

// Parse a module declaration of the form "(module m1)" which indicates the
// start of module m1.
func (p *Parser) parseModuleStart(s sexp.SExp) (string, []SyntaxError) {
	var (
		name   string
		errors []SyntaxError
	)

	l, ok := s.(*sexp.List)
	// Check for error
	if !ok {
		err := p.translator.SyntaxError(s, "unexpected or malformed declaration")
		return "", []SyntaxError{*err}
	}
	// Sanity check declaration
	if len(l.Elements) != 2 {
		err := p.translator.SyntaxError(l, "malformed module declaration")
		return "", []SyntaxError{*err}
	}
	// Extract column name
	name = l.Elements[1].AsSymbol().Value
	// Done
	return name, errors
}

func (p *Parser) parseDeclaration(module file.Path, s *sexp.List) (ast.Declaration, []SyntaxError) {
	//
	var (
		decl   ast.Declaration
		errors []SyntaxError
	)
	//
	if s.MatchSymbols(1, "defalias") {
		decl, errors = p.parseDefAlias(s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(1, "defcall") {
		decl, errors = p.parseDefCall(false, s.Elements)
	} else if s.Len() == 5 && s.MatchSymbols(1, "defcall") {
		decl, errors = p.parseDefCall(true, s.Elements)
	} else if s.MatchSymbols(1, "defcolumns") {
		decl, errors = p.parseDefColumns(module, s)
	} else if s.Len() == 3 && s.MatchSymbols(1, "defcomputed") {
		decl, errors = p.parseDefComputed(module, s.Elements)
	} else if s.Len() > 1 && s.MatchSymbols(1, "defconst") {
		decl, errors = p.parseDefConst(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(2, "defconstraint") {
		decl, errors = p.parseDefConstraint(module, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "defpurefun") {
		decl, errors = p.parseDefFun(module, true, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "defun") {
		decl, errors = p.parseDefFun(module, false, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "definrange") {
		decl, errors = p.parseDefInRange(s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "definterleaved") {
		decl, errors = p.parseDefInterleaved(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(1, "deflookup") {
		decl, errors = p.parseDefLookup(module, s.Elements)
	} else if (s.Len() == 5 || s.Len() == 6) && s.MatchSymbols(1, "defclookup") {
		decl, errors = p.parseDefConditionalLookup(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(1, "defmlookup") {
		decl, errors = p.parseDefMultiLookup(module, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(2, "defpermutation") {
		decl, errors = p.parseDefPermutation(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(2, "defperspective") {
		decl, errors = p.parseDefPerspective(module, s.Elements)
	} else if 3 <= s.Len() && s.Len() <= 4 && s.MatchSymbols(2, "defproperty") {
		decl, errors = p.parseDefProperty(module, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(2, "defsorted") {
		decl, errors = p.parseDefSorted(module, false, s.Elements)
	} else if 3 <= s.Len() && s.Len() <= 4 && s.MatchSymbols(2, "defstrictsorted") {
		decl, errors = p.parseDefSorted(module, true, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "defcomputedcolumn") {
		decl, errors = p.parseDefComputedColumn(module, s.Elements)
	} else {
		errors = p.translator.SyntaxErrors(s, "malformed declaration")
	}
	// Register node if appropriate
	if decl != nil {
		p.mapSourceNode(s, decl)
	}
	// done
	return decl, errors
}

// Parse an alias declaration
func (p *Parser) parseDefAlias(elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors  []SyntaxError
		aliases []*ast.DefAlias
		names   []ast.Symbol
	)

	for i := 1; i < len(elements); i += 2 {
		// Sanity check first
		if i+1 == len(elements) {
			// Uneven number of constant declarations!
			errors = append(errors, *p.translator.SyntaxError(elements[i], "missing alias definition"))
		} else if !isEitherOrIdentifier(elements[i], false) {
			// ast.Symbol expected!
			errors = append(errors, *p.translator.SyntaxError(elements[i], "invalid alias name"))
		} else if !isEitherOrIdentifier(elements[i+1], false) {
			// ast.Symbol expected!
			errors = append(errors, *p.translator.SyntaxError(elements[i+1], "invalid alias definition"))
		} else {
			alias := ast.NewDefAlias(elements[i].AsSymbol().Value)
			path := file.NewRelativePath(elements[i+1].AsSymbol().Value)
			name := ast.NewUnboundName[ast.Binding](path, ast.NON_FUNCTION)
			//
			p.mapSourceNode(elements[i], alias)
			p.mapSourceNode(elements[i+1], name)
			//
			aliases = append(aliases, alias)
			names = append(names, name)
		}
	}
	// Done
	return ast.NewDefAliases(aliases, names), errors
}

// Parse a column declaration
func (p *Parser) parseDefColumns(module file.Path, l *sexp.List) (ast.Declaration, []SyntaxError) {
	columns := make([]*ast.DefColumn, l.Len()-1)
	// Sanity check declaration
	if len(l.Elements) == 1 {
		err := p.translator.SyntaxError(l, "malformed column declaration")
		return nil, []SyntaxError{*err}
	}
	//
	var errors []SyntaxError
	// Process column declarations one by one.
	for i := 1; i < len(l.Elements); i++ {
		decl, err := p.parseColumnDeclaration(module, module, false, 1, l.Elements[i])
		// Extract column name
		if err != nil {
			errors = append(errors, *err)
		}
		// Assign the declaration
		columns[i-1] = decl
	}
	// Sanity check errors
	if len(errors) > 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefColumns(columns), nil
}

func (p *Parser) parseColumnDeclaration(context file.Path, path file.Path, computed bool, multiplier int,
	e sexp.SExp) (*ast.DefColumn, *SyntaxError) {
	//
	var (
		zero  big.Int
		error *SyntaxError
		// Initial binding with defaults
		binding = ast.ColumnBinding{
			ColumnContext: context,
			Kind:          ast.NOT_COMPUTED,
			Multiplier:    uint(multiplier),
			MustProve:     p.config.EnforceTypes,
			Padding:       zero,
			Display:       "hex",
		}
	)
	// Update computed status
	if computed {
		binding.Kind = ast.COMPUTED
	}
	// Check whether extended declaration or not.
	if l := e.AsList(); l != nil {
		// Check at least the name provided.
		if len(l.Elements) == 0 {
			return nil, p.translator.SyntaxError(l, "empty column declaration")
		} else if !isIdentifier(l.Elements[0]) {
			return nil, p.translator.SyntaxError(l.Elements[0], "invalid column name")
		}
		// Column name is always first
		binding.Path = *path.Extend(l.Elements[0].String(false))
		//	Parse type (if applicable)
		if binding, error = p.parseColumnDeclarationAttributes(e, binding, l.Elements[1:]); error != nil {
			return nil, error
		}
	} else if computed {
		// Only computed columns can be given without attributes.
		binding.Path = *path.Extend(e.String(false))
	} else {
		return nil, p.translator.SyntaxError(e, "column is untyped")
	}
	// Final sanity checks
	if computed && binding.DataType == nil {
		// computed columns initially have multiplier 0 in order to signal that
		// this needs to be subsequently determined from context.
		binding.Multiplier = 0
		binding.DataType = ast.UINT_TYPE
	} else if !binding.DataType.HasUnderlying() {
		return nil, p.translator.SyntaxError(e, "invalid column type")
	}
	//
	def := ast.NewDefColumn(binding)
	// Update source mapping
	p.mapSourceNode(e, def)
	//
	return def, nil
}

func (p *Parser) parseColumnDeclarationAttributes(node sexp.SExp, binding ast.ColumnBinding,
	attrs []sexp.SExp) (ast.ColumnBinding, *SyntaxError) {
	//
	var (
		array_min uint
		array_max uint
		err       *SyntaxError
	)

	for i := 0; i < len(attrs); i++ {
		ith := attrs[i]
		symbol := ith.AsSymbol()
		// Sanity check
		if symbol == nil {
			return binding, p.translator.SyntaxError(ith, "unknown column attribute")
		}
		//
		switch symbol.Value {
		case ":display":
			// skip these for now, as they are only relevant to the inspector.
			if i+1 == len(attrs) {
				return binding, p.translator.SyntaxError(ith, "incomplete display definition")
			} else if attrs[i+1].AsSymbol() == nil {
				return binding, p.translator.SyntaxError(ith, "malformed display definition")
			}
			//
			binding.Display = attrs[i+1].AsSymbol().String(false)
			// Check what display attribute we have
			switch binding.Display {
			case ":dec", ":hex", ":bytes", ":opcode":
				binding.Display = binding.Display[1:]
				// all good
				i = i + 1
			default:
				// not good
				return binding, p.translator.SyntaxError(ith, "unknown display definition")
			}
		case ":array":
			if i+1 == len(attrs) {
				return binding, p.translator.SyntaxError(ith, "missing array dimension")
			} else if array_min, array_max, err = p.parseArrayDimension(attrs[i+1]); err != nil {
				return binding, err
			}
			// skip dimension
			i++
		case ":padding":
			if i+1 == len(attrs) {
				return binding, p.translator.SyntaxError(ith, "missing padding value")
			} else if binding.Padding, err = p.parsePaddingValue(attrs[i+1]); err != nil {
				return binding, err
			}
			// skip dimension
			i++
		case ":fwd":
			switch binding.Kind {
			case ast.NOT_COMPUTED:
				return binding, p.translator.SyntaxError(ith, "input columns cannot be recursive")
			case ast.COMPUTED_BWD:
				return binding, p.translator.SyntaxError(ith, "conflicting direction of recursion")
			default:
				binding.Kind = ast.COMPUTED_FWD
			}
		case ":bwd":
			switch binding.Kind {
			case ast.NOT_COMPUTED:
				return binding, p.translator.SyntaxError(ith, "input columns cannot be recursive")
			case ast.COMPUTED_FWD:
				return binding, p.translator.SyntaxError(ith, "conflicting direction of recursion")
			default:
				binding.Kind = ast.COMPUTED_BWD
			}
		default:
			if binding.DataType, binding.MustProve, err = p.parseType(ith); err != nil {
				return binding, err
			}
		}
	}
	// Done
	if binding.DataType == nil {
		return binding, p.translator.SyntaxError(node, "column is untyped")
	} else if array_max != 0 {
		binding.DataType = ast.NewArrayType(binding.DataType, array_min, array_max)
	}
	// Success!
	return binding, nil
}

func (p *Parser) parsePaddingValue(s sexp.SExp) (big.Int, *SyntaxError) {
	var (
		err     error
		ok      bool
		padding big.Int
	)
	//
	if symbol := s.AsSymbol(); symbol == nil {
		return padding, p.translator.SyntaxError(s, "invalid padding value")
	} else if padding, ok, err = parseConstant(symbol.Value); err != nil {
		return padding, p.translator.SyntaxError(s, err.Error())
	} else if !ok {
		return padding, p.translator.SyntaxError(s, "invalid padding value")
	}
	//
	return padding, nil
}

func (p *Parser) parseArrayDimension(s sexp.SExp) (uint, uint, *SyntaxError) {
	dim := s.AsArray()
	//
	if dim == nil || dim.Get(0).AsSymbol() == nil || dim.Len() != 1 {
		return 0, 0, p.translator.SyntaxError(s, "invalid array dimension")
	} else {
		// Check for interval dimensions
		split := strings.Split(dim.Get(0).AsSymbol().Value, ":")
		//
		if len(split) == 0 || len(split) > 2 {
			return 0, 0, p.translator.SyntaxError(s, "invalid array dimension")
		} else if m, ok_m := strconv.Atoi(split[0]); len(split) == 1 && m >= 0 && ok_m == nil {
			return uint(1), uint(m), nil
		} else if ok_m != nil || m < 0 {
			//unlikely scenarios
		} else if n, ok_n := strconv.Atoi(split[1]); len(split) == 2 && n >= 0 && ok_n == nil {
			return uint(m), uint(n), nil
		}
	}
	//
	return 0, 0, p.translator.SyntaxError(s, "invalid array dimension")
}

// Parse a defcomputed declaration
func (p *Parser) parseDefComputed(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors      []SyntaxError
		sexpTargets *sexp.List = elements[1].AsList()
		sexpSources *sexp.List = elements[2].AsList()
		targets     []*ast.DefColumn
		sources     []ast.Symbol
	)
	// Sanity checks
	if sexpTargets == nil || sexpTargets.Len() == 0 {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "malformed target columns"))
	} else {
		targets = make([]*ast.DefColumn, sexpTargets.Len())
		//
		for i := 0; i < sexpTargets.Len(); i++ {
			var err *SyntaxError
			// Parse target declaration
			if targets[i], err = p.parseColumnDeclaration(module, module, true, 1, sexpTargets.Get(i)); err != nil {
				errors = append(errors, *err)
			}
		}
	}
	//
	if sexpSources == nil || sexpSources.Len() == 0 {
		errors = append(errors, *p.translator.SyntaxError(elements[2], "malformed source invocation"))
	} else {
		sources = make([]ast.Symbol, sexpSources.Len())
		//
		for i := 0; i < sexpSources.Len(); i++ {
			ith := sexpSources.Get(i)
			if symbol := sexpSources.Get(i).AsSymbol(); symbol == nil {
				errors = append(errors, *p.translator.SyntaxError(ith, "malformed symbol or function name"))
			} else {
				// Handle qualified accesses (where permitted)
				path, err := parseQualifiableName(symbol.Value)
				//
				if err != nil {
					errors = append(errors, *p.translator.SyntaxError(ith, "invalid symbol or function name"))
				} else {
					var arity util.Option[uint] = ast.NON_FUNCTION
					// Valid symbol
					if i == 0 {
						arity = util.Some(uint(sexpSources.Len() - 1))
					}
					//
					sources[i] = ast.NewVariableAccess(path, arity, nil)
					// Update source mapping
					p.mapSourceNode(ith, sources[i])
				}
			}
		}
	}
	//
	if len(errors) > 0 {
		return nil, errors
	}
	//
	return &ast.DefComputed{Targets: targets, Function: sources[0], Sources: sources[1:]}, nil
}

// Parse a defcomputedcolumn declaration
func (p *Parser) parseDefComputedColumn(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors []SyntaxError
		target *ast.DefColumn
		err    *SyntaxError
	)

	// Parse target declaration
	columnDeclaration := elements[1].AsList()
	if columnDeclaration == nil {
		err := *p.translator.SyntaxError(elements[1], "computed column is not of the right format")
		errors = append(errors, err)
	}
	//
	if len(errors) != 0 {
		return nil, errors
	} else if target, err = p.parseColumnDeclaration(module, module, true, 1, columnDeclaration); err != nil {
		errors = append(errors, *err)
	}
	// Translate expression
	expr, exprErrors := p.translator.Translate(elements[2])
	errors = append(errors, exprErrors...)

	//
	if len(errors) > 0 {
		return nil, errors
	}
	//
	return &ast.DefComputedColumn{Target: target, Computation: expr}, nil
}

// Parse a constant declaration
func (p *Parser) parseDefConst(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors    []SyntaxError
		constants []*ast.DefConstUnit
	)

	for i := 1; i < len(elements); i += 2 {
		// Sanity check first
		if i+1 == len(elements) {
			// Uneven number of constant declarations!
			errors = append(errors, *p.translator.SyntaxError(elements[i], "missing constant definition"))
		} else {
			// Attempt to parse definition
			constant, errs := p.parseDefConstUnit(module, elements[i], elements[i+1])
			errors = append(errors, errs...)
			constants = append(constants, constant)
		}
	}
	// Done
	return &ast.DefConst{Constants: constants}, errors
}

func (p *Parser) parseDefConstUnit(module file.Path, head sexp.SExp,
	value sexp.SExp) (*ast.DefConstUnit, []SyntaxError) {
	//
	var (
		name     *sexp.Symbol
		datatype ast.Type
		errors   []SyntaxError
		expr     ast.Expr
		extern   bool
	)
	// Parse head
	if name, datatype, extern, errors = p.parseDefConstHead(head); len(errors) > 0 {
		return nil, errors
	} else if expr, errors = p.translator.Translate(value); len(errors) > 0 {
		return nil, errors
	}
	// Looks good
	path := module.Extend(name.Value)
	def := &ast.DefConstUnit{ConstBinding: ast.NewConstantBinding(*path, datatype, expr, extern)}
	// Map to source node
	p.mapSourceNode(value, def)
	// Done
	return def, nil
}

func (p *Parser) parseDefConstHead(head sexp.SExp) (*sexp.Symbol, ast.Type, bool, []SyntaxError) {
	var (
		list     = head.AsList()
		datatype ast.Type
		extern   bool
	)

	// Parse the head
	if isIdentifier(head) {
		// no attributes provided
		return head.AsSymbol(), nil, false, nil
	} else if list == nil {
		return nil, nil, false, p.translator.SyntaxErrors(head, "invalid constant name")
	} else if list.Len() < 2 {
		return nil, nil, false, p.translator.SyntaxErrors(list, "invalid constant declaration")
	} else if !isIdentifier(list.Get(0)) {
		return nil, nil, false, p.translator.SyntaxErrors(list.Get(0), "invalid constant name")
	}
	//
	for i := 1; i < list.Len(); i++ {
		var (
			prove bool
			err   *SyntaxError
		)
		//
		sym := list.Get(i).AsSymbol()
		// Catch error
		if sym == nil {
			return nil, nil, false, p.translator.SyntaxErrors(list.Get(i), "invalid constant attribute")
		}
		// Parse attribute
		switch sym.Value {
		case ":extern":
			extern = true
		default:
			datatype, prove, err = p.parseType(list.Get(i))
			// Handle errors
			if err != nil {
				return nil, nil, false, []SyntaxError{*err}
			} else if prove && !p.config.EnforceTypes {
				return nil, nil, false, p.translator.SyntaxErrors(list, "constants cannot have proven types")
			}
		}
	}
	// Sanity check type
	return list.Get(0).AsSymbol(), datatype, extern, nil
}

// Parse a vanishing declaration
func (p *Parser) parseDefConstraint(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var errors []SyntaxError
	// Initial sanity checks
	if !isIdentifier(elements[1]) {
		return nil, p.translator.SyntaxErrors(elements[1], "invalid constraint handle")
	}
	//
	handle := elements[1].AsSymbol().Value
	// Generate qualified name
	qualifiedHandle := fmt.Sprintf("%s:%s", module.String(), handle)
	// Check for duplicate
	if _, ok := p.handles[qualifiedHandle]; ok {
		return nil, p.translator.SyntaxErrors(elements[1], "duplicate handle")
	} else {
		p.handles[qualifiedHandle] = true
	}
	// Vanishing constraints do not have global scope, hence qualified column
	// accesses are not permitted.
	domain, guard, perspective, errs := p.parseConstraintAttributes(module, elements[2])
	errors = append(errors, errs...)
	// Translate expression
	expr, errs := p.translator.Translate(elements[3])
	errors = append(errors, errs...)
	// Error Check
	if len(errors) > 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefConstraint(handle, domain, guard, perspective, expr), nil
}

// Parse a interleaved declaration
func (p *Parser) parseDefInterleaved(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors      []SyntaxError
		err         *SyntaxError
		sources     []ast.TypedSymbol
		sexpSources = elements[2].AsList()
		target      *ast.DefColumn
	)
	// Sanity check we have sourcers
	if sexpSources == nil {
		return nil, p.translator.SyntaxErrors(elements[2], "malformed source columns")
	}
	// Parse target declaration
	if target, err = p.parseColumnDeclaration(module, module, true, sexpSources.Len(), elements[1]); err != nil {
		errors = append(errors, *err)
	}
	// Check source columns
	sources = make([]ast.TypedSymbol, sexpSources.Len())
	//
	for i := 0; i != sexpSources.Len(); i++ {
		var errs []SyntaxError
		//
		sources[i], errs = p.parseDefInterleavedSource(sexpSources.Get(i))
		errors = append(errors, errs...)
	}
	// Error Check
	if len(errors) != 0 {
		return nil, errors
	}
	// Done
	return &ast.DefInterleaved{Target: target, Sources: sources}, nil
}

func (p *Parser) parseDefInterleavedSource(source sexp.SExp) (ast.TypedSymbol, []SyntaxError) {
	if source.AsSymbol() != nil {
		return p.parseDefInterleavedSourceColumn(source.AsSymbol())
	} else if source.AsArray() != nil {
		return p.parseDefInterleavedSourceArray(source.AsArray())
	}
	//
	return nil, p.translator.SyntaxErrors(source, "malformed source column")
}

func (p *Parser) parseDefInterleavedSourceColumn(source *sexp.Symbol) (ast.TypedSymbol, []SyntaxError) {
	if path, err := parseQualifiableName(source.Value); err != nil {
		return nil, p.translator.SyntaxErrors(source, err.Error())
	} else {
		varAccess := ast.NewVariableAccess(path, ast.NON_FUNCTION, nil)
		p.mapSourceNode(source, varAccess)

		return varAccess, nil
	}
}

func (p *Parser) parseDefInterleavedSourceArray(source *sexp.Array) (ast.TypedSymbol, []SyntaxError) {
	// Parse index
	name := source.Get(0).AsSymbol()
	index, errors := p.translator.Translate(source.Get(1))
	//
	if name == nil {
		errors = p.translator.SyntaxErrors(source, "malformed source column")
	} else if path, err := parseQualifiableName(name.Value); err != nil {
		errors = append(errors, *p.translator.SyntaxError(source, err.Error()))
	} else {
		arrAccess := &ast.ArrayAccess{Name: path, Arg: index, ArrayBinding: nil}
		p.mapSourceNode(source, arrAccess)

		return arrAccess, nil
	}
	//
	return nil, errors
}

func (p *Parser) parseDefCall(hasSelector bool, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors             []SyntaxError
		returns, retErrors = p.parseDefLookupSources("return", elements[1])
		args, argErrors    = p.parseDefLookupSources("argument", elements[3])
		selector           = util.None[ast.Expr]()
	)
	// Sanity check function name
	if !isIdentifier(elements[2]) {
		return nil, p.translator.SyntaxErrors(elements[2], "malformed function name")
	}
	// Extract function name
	fun := elements[2].AsSymbol().Value
	// Combine any and all errors
	errors = append(errors, argErrors...)
	errors = append(errors, retErrors...)
	// Parse selector (if applicable)
	if hasSelector {
		sel, errs := p.translator.Translate(elements[4])
		selector = util.Some(sel)
		//
		errors = append(errors, errs...)
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	//
	return ast.NewDefCall(returns, fun, args, selector), nil
}

// Parse a lookup declaration
func (p *Parser) parseDefLookup(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	// Extract items
	handle, checked, errors := p.parseLookupHandle(module, elements[1])
	targets, tgtErrors := p.parseDefLookupSources("target", elements[2])
	sources, srcErrors := p.parseDefLookupSources("source", elements[3])
	// Combine any and all errors
	errors = append(errors, srcErrors...)
	errors = append(errors, tgtErrors...)
	// Sanity check length of sources / targets
	if len(sources) != len(targets) {
		msg := fmt.Sprintf("differing number of source and target columns (%d v %d)", len(sources), len(targets))
		errors = append(errors, *p.translator.SyntaxError(elements[3], msg))
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	//
	targetSelectors := make([]ast.Expr, len(targets))
	sourceSelectors := make([]ast.Expr, len(sources))
	// Done
	return ast.NewDefLookup(handle,
		checked,
		sourceSelectors, [][]ast.Expr{sources},
		targetSelectors, [][]ast.Expr{targets}), nil
}

// Parse a conditional lookup declaration
func (p *Parser) parseDefConditionalLookup(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	// Extract items
	var (
		targets, sources               []ast.Expr
		targetSelector, sourceSelector ast.Expr
		errs1, errs2, errs3, errs4     []SyntaxError
	)
	//
	handle, checked, errors := p.parseLookupHandle(module, elements[1])
	//
	if len(elements) == 6 {
		targetSelector, errs1 = p.translator.Translate(elements[2])
		targets, errs2 = p.parseDefLookupSources("target", elements[3])
		sourceSelector, errs3 = p.translator.Translate(elements[4])
		sources, errs4 = p.parseDefLookupSources("source", elements[5])
	} else {
		// Assume source selector
		targets, errs1 = p.parseDefLookupSources("target", elements[2])
		sourceSelector, errs2 = p.translator.Translate(elements[3])
		sources, errs3 = p.parseDefLookupSources("source", elements[4])
	}
	// Combine any and all errors
	errors = append(errors, errs1...)
	errors = append(errors, errs2...)
	errors = append(errors, errs3...)
	errors = append(errors, errs4...)
	// Sanity check length of sources / targets
	if len(sources) != len(targets) {
		msg := fmt.Sprintf("differing number of source and target columns (%d v %d)", len(sources), len(targets))
		errors = append(errors, *p.translator.SyntaxError(elements[3], msg))
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefLookup(handle,
		checked,
		[]ast.Expr{sourceSelector},
		[][]ast.Expr{sources},
		[]ast.Expr{targetSelector},
		[][]ast.Expr{targets}), nil
}

func (p *Parser) parseDefMultiLookup(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	// Extract items
	handle, checked, errors := p.parseLookupHandle(module, elements[1])
	m, targets, tgtErrors := p.parseDefLookupMultiSources("target", elements[2])
	n, sources, srcErrors := p.parseDefLookupMultiSources("source", elements[3])
	// Combine any and all errors
	errors = append(errors, srcErrors...)
	errors = append(errors, tgtErrors...)
	// Sanity check length of sources / targets
	if n != m {
		msg := fmt.Sprintf("differing number of source and target columns (%d v %d)", n, m)
		errors = append(errors, *p.translator.SyntaxError(elements[3], msg))
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	//
	targetSelectors := make([]ast.Expr, len(targets))
	sourceSelectors := make([]ast.Expr, len(sources))
	// Done
	return ast.NewDefLookup(handle, checked, sourceSelectors, sources, targetSelectors, targets), nil
}

func (p *Parser) parseLookupHandle(module file.Path, element sexp.SExp) (string, bool, []SyntaxError) {
	var (
		checked = true
		errors  []SyntaxError
		list    = element.AsList()
		handle  string
	)
	// Extract handle from list
	if list != nil && list.Len() > 0 {
		handle, checked, errors = p.parseLookupAttributes(list)
	} else if !isIdentifier(element) {
		return "", checked, p.translator.SyntaxErrors(element, "malformed handle")
	} else {
		handle = element.AsSymbol().Value
	}
	// Generate qualified name
	qualifiedHandle := fmt.Sprintf("%s:%s", module.String(), handle)
	// Check for duplicate handle
	if _, ok := p.handles[qualifiedHandle]; ok {
		return "", checked, p.translator.SyntaxErrors(element, "duplicate handle")
	} else if len(errors) == 0 {
		// Done
		p.handles[qualifiedHandle] = true
	}
	//
	return handle, checked, errors
}

func (p *Parser) parseLookupAttributes(list *sexp.List) (string, bool, []SyntaxError) {
	var (
		checked = true
		handle  string
		errors  []SyntaxError
	)
	// Sanity check handle
	if !isIdentifier(list.Get(0)) {
		errors = p.translator.SyntaxErrors(list.Get(0), "malformed handle")
	} else {
		handle = list.Get(0).AsSymbol().Value
	}
	// Sanity check attributes
	for i := 1; i < list.Len(); i++ {
		if ith := list.Get(i).AsSymbol(); ith != nil {
			switch ith.Value {
			case ":unchecked":
				checked = false
			default:
				errors = p.translator.SyntaxErrors(list.Get(i), "unknown attribute")
			}
		} else {
			errors = p.translator.SyntaxErrors(list.Get(i), "malformed attribute")
		}
	}
	//
	return handle, checked, errors
}

func (p *Parser) parseDefLookupMultiSources(handle string, element sexp.SExp) (int, [][]ast.Expr, []SyntaxError) {
	var (
		sexpTargets = element.AsList()
		errors      []SyntaxError
		width       int
	)
	// Check target expressions
	if sexpTargets == nil {
		return width, nil, p.translator.SyntaxErrors(element, "malformed target columns")
	}
	//
	targets := make([][]ast.Expr, sexpTargets.Len())
	// Translate all target expressions
	for i := range sexpTargets.Len() {
		ith := sexpTargets.Get(i).AsList()
		// Sanity check length
		if ith == nil {
			errors = append(errors, *p.translator.SyntaxError(ith, "malformed columns"))
		} else if i != 0 && ith.Len() != width {
			errors = append(errors, *p.translator.SyntaxError(ith, "incorrect number of columns"))
		} else {
			ith_targets, errs := p.parseDefLookupSources(handle, ith)
			errors = append(errors, errs...)
			targets[i] = ith_targets
			width = ith.Len()
		}
	}
	//
	return width, targets, errors
}

func (p *Parser) parseDefLookupSources(handle string, element sexp.SExp) ([]ast.Expr, []SyntaxError) {
	var (
		sexpSources = element.AsList()
		errors      []SyntaxError
		sources     []ast.Expr
	)
	// Check source Expressions
	if sexpSources == nil {
		msg := fmt.Sprintf("malformed %s columns", handle)
		return nil, p.translator.SyntaxErrors(element, msg)
	}
	//
	if len(errors) == 0 {
		sources = make([]ast.Expr, sexpSources.Len())
		//
		for i := 0; i != sexpSources.Len(); i++ {
			var errs []SyntaxError
			// Translate source expressions
			sources[i], errs = p.translator.Translate(sexpSources.Get(i))
			errors = append(errors, errs...)
		}
	}
	//
	return sources, errors
}

// Parse a permutation declaration
func (p *Parser) parseDefPermutation(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors  []SyntaxError
		sources []ast.Symbol
		signs   []bool
		targets []*ast.DefColumn
	)
	//
	sexpTargets := elements[1].AsList()
	sexpSources := elements[2].AsList()
	// Check target columns
	if sexpTargets == nil {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "malformed target columns"))
	}
	// Check source columns
	if sexpSources == nil {
		errors = append(errors, *p.translator.SyntaxError(elements[2], "malformed source columns"))
	}
	// Sanity check relative lengths
	if sexpTargets.Len() < sexpSources.Len() {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "too few target columns"))
	}
	// Sanity check relative lengths
	if sexpTargets.Len() > sexpSources.Len() {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "too many target columns"))
	}
	//
	if sexpTargets != nil && sexpSources != nil {
		signing := true
		targets = make([]*ast.DefColumn, sexpTargets.Len())
		sources = make([]ast.Symbol, sexpSources.Len())
		//
		for i := 0; i < min(len(sources), len(targets)); i++ {
			var (
				sign *bool
				err  *SyntaxError
			)
			//
			ith_src := sexpSources.Get(i)
			// Parse target column
			if targets[i], err = p.parseColumnDeclaration(module, module, true, 1, sexpTargets.Get(i)); err != nil {
				errors = append(errors, *err)
			}
			// Parse source column
			if !signing && ith_src.AsList() != nil {
				// Cannot begin with a negative sign
				errors = append(errors, *p.translator.SyntaxError(ith_src, "sorted columns must come first"))
			} else if sources[i], sign, err = p.parsePermutedColumnAccess(ith_src); err != nil {
				errors = append(errors, *err)
			} else if i == 0 && sign != nil && !*sign {
				// Cannot begin with a negative sign
				errors = append(errors, *p.translator.SyntaxError(ith_src, "expected positive sort"))
			} else if sign != nil {
				signs = append(signs, *sign)
			} else if i == 0 {
				errors = append(errors, *p.translator.SyntaxError(ith_src, "missing sort direction"))
			}
			// Check whether still signing
			signing = ith_src.AsList() != nil
		}
	}
	// Error Check
	if len(errors) != 0 {
		return nil, errors
	}
	//
	return ast.NewDefPermutation(targets, sources, signs), nil
}

// Parse a permutation declaration
func (p *Parser) parseDefSorted(module file.Path, strict bool, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	//
	var (
		selector           util.Option[ast.Expr]
		errors             []SyntaxError
		sources            []ast.Expr
		sexpSourcesElement sexp.SExp
		sexpSources        *sexp.List
		signs              []bool
		handle             string
	)
	//
	if len(elements) == 3 {
		// selector not provided
		sexpSourcesElement = elements[2]
	} else {
		// selector provided
		sexpSourcesElement = elements[3]
		// Translate selector expression
		expr, errs := p.translator.Translate(elements[2])
		selector = util.Some(expr)
		// update errors
		errors = append(errors, errs...)
	}
	//
	sexpSources = sexpSourcesElement.AsList()
	// Check Handle
	if !isIdentifier(elements[1]) {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "malformed handle"))
	} else {
		handle = elements[1].AsSymbol().Value
	}
	// Generate qualified name
	qualifiedHandle := fmt.Sprintf("%s:%s", module.String(), handle)
	// Check for duplicate handle
	if _, ok := p.handles[qualifiedHandle]; ok {
		return nil, p.translator.SyntaxErrors(elements[1], "duplicate handle")
	} else if len(errors) == 0 {
		// Record handle
		p.handles[qualifiedHandle] = true
	}
	// Check source Expressions
	if sexpSources == nil {
		errors = append(errors, *p.translator.SyntaxError(sexpSourcesElement, "malformed source columns"))
	}
	// Sanity check number of columns matches
	if sexpSources != nil {
		signing := true
		sources = make([]ast.Expr, sexpSources.Len())
		signs = make([]bool, 0)
		// Translate source & target expressions
		for i := 0; i < sexpSources.Len(); i++ {
			var (
				err  *SyntaxError
				sign *bool
			)
			//
			ith := sexpSources.Get(i)
			//
			if !signing && ith.AsList() != nil {
				// Cannot begin with a negative sign
				errors = append(errors, *p.translator.SyntaxError(ith, "sorted columns must come first"))
			} else if sources[i], sign, err = p.parsePermutedColumnAccess(ith); err != nil {
				errors = append(errors, *err)
			} else if i == 0 && sign != nil && !*sign {
				// Cannot begin with a negative sign
				errors = append(errors, *p.translator.SyntaxError(ith, "expected positive sort"))
			} else if sign != nil {
				signs = append(signs, *sign)
			} else if i == 0 {
				errors = append(errors, *p.translator.SyntaxError(ith, "missing sort direction"))
			}
			// Check whether still signing
			signing = ith.AsList() != nil
		}
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefSorted(handle, selector, sources, signs, strict), nil
}

func (p *Parser) parsePermutedColumnAccess(e sexp.SExp) (*ast.VariableAccess, *bool, *SyntaxError) {
	//
	var (
		err  *SyntaxError
		name string
		sign *bool = nil
	)
	// Check whether extended declaration or not.
	if l := e.AsList(); l != nil {
		// Check at least the name provided.
		if len(l.Elements) == 0 {
			return nil, nil, p.translator.SyntaxError(l, "empty permutation column")
		} else if len(l.Elements) != 2 {
			return nil, nil, p.translator.SyntaxError(l, "malformed permutation column")
		} else if l.Get(0).AsSymbol() == nil || l.Get(1).AsSymbol() == nil {
			return nil, nil, p.translator.SyntaxError(l, "empty permutation column")
		}
		// Parse sign
		if sign, err = p.parsePermutedColumnSign(l.Get(0).AsSymbol()); err != nil {
			return nil, nil, err
		}
		// Parse column name
		name = l.Get(1).AsSymbol().Value
	} else {
		name = e.String(false)
	}
	//
	if path, err := parseQualifiableName(name); err == nil {
		colAccess := ast.NewVariableAccess(path, ast.NON_FUNCTION, nil)
		// Update source mapping
		p.mapSourceNode(e, colAccess)
		//
		return colAccess, sign, nil
	} else {
		return nil, nil, p.translator.SyntaxError(e, err.Error())
	}
}

func (p *Parser) parsePermutedColumnSign(sign *sexp.Symbol) (*bool, *SyntaxError) {
	switch sign.Value {
	case "+", "↓":
		val := true
		return &val, nil
	case "-", "↑":
		val := false
		return &val, nil
	default:
		return nil, p.translator.SyntaxError(sign, "malformed sort direction")
	}
}

// Parse a perspective declaration
func (p *Parser) parseDefPerspective(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors       []SyntaxError
		sexp_columns *sexp.List = elements[3].AsList()
		columns      []*ast.DefColumn
		perspective  *ast.PerspectiveName
	)
	// Check for columns
	if sexp_columns == nil {
		errors = append(errors, *p.translator.SyntaxError(elements[3], "expected column declarations"))
	}
	// Translate selector
	selector, errs := p.translator.Translate(elements[2])
	errors = append(errors, errs...)
	// Parse perspective selector
	binding := ast.NewPerspectiveBinding(selector)
	// Parse perspective name
	if perspective, errs = parseSymbolName(p, elements[1], module, ast.NON_FUNCTION, binding); len(errs) != 0 {
		errors = append(errors, errs...)
	}
	// Process column declarations one by one.
	if sexp_columns != nil && perspective != nil {
		columns = make([]*ast.DefColumn, sexp_columns.Len())

		for i := 0; i < len(sexp_columns.Elements); i++ {
			decl, err := p.parseColumnDeclaration(module, *perspective.Path(), false, 1, sexp_columns.Elements[i])
			// Extract column name
			if err != nil {
				errors = append(errors, *err)
			}
			// Assign the declaration
			columns[i] = decl
		}
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	//
	return ast.NewDefPerspective(perspective, selector, columns), nil
}

// Parse a property assertion
func (p *Parser) parseDefProperty(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		errors    []SyntaxError
		assertion int = len(elements) - 1
		domain    util.Option[int]
		handle    string
	)
	// Check Handle
	if !isIdentifier(elements[1]) {
		errors = append(errors, *p.translator.SyntaxError(elements[1], "malformed handle"))
	} else {
		// Extract handle
		handle = elements[1].AsSymbol().Value
	}
	// Generate qualified name
	qualifiedHandle := fmt.Sprintf("%s:%s", module.String(), handle)
	// Check for duplicate handle
	if _, ok := p.handles[qualifiedHandle]; ok {
		return nil, p.translator.SyntaxErrors(elements[1], "duplicate handle")
	} else if len(errors) == 0 {
		// Record handle
		p.handles[qualifiedHandle] = true
	}
	// Check for any attributes
	if len(elements) > 3 {
		var errs []SyntaxError

		domain, errs = p.parsePropertyAttributes(elements[2])
		errors = append(errors, errs...)
	}
	// Translate expression
	expr, errs := p.translator.Translate(elements[assertion])
	errors = append(errors, errs...)
	// Error Check
	if len(errors) != 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefProperty(handle, domain, expr), nil
}

func (p *Parser) parsePropertyAttributes(attributes sexp.SExp) (domain util.Option[int], err []SyntaxError) {
	//
	var errors []SyntaxError
	// Check attribute list is a list
	if attributes.AsList() == nil {
		return util.None[int](), p.translator.SyntaxErrors(attributes, "expected attribute list")
	}
	// Deconstruct as list
	attrs := attributes.AsList()
	// Process each attribute in turn
	for i := 0; i < attrs.Len(); i++ {
		ith := attrs.Get(i)
		// Check start of attribute
		if ith.AsSymbol() == nil {
			errors = append(errors, *p.translator.SyntaxError(ith, "malformed attribute"))
		} else {
			var errs []SyntaxError
			// Check what we've got
			switch ith.AsSymbol().Value {
			case ":domain":
				i++
				domain, errs = p.parseDomainAttribute(attrs.Get(i))
			default:
				errs = p.translator.SyntaxErrors(ith, "unknown attribute")
			}
			//
			if len(errs) != 0 {
				errors = append(errors, errs...)
			}
		}
	}
	// Error Check
	if len(errors) != 0 {
		return util.None[int](), errors
	}
	// Done
	return domain, nil
}

// Parse a permutation declaration
func (p *Parser) parseDefFun(module file.Path, pure bool, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		name      *sexp.Symbol
		ret       ast.Type
		forced    bool
		params    []*ast.DefParameter
		errors    []SyntaxError
		signature *sexp.List = elements[1].AsList()
	)
	// Parse signature
	if signature == nil || signature.Len() == 0 {
		err := p.translator.SyntaxError(elements[1], "malformed function signature")
		errors = append(errors, *err)
	} else {
		name, ret, forced, params, errors = p.parseFunSignature(signature.Elements)
	}
	// Translate expression
	body, errs := p.translator.Translate(elements[2])
	// Apply return type
	if ret != nil {
		// TODO: the notion of "forcing" should be deprecated in favour of
		// explicit type casts.
		body = &ast.Cast{Arg: body, Type: ret, Unsafe: forced}
		p.mapSourceNode(elements[2], body)
	}
	//
	errors = append(errors, errs...)
	// Check for errors
	if len(errors) > 0 {
		return nil, errors
	}
	// Extract parameter types
	paramTypes := make([]ast.Type, len(params))
	for i, p := range params {
		paramTypes[i] = p.Binding.DataType
	}
	// Construct binding
	path := module.Extend(name.Value)
	binding := ast.NewDefunBinding(pure, paramTypes, ret, forced, body)
	fn_name := ast.NewFunctionName(*path, &binding)
	// Update source mapping
	p.mapSourceNode(name, fn_name)
	//
	return ast.NewDefFun(fn_name, params, ret), nil
}

func (p *Parser) parseFunSignature(elements []sexp.SExp) (*sexp.Symbol,
	ast.Type, bool, []*ast.DefParameter, []SyntaxError) {
	//
	var params []*ast.DefParameter = make([]*ast.DefParameter, len(elements)-1)
	// Parse name and (optional) return type
	name, ret, forced, errors := p.parseFunctionNameReturn(elements[0])
	// Parse parameters
	for i := 0; i < len(params); i = i + 1 {
		var errs []SyntaxError

		if params[i], errs = p.parseFunctionParameter(elements[i+1]); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}
	// Check for any errors arising
	if len(errors) > 0 {
		return nil, nil, false, nil, errors
	}
	//
	return name, ret, forced, params, nil
}

func (p *Parser) parseFunctionNameReturn(element sexp.SExp) (*sexp.Symbol, ast.Type, bool, []SyntaxError) {
	var (
		err    *SyntaxError
		name   sexp.SExp
		ret    ast.Type = nil
		forced bool
		symbol *sexp.Symbol = element.AsSymbol()
		list   *sexp.List   = element.AsList()
	)
	//
	if symbol != nil {
		name = symbol
	} else {
		// Check all modifiers
		for i, element := range list.Elements {
			symbol := element.AsSymbol()
			// Check what we have
			if symbol == nil {
				err := p.translator.SyntaxError(element, "modifier expected")
				return nil, nil, false, []SyntaxError{*err}
			} else if i == 0 {
				name = symbol
			} else {
				switch symbol.Value {
				case ":force":
					forced = true
				default:
					if ret, _, err = p.parseType(element); err != nil {
						return nil, nil, false, []SyntaxError{*err}
					}
				}
			}
		}
	}
	//
	if isFunIdentifier(name) {
		return name.AsSymbol(), ret, forced, nil
	} else {
		// Must be non-identifier symbol
		err = p.translator.SyntaxError(element, "invalid function name")
		return nil, nil, false, []SyntaxError{*err}
	}
}

func (p *Parser) parseFunctionParameter(element sexp.SExp) (*ast.DefParameter, []SyntaxError) {
	list := element.AsList()
	//
	if isIdentifier(element) {
		return ast.NewDefParameter(element.AsSymbol().Value, ast.UINT_TYPE), nil
	} else if list == nil || list.Len() != 2 || !isIdentifier(list.Get(0)) {
		// Construct error message (for now)
		err := p.translator.SyntaxError(element, "malformed parameter declaration")
		//
		return nil, []SyntaxError{*err}
	}
	// Parse the type
	datatype, prove, err := p.parseType(list.Get(1))
	//
	if err != nil {
		return nil, []SyntaxError{*err}
	} else if prove && !p.config.EnforceTypes {
		// Parameters cannot be marked @prove
		err := p.translator.SyntaxError(element, "malformed parameter declaration")
		//
		return nil, []SyntaxError{*err}
	}
	// Done
	return ast.NewDefParameter(list.Get(0).AsSymbol().Value, datatype), nil
}

// Parse a range declaration
func (p *Parser) parseDefInRange(elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		bound int
		err   error
	)
	// Translate expression
	expr, errors := p.translator.Translate(elements[1])
	// Check & parse bound
	if elements[2].AsSymbol() == nil {
		errors = append(errors, *p.translator.SyntaxError(elements[2], "malformed bound"))
	} else if bound, err = strconv.Atoi(elements[2].AsSymbol().Value); err != nil {
		errors = append(errors, *p.translator.SyntaxError(elements[2], "malformed bound"))
	}
	// Error check
	if len(errors) != 0 {
		return nil, errors
	}
	// Sanity check that the bound is actually a power of two.  Since range
	// constraints are now compiled into table lookups, it is simpler to limit
	// them accordingly.
	if bitwidth := bitwidth(bound); bitwidth != math.MaxUint {
		return &ast.DefInRange{Expr: expr, Bitwidth: bitwidth}, nil
	}
	//
	return nil, p.translator.SyntaxErrors(elements[2], "bound not power of 2")
}

func bitwidth(bound int) uint {
	// Determine actual bound
	bitwidth := uint(1)
	acc := 2
	//
	for ; acc < bound; acc = acc * 2 {
		bitwidth++
	}
	// Check whethe it makes sense
	if acc == bound {
		return bitwidth
	}
	// invalid bound
	return math.MaxUint
}

func (p *Parser) parseConstraintAttributes(module file.Path, attributes sexp.SExp) (domain util.Option[int],
	guard ast.Expr, perspective *ast.PerspectiveName, err []SyntaxError) {
	//
	var errors []SyntaxError
	// Check attribute list is a list
	if attributes.AsList() == nil {
		return util.None[int](), nil, nil, p.translator.SyntaxErrors(attributes, "expected attribute list")
	}
	// Deconstruct as list
	attrs := attributes.AsList()
	// Process each attribute in turn
	for i := 0; i < attrs.Len(); i++ {
		ith := attrs.Get(i)
		// Check start of attribute
		if ith.AsSymbol() == nil {
			errors = append(errors, *p.translator.SyntaxError(ith, "malformed attribute"))
		} else {
			var errs []SyntaxError
			// Check what we've got
			switch ith.AsSymbol().Value {
			case ":domain":
				i++
				domain, errs = p.parseDomainAttribute(attrs.Get(i))
			case ":guard":
				i++
				guard, errs = p.translator.Translate(attrs.Get(i))
			case ":perspective":
				i++
				perspective, errs = parseSymbolName[*ast.PerspectiveBinding](p, attrs.Get(i), module, ast.NON_FUNCTION, nil)
			default:
				errs = p.translator.SyntaxErrors(ith, "unknown attribute")
			}
			//
			if len(errs) != 0 {
				errors = append(errors, errs...)
			}
		}
	}
	// Error Check
	if len(errors) != 0 {
		return util.None[int](), nil, nil, errors
	}
	// Done
	return domain, guard, perspective, nil
}

// Parse a symbol name, which will include a binding.
func parseSymbolName[T ast.Binding](p *Parser, symbol sexp.SExp, module file.Path, arity util.Option[uint],
	binding T) (*ast.Name[T], []SyntaxError) {
	//
	if !isEitherOrIdentifier(symbol, arity.HasValue()) {
		return nil, p.translator.SyntaxErrors(symbol, "expected identifier")
	}
	// Extract
	path := module.Extend(symbol.AsSymbol().Value)
	name := ast.NewBoundName(*path, arity, binding)
	// Update source mapping
	p.mapSourceNode(symbol, name)
	// Construct
	return name, nil
}

func (p *Parser) parseDomainAttribute(attribute sexp.SExp) (domain util.Option[int], err []SyntaxError) {
	if attribute.AsSet() == nil {
		return util.None[int](), p.translator.SyntaxErrors(attribute, "malformed domain set")
	}
	// Sanity check
	set := attribute.AsSet()
	// Check all domain elements well-formed.
	for i := 0; i < set.Len(); i++ {
		ith := set.Get(i)
		if ith.AsSymbol() == nil {
			return util.None[int](), p.translator.SyntaxErrors(ith, "malformed domain")
		}
	}
	// Currently, only support domains of size 1.
	if set.Len() == 1 {
		first, err := strconv.Atoi(set.Get(0).AsSymbol().Value)
		// Check for parse error
		if err != nil {
			return util.None[int](), p.translator.SyntaxErrors(set.Get(0), "malformed domain element")
		}
		// Done
		return util.Some(first), nil
	}
	// Fail
	return util.None[int](), p.translator.SyntaxErrors(attribute, "multiple values not supported")
}

func (p *Parser) parseType(term sexp.SExp) (ast.Type, bool, *SyntaxError) {
	symbol := term.AsSymbol()
	if symbol == nil {
		return nil, false, p.translator.SyntaxError(term, "malformed type")
	}
	// Access string of symbol
	parts := strings.Split(symbol.Value, "@")
	// Determine whether type should be proven or not.
	var datatype ast.Type
	// See what we've got.
	switch parts[0] {
	case ":bool":
		datatype = ast.BOOL_TYPE
	case ":binary":
		datatype = ast.NewUintType(1)
	case ":byte":
		datatype = ast.NewUintType(8)
	case ":int":
		datatype = ast.UINT_TYPE
	case ":any":
		datatype = ast.ANY_TYPE
	default:
		// Handle generic types like i16, i128, etc.
		str := parts[0]
		if !strings.HasPrefix(str, ":i") && !strings.HasPrefix(str, ":u") {
			return nil, false, p.translator.SyntaxError(symbol, "unknown type")
		}
		// Parse bitwidth
		n, err := strconv.Atoi(str[2:])
		if err != nil {
			return nil, false, p.translator.SyntaxError(symbol, err.Error())
		}
		// Done
		datatype = ast.NewUintType(uint(n))
	}
	// Types default to not proven (unless explicitly requested)
	var proven bool = p.config.EnforceTypes
	// Process type modifiers
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "prove":
			proven = true
		default:
			msg := fmt.Sprintf("unknown modifier \"%s\"", parts[i])
			return nil, false, p.translator.SyntaxError(symbol, msg)
		}
	}
	// Done
	return datatype, proven, nil
}

func beginParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	return &ast.List{Args: args}, nil
}

func debugParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	if len(args) == 1 {
		return &ast.Debug{Arg: args[0]}, nil
	}
	//
	return nil, errors.New("incorrect number of arguments")
}

func forParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		var (
			errors   []SyntaxError
			indexVar *sexp.Symbol
		)
		// Check we've got the expected number
		if list.Len() != 4 {
			msg := fmt.Sprintf("expected 3 arguments, found %d", list.Len()-1)
			return nil, p.translator.SyntaxErrors(list, msg)
		}
		// Extract index variable
		if indexVar = list.Get(1).AsSymbol(); indexVar == nil {
			err := p.translator.SyntaxError(list.Get(1), "invalid index variable")
			errors = append(errors, *err)
		}
		// Parse range
		start, end, errs := parseForRange(p, list.Get(2))
		// Error Check
		errors = append(errors, errs...)
		// Parse body
		body, errs := p.translator.Translate(list.Get(3))
		errors = append(errors, errs...)
		// Error check
		if len(errors) > 0 {
			return nil, errors
		}
		// Construct expression.  At this stage, its unclear what the best type
		// to use for the index variable is here.  Potentially, it could be
		// refined based on the range of actual values, etc.
		return ast.NewFor(indexVar.Value, start, end, body), nil
	}
}

func letParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		var (
			errors []SyntaxError
		)
		// Check we've got the expected number
		if list.Len() != 3 {
			msg := fmt.Sprintf("expected 2 arguments, found %d", list.Len()-1)
			return nil, p.translator.SyntaxErrors(list, msg)
		} else if list.Get(1).AsList() == nil {
			return nil, p.translator.SyntaxErrors(list.Get(1), "expected list")
		}
		// Prep assignments
		assignments := list.Get(1).AsList()
		bindings := make([]util.Pair[string, ast.Expr], assignments.Len())
		names := make(map[string]bool)
		// Parse var assignmnts
		for i, e := range assignments.Elements {
			// Sanity checks first
			if ith := e.AsList(); ith == nil {
				errors = append(errors, *p.translator.SyntaxError(e, "expected list"))
			} else if ith.Len() != 2 {
				errors = append(errors, *p.translator.SyntaxError(e, "malformed let assignment"))
			} else if !isIdentifier(ith.Get(0)) {
				errors = append(errors, *p.translator.SyntaxError(e, "invalid let name"))
			} else {
				name := ith.Get(0).AsSymbol().Value
				// sanity check names are unique
				if _, ok := names[name]; ok {
					// name already defined
					errors = append(errors, *p.translator.SyntaxError(ith.Get(0), "already defined"))
				}
				//
				names[name] = true
				expr, errs := p.translator.Translate(ith.Get(1))
				errors = append(errors, errs...)
				bindings[i] = util.NewPair(name, expr)
			}
		}
		// Parse body
		body, errs := p.translator.Translate(list.Get(2))
		errors = append(errors, errs...)
		// Error check
		if len(errors) > 0 {
			return nil, errors
		}
		// Done
		return ast.NewLet(bindings, body), nil
	}
}

// Parse a range which, represented as a string is "[s:e]".
func parseForRange(p *Parser, interval sexp.SExp) (uint, uint, []SyntaxError) {
	var (
		start int
		end   int
		err1  error
		err2  error
	)
	// This is a bit dirty.  Essentially, we turn the sexp.Array back into a
	// string and then parse it from there.
	str := interval.String(false)
	// Strip out any whitespace (which is permitted)
	str = strings.ReplaceAll(str, " ", "")
	// Check has form "[...]"
	if !strings.HasPrefix(str, "[") || !strings.HasSuffix(str, "]") {
		// error
		return 0, 0, p.translator.SyntaxErrors(interval, "invalid interval")
	}
	// Split out components
	splits := strings.Split(str[1:len(str)-1], ":")
	// Error check
	if len(splits) == 0 || len(splits) > 2 {
		// error
		return 0, 0, p.translator.SyntaxErrors(interval, "invalid interval")
	} else if len(splits) == 1 {
		end, err1 = strconv.Atoi(splits[0])
		start = 1
	} else if len(splits) == 2 {
		start, err1 = strconv.Atoi(splits[0])
		end, err2 = strconv.Atoi(splits[1])
	}
	//
	if err1 != nil || err2 != nil {
		return 0, 0, p.translator.SyntaxErrors(interval, "invalid interval")
	}
	// Success
	return uint(start), uint(end), nil
}

func reduceParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		var errors []SyntaxError
		// Check we've got the expected number
		if list.Len() != 3 {
			msg := fmt.Sprintf("expected 2 arguments, found %d", list.Len()-1)
			return nil, p.translator.SyntaxErrors(list, msg)
		}
		// function name
		name := list.Get(1).AsSymbol()
		//
		if name == nil {
			errors = append(errors, *p.translator.SyntaxError(list.Get(1), "invalid function"))
		}
		// Parse body
		body, errs := p.translator.Translate(list.Get(2))
		errors = append(errors, errs...)
		// Error check
		if len(errors) > 0 {
			return nil, errors
		}
		//
		path := file.NewRelativePath(name.Value)
		arity := util.Some(uint(2))
		varaccess := ast.NewVariableAccess(path, arity, nil)
		p.mapSourceNode(name, varaccess)
		// Done
		return &ast.Reduce{Name: varaccess, Arg: body}, nil
	}
}

func constantParserRule(symbol string) (ast.Expr, bool, error) {
	num, ok, err := parseConstant(symbol)
	// Check for error
	if !ok || err != nil {
		return nil, ok, err
	}
	// Success
	return &ast.Constant{Val: num}, true, nil
}

func varAccessParserRule(col string) (ast.Expr, bool, error) {
	// Sanity check what we have
	if col[0] != '_' && !unicode.IsLetter(rune(col[0])) {
		return nil, false, errors.New("malformed column access")
	}
	// Handle qualified accesses (where permitted)
	// Attempt to split column name into module / column pair.
	path, err := parseQualifiableName(col)
	// Sanity check for errors
	if err != nil {
		return nil, true, err
	}
	//
	return ast.NewVariableAccess(path, ast.NON_FUNCTION, nil), true, nil
}

func arrayAccessParserRule(name string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 1 {
		return nil, errors.New("malformed array access")
	}
	// Handle qualified accesses (where permitted)
	// Attempt to split column name into module / column pair.
	path, err := parseQualifiableName(name)
	if err != nil {
		return nil, err
	}
	//
	return &ast.ArrayAccess{Name: path, Arg: args[0], ArrayBinding: nil}, nil
}

func addParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	return &ast.Add{Args: args}, nil
}

func concatParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	// Reverse the order as we want most significant to be highest in the actual
	// array.
	array.ReverseInPlace(args)
	//
	return &ast.Concat{Args: args}, nil
}

func subParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	return &ast.Sub{Args: args}, nil
}

func mulParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	return &ast.Mul{Args: args}, nil
}

func ifParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		var (
			condition           ast.Expr
			lhs, rhs            ast.Expr
			errs1, errs2, errs3 []SyntaxError
		)
		// Can assume first item of list is "if"
		if list.Len() != 3 && list.Len() != 4 {
			return nil, p.translator.SyntaxErrors(list, "incorrect number of arguments")
		}
		// Translate condition
		condition, errs1 = p.translator.Translate(list.Get(1))
		lhs, errs2 = p.translator.Translate(list.Get(2))
		//
		if list.Len() == 4 {
			rhs, errs3 = p.translator.Translate(list.Get(3))
		}
		//
		errs := append(errs1, append(errs2, errs3...)...)
		// Error Check
		if len(errs) > 0 {
			return nil, errs
		}
		//
		return &ast.If{Condition: condition, TrueBranch: lhs, FalseBranch: rhs}, nil
	}
}

func invokeParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		var (
			varaccess *ast.VariableAccess
			errors    []SyntaxError
		)
		//
		if list.Len() == 0 || list.Get(0).AsSymbol() == nil {
			return nil, p.translator.SyntaxErrors(list, "invalid invocation")
		}
		// Extract function name
		name := list.Get(0).AsSymbol()
		// Sanity check what we have
		if !isFunIdentifier(name) {
			errors = append(errors, *p.translator.SyntaxError(list.Get(0), "invalid function name"))
		}
		// Handle qualified accesses (where permitted)
		path, err := parseQualifiableName(name.Value)
		//
		if err != nil {
			return nil, p.translator.SyntaxErrors(list.Get(0), "invalid function name")
		} else {
			arity := util.Some(uint(list.Len() - 1))
			varaccess = ast.NewVariableAccess(path, arity, nil)
		}
		// Parse arguments
		args := make([]ast.Expr, list.Len()-1)
		for i := 0; i < len(args); i++ {
			var errs []SyntaxError
			//
			args[i], errs = p.translator.Translate(list.Get(i + 1))
			errors = append(errors, errs...)
		}
		// Error check
		if len(errors) > 0 {
			return nil, errors
		}
		//
		p.mapSourceNode(list.Get(0), varaccess)
		// Done
		return &ast.Invoke{Name: varaccess, Args: args}, nil
	}
}

func shiftParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 2 {
		return nil, errors.New("incorrect number of arguments")
	}
	// Done
	return &ast.Shift{Arg: args[0], Shift: args[1]}, nil
}

func powParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 2 {
		return nil, errors.New("incorrect number of arguments")
	}
	// Done
	return &ast.Exp{Arg: args[0], Pow: args[1]}, nil
}

func eqParserRule(op string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 2 {
		return nil, errors.New("incorrect number of arguments")
	}
	//
	switch op {
	case "==":
		return &ast.Equation{Kind: ast.EQUALS, Lhs: args[0], Rhs: args[1]}, nil
	case "!=":
		return &ast.Equation{Kind: ast.NOT_EQUALS, Lhs: args[0], Rhs: args[1]}, nil
	}
	//
	panic("unreachable")
}

func logicalParserRule(op string, args []ast.Expr) (ast.Expr, error) {
	if len(args) == 0 {
		return nil, errors.New("incorrect number of arguments")
	}
	//
	switch op {
	case "∨":
		return &ast.Connective{Sign: true, Args: args}, nil
	case "∧":
		return &ast.Connective{Sign: false, Args: args}, nil
	}
	//
	panic("unreachable")
}

func logicalNegationRule(op string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 1 {
		return nil, errors.New("incorrect number of arguments")
	}
	//
	return &ast.Not{Arg: args[0]}, nil
}

func normParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 1 {
		return nil, errors.New("incorrect number of arguments")
	}

	return &ast.Normalise{Arg: args[0]}, nil
}

func parseConstant(symbol string) (constant big.Int, ok bool, err error) {
	var (
		base int
		num  big.Int
		name string
	)
	//
	if strings.HasPrefix(symbol, "0x") {
		symbol = symbol[2:]
		base = 16
		name = "hexadecimal"
	} else if (symbol[0] >= '0' && symbol[0] <= '9') || symbol[0] == '-' {
		base = 10
		name = "integer"
	} else {
		// Not applicable
		return big.Int{}, false, nil
	}
	// Attempt to parse
	if _, ok := num.SetString(symbol, base); !ok {
		err := fmt.Sprintf("invalid %s constant", name)
		return num, true, errors.New(err)
	}
	// Done
	return num, true, nil
}

// Parse a name which can be (optionally) adorned with either a module or
// perspective qualifier, or both.
func parseQualifiableName(qualName string) (path file.Path, err error) {
	// Look for module qualification
	split := strings.Split(qualName, ".")
	switch len(split) {
	case 1:
		return parsePerspectifiableName(qualName)
	case 2:
		module := split[0]
		path, err := parsePerspectifiableName(split[1])

		return *path.PushRoot(module), err
	default:
		return path, errors.New("malformed qualified name")
	}
}

// Parse a name which can (optionally) adorned with a perspective qualifier
func parsePerspectifiableName(qualName string) (path file.Path, err error) {
	// Look for module qualification
	split := strings.Split(qualName, "/")
	switch len(split) {
	case 1:
		return file.NewRelativePath(split[0]), nil
	case 2:
		return file.NewRelativePath(split[0], split[1]), nil
	default:
		return path, errors.New("malformed qualified name")
	}
}

// Attempt to parse an S-Expression as an identifier, return nil if this fails.
// The function flag switches this to identifiers suitable for functions and
// invocations.
func isEitherOrIdentifier(sexp sexp.SExp, function bool) bool {
	if function {
		return isFunIdentifier(sexp)
	}
	//
	return isIdentifier(sexp)
}

// Attempt to parse an S-Expression as an identifier suitable for something
// which is not a function (e.g. column, constant, etc).
func isIdentifier(sexp sexp.SExp) bool {
	if symbol := sexp.AsSymbol(); symbol != nil && len(symbol.Value) > 0 {
		runes := []rune(symbol.Value)
		if isIdentifierStart(runes[0]) {
			for i := 1; i < len(runes); i++ {
				if !isIdentifierMiddle(runes[i]) {
					return false
				}
			}
			// Success
			return true
		}
	}
	// Fail
	return false
}

// Attempt to parse an S-Expression as an identifier suitable for something
// which is not a function (e.g. column, constant, etc).
func isFunIdentifier(sexp sexp.SExp) bool {
	if symbol := sexp.AsSymbol(); symbol != nil && len(symbol.Value) > 0 {
		runes := []rune(symbol.Value)
		if isFunctionSymbol(runes) {
			return true
		} else if isFunctionIdentifierStart(runes[0]) {
			for i := 1; i < len(runes); i++ {
				if !isIdentifierMiddle(runes[i]) {
					return false
				}
			}
			// Success
			return true
		}
	}
	// Fail
	return false
}

func isIdentifierStart(c rune) bool {
	return unicode.IsLetter(c) || c == '_' || c == '\'' || c == '$'
}

func isIdentifierMiddle(c rune) bool {
	return unicode.IsDigit(c) || isIdentifierStart(c) || c == '-' || c == '!' || c == '@'
}

func isFunctionIdentifierStart(c rune) bool {
	return isIdentifierStart(c) || c == '~'
}

func isFunctionSymbol(runes []rune) bool {
	return len(runes) == 1 && (runes[0] == '+' || runes[0] == '*' || runes[0] == '-' || runes[0] == '=')
}
