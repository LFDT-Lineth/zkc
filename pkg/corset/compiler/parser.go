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
	p.AddRecursiveListRule("¬", logicalNegationRule)
	p.AddRecursiveListRule("∨", logicalParserRule)
	p.AddRecursiveListRule("∧", logicalParserRule)
	p.AddRecursiveListRule("==", eqParserRule)
	p.AddRecursiveListRule("!=", eqParserRule)
	p.AddListRule("if", ifParserRule(parser))
	p.AddRecursiveListRule("shift", shiftParserRule)
	p.AddDefaultListRule(unknownExpressionParserRule(parser))
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
	if s.MatchSymbols(1, "defcolumns") {
		decl, errors = p.parseDefColumns(module, s)
	} else if s.Len() > 1 && s.MatchSymbols(1, "defconst") {
		decl, errors = p.parseDefConst(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(2, "defconstraint") {
		decl, errors = p.parseDefConstraint(module, s.Elements)
	} else if s.Len() == 3 && s.MatchSymbols(1, "definrange") {
		decl, errors = p.parseDefInRange(s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(1, "deflookup") {
		decl, errors = p.parseDefLookup(module, s.Elements)
	} else if (s.Len() == 5 || s.Len() == 6) && s.MatchSymbols(1, "defclookup") {
		decl, errors = p.parseDefConditionalLookup(module, s.Elements)
	} else if s.Len() == 4 && s.MatchSymbols(1, "defmlookup") {
		decl, errors = p.parseDefMultiLookup(module, s.Elements)
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
		decl, err := p.parseColumnDeclaration(module, module, false, l.Elements[i])
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

func (p *Parser) parseColumnDeclaration(context file.Path, path file.Path, computed bool,
	e sexp.SExp) (*ast.DefColumn, *SyntaxError) {
	//
	var (
		zero  big.Int
		error *SyntaxError
		// Initial binding with defaults
		binding = ast.ColumnBinding{
			ColumnContext: context,
			Kind:          ast.NOT_COMPUTED,
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
		// computed columns initially have no type, since this needs to be
		// subsequently determined from context.
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
	)
	// Parse head
	if name, datatype, errors = p.parseDefConstHead(head); len(errors) > 0 {
		return nil, errors
	} else if expr, errors = p.translator.Translate(value); len(errors) > 0 {
		return nil, errors
	}
	// Looks good
	path := module.Extend(name.Value)
	def := &ast.DefConstUnit{ConstBinding: ast.NewConstantBinding(*path, datatype, expr)}
	// Map to source node
	p.mapSourceNode(value, def)
	// Done
	return def, nil
}

func (p *Parser) parseDefConstHead(head sexp.SExp) (*sexp.Symbol, ast.Type, []SyntaxError) {
	var (
		list     = head.AsList()
		datatype ast.Type
	)

	// Parse the head
	if isIdentifier(head) {
		// no attributes provided
		return head.AsSymbol(), nil, nil
	} else if list == nil {
		return nil, nil, p.translator.SyntaxErrors(head, "invalid constant name")
	} else if list.Len() < 2 {
		return nil, nil, p.translator.SyntaxErrors(list, "invalid constant declaration")
	} else if !isIdentifier(list.Get(0)) {
		return nil, nil, p.translator.SyntaxErrors(list.Get(0), "invalid constant name")
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
			return nil, nil, p.translator.SyntaxErrors(list.Get(i), "invalid constant attribute")
		}
		//
		datatype, prove, err = p.parseType(sym)
		// Handle errors
		if err != nil {
			return nil, nil, []SyntaxError{*err}
		} else if prove && !p.config.EnforceTypes {
			return nil, nil, p.translator.SyntaxErrors(list, "constants cannot have proven types")
		}
	}
	// Sanity check type
	return list.Get(0).AsSymbol(), datatype, nil
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
	domain, guard, errs := p.parseConstraintAttributes(elements[2])
	errors = append(errors, errs...)
	// Translate expression
	expr, errs := p.translator.Translate(elements[3])
	errors = append(errors, errs...)
	// Error Check
	if len(errors) > 0 {
		return nil, errors
	}
	// Done
	return ast.NewDefConstraint(handle, domain, guard, expr), nil
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
	targetSelectors := make([]ast.TypedSymbol, 1)
	sourceSelectors := make([]ast.TypedSymbol, 1)
	// Done
	return ast.NewDefLookup(handle,
		checked,
		sourceSelectors, [][]ast.TypedSymbol{sources},
		targetSelectors, [][]ast.TypedSymbol{targets}), nil
}

// Parse a conditional lookup declaration
func (p *Parser) parseDefConditionalLookup(module file.Path, elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	// Extract items
	var (
		targets, sources               []ast.TypedSymbol
		targetSelector, sourceSelector ast.TypedSymbol
		errs1, errs2, errs3, errs4     []SyntaxError
	)
	//
	handle, checked, errors := p.parseLookupHandle(module, elements[1])
	//
	if len(elements) == 6 {
		targetSelector, errs1 = p.parseDefLookupSelector(elements[2])
		targets, errs2 = p.parseDefLookupSources("target", elements[3])
		sourceSelector, errs3 = p.parseDefLookupSelector(elements[4])
		sources, errs4 = p.parseDefLookupSources("source", elements[5])
	} else {
		// Assume source selector
		targets, errs1 = p.parseDefLookupSources("target", elements[2])
		sourceSelector, errs2 = p.parseDefLookupSelector(elements[3])
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
		[]ast.TypedSymbol{sourceSelector},
		[][]ast.TypedSymbol{sources},
		[]ast.TypedSymbol{targetSelector},
		[][]ast.TypedSymbol{targets}), nil
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
	targetSelectors := make([]ast.TypedSymbol, len(targets))
	sourceSelectors := make([]ast.TypedSymbol, len(sources))
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

func (p *Parser) parseDefLookupMultiSources(handle string, element sexp.SExp) (int, [][]ast.TypedSymbol,
	[]SyntaxError) {
	//
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
	targets := make([][]ast.TypedSymbol, sexpTargets.Len())
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

func (p *Parser) parseDefLookupSources(handle string, element sexp.SExp) ([]ast.TypedSymbol, []SyntaxError) {
	var (
		sexpSources = element.AsList()
		errors      []SyntaxError
		sources     []ast.TypedSymbol
	)
	// Check source columns
	if sexpSources == nil {
		msg := fmt.Sprintf("malformed %s columns", handle)
		return nil, p.translator.SyntaxErrors(element, msg)
	}
	//
	sources = make([]ast.TypedSymbol, sexpSources.Len())
	//
	for i := 0; i != sexpSources.Len(); i++ {
		msg := fmt.Sprintf("malformed %s column", handle)
		// Sources must be column accesses, rather than arbitrary expressions.
		ith, errs := p.parseColumnAccess(sexpSources.Get(i), msg)
		//
		sources[i] = ith
		//
		errors = append(errors, errs...)
	}
	//
	return sources, errors
}

// Parse the selector of a (conditional) lookup which, as for the sources /
// targets it gates, must be a column access rather than an arbitrary
// expression.
func (p *Parser) parseDefLookupSelector(element sexp.SExp) (ast.TypedSymbol, []SyntaxError) {
	return p.parseColumnAccess(element, "malformed selector")
}

// Parse a column access, as arises (for example) within a lookup or range
// constraint.  Such accesses must be simple (qualifiable) names, rather than
// arbitrary expressions.  The given message is reported when this is not the
// case.
func (p *Parser) parseColumnAccess(element sexp.SExp, msg string) (ast.TypedSymbol, []SyntaxError) {
	source := element.AsSymbol()
	//
	if source == nil {
		return nil, p.translator.SyntaxErrors(element, msg)
	} else if path, err := parseQualifiableName(source.Value); err != nil {
		return nil, p.translator.SyntaxErrors(source, err.Error())
	} else {
		varAccess := ast.NewVariableAccess(path, nil)
		p.mapSourceNode(source, varAccess)

		return varAccess, nil
	}
}

// Parse a range declaration
func (p *Parser) parseDefInRange(elements []sexp.SExp) (ast.Declaration, []SyntaxError) {
	var (
		bound int
		err   error
	)
	// Parse constrained column.  As for a lookup, this must be a column access
	// rather than an arbitrary expression.
	column, errors := p.parseColumnAccess(elements[1], "malformed column")
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
		return &ast.DefInRange{Column: column, Bitwidth: bitwidth}, nil
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

func (p *Parser) parseConstraintAttributes(attributes sexp.SExp) (domain util.Option[int],
	guard ast.Expr, err []SyntaxError) {
	//
	var errors []SyntaxError
	// Check attribute list is a list
	if attributes.AsList() == nil {
		return util.None[int](), nil, p.translator.SyntaxErrors(attributes, "expected attribute list")
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
		return util.None[int](), nil, errors
	}
	// Done
	return domain, guard, nil
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
	return ast.NewVariableAccess(path, nil), true, nil
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

func unknownExpressionParserRule(p *Parser) sexp.ListRule[ast.Expr] {
	return func(list *sexp.List) (ast.Expr, []SyntaxError) {
		return nil, p.translator.SyntaxErrors(list, "unknown expression form")
	}
}

func shiftParserRule(_ string, args []ast.Expr) (ast.Expr, error) {
	if len(args) != 2 {
		return nil, errors.New("incorrect number of arguments")
	}
	// Done
	return &ast.Shift{Arg: args[0], Shift: args[1]}, nil
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

// Parse a name which can be (optionally) adorned with a module qualifier.
func parseQualifiableName(qualName string) (path file.Path, err error) {
	// Look for module qualification
	split := strings.Split(qualName, ".")
	switch len(split) {
	case 1:
		return file.NewRelativePath(split[0]), nil
	case 2:
		relative := file.NewRelativePath(split[1])
		return *relative.PushRoot(split[0]), nil
	default:
		return path, errors.New("malformed qualified name")
	}
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

func isIdentifierStart(c rune) bool {
	return unicode.IsLetter(c) || c == '_' || c == '\'' || c == '$'
}

func isIdentifierMiddle(c rune) bool {
	return unicode.IsDigit(c) || isIdentifierStart(c) || c == '-' || c == '!' || c == '@'
}
