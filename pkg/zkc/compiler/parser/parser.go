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
package parser

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/lex"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/lval"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/stmt"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	zkc_util "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
)

// Condition is a convenient alias
type Condition = expr.Condition[symbol.Unresolved]

// Expr is a convenient alias
type Expr = expr.Expr[symbol.Unresolved]

// LVal is a convenient alias
type LVal = lval.LVal[symbol.Unresolved]

// Type is a convenient alias
type Type = data.Type[symbol.Unresolved]

// VariableDescriptor is a convenient alias
type VariableDescriptor = variable.Descriptor[symbol.Unresolved]

// UnlinkedSourceFile captures a source file has been successfully parsed but
// which has not yet been linked.   As such, its possible that such a file may
// fail with an error at link time due to an unresolvable reference to an
// external component (e.g. function, RAM, ROM, etc).
type UnlinkedSourceFile struct {
	// Components making up this assembly item.
	Components []decl.Unresolved
	// Mapping of instructions back to the source file.
	SourceMap source.Map[any]
}

// Parse accepts a given source file representing an assembly language
// program, and assembles it into an instruction sequence which can then the
// executed.
func Parse(srcfile *source.File) (UnlinkedSourceFile, []source.SyntaxError) {
	parser := NewParser(srcfile)
	// Parse functions
	return parser.Parse()
}

// BINOPS captures the set of binary operations
var BINOPS = []uint{
	SUB, MUL, ADD, DIV, REM, BITWISE_AND, BITWISE_OR, BITWISE_XOR, BITWISE_SHL,
	BITWISE_SHR, EQUALS_EQUALS, NOT_EQUALS,
	LESS_THAN, LESS_THAN_EQUALS, GREATER_THAN, GREATER_THAN_EQUALS}

// LOGICAL_BINOPS captures the set of logical binary operations
var LOGICAL_BINOPS = []uint{LOGICAL_AND, LOGICAL_OR}

// ============================================================================
// Assembler
// ============================================================================

// Parser is a parser for assembly language.
type Parser struct {
	srcfile *source.File
	tokens  []lex.Token
	// Source mapping
	srcmap *source.Map[any]
	// Position within the tokens
	index int
}

// NewParser constructs a new parser for a given source file.
func NewParser(srcfile *source.File) *Parser {
	// Construct (initially empty) source mapping
	srcmap := source.NewSourceMap[any](*srcfile)
	//
	return &Parser{srcfile, nil, srcmap, 0}
}

// Parse the given source file into a sequence of zero or more components and/or
// some number of syntax errors.
func (p *Parser) Parse() (UnlinkedSourceFile, []source.SyntaxError) {
	var (
		item         UnlinkedSourceFile
		errors, errs []source.SyntaxError
		components   []decl.Unresolved
	)
	// Convert source file into tokens
	p.tokens = Lex(*p.srcfile, false, false)
	// Continue going until all consumed
	for p.lookahead().Kind != EOF {
		// Parse declaration
		components, errs = p.parseDeclaration()
		// Accumulate errors
		errors = append(errors, errs...)
		// Check for unrecoverable error; if so, break out.
		if components == nil {
			break
		}
		//
		item.Components = append(item.Components, components...)
	}
	// Copy over source map
	item.SourceMap = *p.srcmap
	//
	return item, errors
}

func (p *Parser) parseDeclaration() ([]decl.Declaration[symbol.Unresolved], []source.SyntaxError) {
	var (
		errors      []source.SyntaxError
		components  = []decl.Unresolved{nil}
		annotations []lex.Token
		kind        decl.DeclarationKind
	)
	// Parse any leading annotations (e.g. #[inline]), checking that each
	// name is known.
	if annotations, errors = p.parseAnnotations(); len(errors) > 0 {
		return nil, errors
	}
	// See what kind of declaration this is
	lookahead := p.lookahead()
	// Determine type of declaration
	switch lookahead.Kind {
	case KEYWORD_CONST:
		kind = decl.CONSTANT_KIND
		components, errors = p.parseConstant()
	case KEYWORD_INCLUDE:
		kind = decl.INCLUDE_KIND
		components[0], errors = p.parseInclude()
	case KEYWORD_FN:
		kind = decl.FUNCTION_KIND
		components[0], errors = p.parseFunction()
	case KEYWORD_PUB, KEYWORD_INPUT, KEYWORD_OUTPUT, KEYWORD_STATIC:
		kind = decl.MEMORY_KIND
		components[0], errors = p.parseInputOutputMemory()
	case KEYWORD_MEMORY:
		kind = decl.MEMORY_KIND
		components[0], errors = p.parseReadWriteMemory()
	case KEYWORD_TYPE:
		kind = decl.TYPE_ALIAS_KIND
		components[0], errors = p.parseTypeAlias()
	case EOF:
		return nil, p.syntaxErrors(lookahead, "unexpected end-of-file")
	default:
		return nil, p.syntaxErrors(lookahead, "unknown declaration")
	}
	//
	errors = append(errors, p.validateAnnotationKinds(annotations, kind)...)
	// Sanity check for unrecorable error
	if len(components) == 0 || components[0] == nil {
		return nil, errors
	}
	// Attach annotation names to the declaration
	components[0].SetAnnotations(p.tokenStrings(annotations))
	//
	return components, errors
}

// parseAnnotations parses zero or more leading annotations of the form "#[ident]"
// that precede a top-level declaration.  It returns the identifier tokens (for
// source-location-aware error reporting) and reports an error immediately if an
// annotation name is not found in decl.ANNOTATIONS.
func (p *Parser) parseAnnotations() ([]lex.Token, []source.SyntaxError) {
	var toks []lex.Token
	//
	for p.lookahead().Kind == HASH {
		if _, errs := p.expect(HASH); len(errs) > 0 {
			return nil, errs
		} else if _, errs := p.expect(LSQUARE); len(errs) > 0 {
			return nil, errs
		}
		// expect an identifier immediately after '#['
		tok, errs := p.expect(IDENTIFIER)
		if len(errs) > 0 {
			return nil, errs
		} else if _, errs := p.expect(RSQUARE); len(errs) > 0 {
			return nil, errs
		}
		// Validate that the annotation name is registered.
		name := p.string(tok)
		known := false

		for _, ann := range decl.ANNOTATIONS {
			if ann.Name() == name {
				known = true
				break
			}
		}

		if !known {
			return nil, p.syntaxErrors(tok, fmt.Sprintf("unknown annotation \"%s\"", name))
		}
		//
		toks = append(toks, tok)
	}
	//
	return toks, nil
}

// validateAnnotationKinds checks that every annotation token in toks is
// permitted on a declaration of the given kind.  The first violation produces
// a syntax error pointing at the offending annotation token.
func (p *Parser) validateAnnotationKinds(toks []lex.Token, kind decl.DeclarationKind) []source.SyntaxError {
	for _, tok := range toks {
		name := p.string(tok)

		for _, ann := range decl.ANNOTATIONS {
			if ann.Name() == name && !ann.Permits(kind) {
				return p.syntaxErrors(tok, "not permitted on "+kind.String()+" declaration")
			}
		}
	}

	return nil
}

// tokenStrings extracts the source text of each token as a string slice.
func (p *Parser) tokenStrings(toks []lex.Token) []string {
	names := make([]string, len(toks))
	for i, tok := range toks {
		names[i] = p.string(tok)
	}

	return names
}

// parseConstant parses a constants declation which must be of the form
//
//	const cdef (, cdef)*
//
// with cdef of the form
//
//	identifier : type = expr
func (p *Parser) parseConstant() ([]decl.Unresolved, []source.SyntaxError) {
	var (
		start    = p.index
		errs     []source.SyntaxError
		datatype Type
		name     string
		env      = EmptyEnvironment()
		consts   []decl.Unresolved
	)
	// Parse const declaration
	if _, errs := p.expect(KEYWORD_CONST); len(errs) > 0 {
		return nil, errs
	} else if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(COLON); len(errs) > 0 {
		return nil, errs
	} else if datatype, errs = p.parseType(); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(EQUALS); len(errs) > 0 {
		return nil, errs
	}
	// Save for source map
	end := p.index
	// So far, so good.
	constExpr, errs := p.parseExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}
	//
	component := decl.NewConstant[symbol.Unresolved](name, datatype, constExpr)
	p.srcmap.Put(component, p.spanOf(start, end-1))
	consts = append(consts, component)
	// Parse additional comma-separated constants on the same line.
	for p.match(COMMA) {
		start = p.index
		if name, errs = p.parseIdentifier(); len(errs) > 0 {
			return nil, errs
		} else if _, errs = p.expect(COLON); len(errs) > 0 {
			return nil, errs
		} else if datatype, errs = p.parseType(); len(errs) > 0 {
			return nil, errs
		} else if _, errs = p.expect(EQUALS); len(errs) > 0 {
			return nil, errs
		}

		end = p.index

		constExpr, errs = p.parseExpr(env)
		if len(errs) > 0 {
			return nil, errs
		}

		component = decl.NewConstant[symbol.Unresolved](name, datatype, constExpr)
		p.srcmap.Put(component, p.spanOf(start, end-1))
		consts = append(consts, component)
	}
	//
	return consts, nil
}

func (p *Parser) parseInclude() (decl.Unresolved, []source.SyntaxError) {
	// Parse include declaration
	if _, errs := p.expect(KEYWORD_INCLUDE); len(errs) > 0 {
		return nil, errs
	}
	//
	tok, errs := p.expect(STRING)
	//
	if len(errs) > 0 {
		return nil, errs
	}
	// Process string
	str := p.string(tok)
	// Construct include
	inc := decl.NewInclude[symbol.Unresolved](str[1 : len(str)-1])
	// Store for error reporting.
	p.srcmap.Put(inc, tok.Span)
	// Done
	return inc, errs
}

func (p *Parser) parseFunction() (decl.Unresolved, []source.SyntaxError) {
	var (
		start    = p.index
		env      = EmptyEnvironment()
		name     string
		code     []stmt.Unresolved
		errs     []source.SyntaxError
		returned bool
	)
	// Parse function declaration
	if _, errs = p.expect(KEYWORD_FN); len(errs) > 0 {
		return nil, errs
	}
	// Parse function name
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	// Check for any effects
	if p.match(LESS_THAN) {
		if errs = p.parseMemoryEffects(env); len(errs) > 0 {
			return nil, errs
		}
	}
	// Parse inputs
	if errs = p.parseArgsList(variable.PARAMETER, env); len(errs) > 0 {
		return nil, errs
	}
	// Parse optional '->'
	if p.match(RIGHTARROW) {
		// Parse returns
		if errs = p.parseArgsList(variable.RETURN, env); len(errs) > 0 {
			return nil, errs
		}
	}
	// Save for source map
	end := p.index
	// Parse start of block
	if returned, code, errs = p.parseStatementBlock(env, false); len(errs) > 0 {
		return nil, errs
	}
	// Sanity check for implicit or explicit return
	if !returned {
		stmt := &stmt.Return[symbol.Unresolved]{}
		// Implicit return
		code = append(code, stmt)
		// Associate return with span of final curly brace.
		p.srcmap.Put(stmt, p.previousToken().Span)
	}
	// Construct function
	fn := decl.NewFunction(name, env.Effects(), env.Variables(), code)
	//
	p.srcmap.Put(fn, p.spanOf(start, end-1))
	// Done
	return fn, nil
}

func (p *Parser) parseMemoryEffects(env Environment) []source.SyntaxError {
	var (
		arg   string
		errs  []source.SyntaxError
		first = true
	)
	// Parse entries until end brace
	for p.lookahead().Kind != GREATER_THAN {
		// look for ","
		if !first {
			if _, errs = p.expect(COMMA); len(errs) > 0 {
				return errs
			}
		}
		//
		first = false
		// save lookahead here so errors point at the name token
		lookahead := p.lookahead()
		// parse name first (new syntax: name:type)
		if arg, errs = p.parseIdentifier(); len(errs) > 0 {
			return errs
		} else if env.IsDeclared(arg) {
			return p.syntaxErrors(lookahead, "effect already declared")
		}
		// construct effect
		sym := symbol.NewUnresolved(arg, symbol.MEMORY_EFFECT, symbol.ANY_INPUTS)
		effect := &sym
		//
		p.srcmap.Put(effect, lookahead.Span)
		//
		env.DeclareEffect(effect)
	}
	// Advance past ">"
	p.match(GREATER_THAN)
	//
	return nil
}

// parseArgsList parses a list of the form (name :type, name' :type', ...)
func (p *Parser) parseArgsList(kind variable.Kind, env Environment) []source.SyntaxError {
	var (
		arg      string
		datatype Type
		errs     []source.SyntaxError
		first    = true
	)
	// Parse start of list
	if _, errs = p.expect(LBRACE); len(errs) > 0 {
		return errs
	}
	// Parse entries until end brace
	for p.lookahead().Kind != RBRACE {
		// look for ","
		if !first {
			if _, errs = p.expect(COMMA); len(errs) > 0 {
				return errs
			}
		}
		//
		first = false
		// save lookahead here so errors point at the name token
		lookahead := p.lookahead()
		// parse name first (new syntax: name:type)
		if arg, errs = p.parseIdentifier(); len(errs) > 0 {
			return errs
		} else if env.IsDeclared(arg) {
			return p.syntaxErrors(lookahead, "variable already declared")
		}
		// parse ':'
		if _, errs = p.expect(COLON); len(errs) > 0 {
			return errs
		}
		// parse type
		if datatype, errs = p.parseType(); len(errs) > 0 {
			return errs
		}
		//
		env.DeclareVariable(kind, arg, datatype)
	}
	// Advance past ")"
	p.match(RBRACE)
	//
	return nil
}

func (p *Parser) parseInputOutputMemory() (decl.Unresolved, []source.SyntaxError) {
	var (
		start   = p.index
		public  bool
		name    string
		errs    []source.SyntaxError
		address []VariableDescriptor
		data    []VariableDescriptor
		mem     decl.Unresolved
	)
	// Parse optional pub modifier; private by default
	if p.match(KEYWORD_PUB) {
		public = true
	}
	//
	lookahead := p.lookahead()
	// Validate keyword before consuming it
	switch lookahead.Kind {
	case KEYWORD_INPUT, KEYWORD_OUTPUT, KEYWORD_STATIC:
		p.index++
	default:
		return nil, p.syntaxErrors(lookahead, "unknown declaration")
	}
	// Parse the shared header: name(address) -> (data)
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	//
	if address, errs = p.parseMemoryArgsList(variable.PARAMETER); len(errs) > 0 {
		return nil, errs
	}
	//
	if _, errs = p.expect(RIGHTARROW); len(errs) > 0 {
		return nil, errs
	}
	//
	if data, errs = p.parseMemoryArgsList(variable.RETURN); len(errs) > 0 {
		return nil, errs
	}
	// Save for source map
	end := p.index
	// Construct the appropriate memory declaration
	switch lookahead.Kind {
	case KEYWORD_INPUT:
		mem = decl.NewReadOnlyMemory[symbol.Unresolved](public, name, address, data)
	case KEYWORD_OUTPUT:
		mem = decl.NewWriteOnceMemory[symbol.Unresolved](public, name, address, data)
	default: // KEYWORD_STATIC
		contents, errs := p.parseStaticInitialiser()
		if len(errs) > 0 {
			return nil, errs
		}
		//
		mem = decl.NewStaticMemory[symbol.Unresolved](public, name, address, data, contents)
	}
	//
	p.srcmap.Put(mem, p.spanOf(start, end-1))
	//
	return mem, nil
}

// parseStaticInitialiser parses a brace-enclosed comma-separated list of
// compile-time constant expressions: { expr, expr, ... }
func (p *Parser) parseStaticInitialiser() ([]expr.Unresolved, []source.SyntaxError) {
	var (
		contents []expr.Unresolved
		errs     []source.SyntaxError
		env      = EmptyEnvironment()
	)
	//
	if _, errs = p.expect(LCURLY); len(errs) > 0 {
		return nil, errs
	}
	//
	for p.lookahead().Kind != RCURLY {
		e, errs := p.parseExpr(env)
		if len(errs) > 0 {
			return nil, errs
		}

		contents = append(contents, e)
		// Consume comma separator; stop if next token is '}'
		if !p.match(COMMA) {
			break
		}
	}
	//
	rcurlyTok, errs := p.expect(RCURLY)
	if len(errs) > 0 {
		return nil, errs
	}
	// A static memory must have at least one entry.
	if len(contents) == 0 {
		return nil, p.syntaxErrors(rcurlyTok, "empty static memory")
	}
	//
	return contents, nil
}

func (p *Parser) parseReadWriteMemory() (decl.Unresolved, []source.SyntaxError) {
	var (
		start   = p.index
		name    string
		errs    []source.SyntaxError
		address []VariableDescriptor
		data    []VariableDescriptor
	)
	//
	if _, errs := p.expect(KEYWORD_MEMORY); len(errs) > 0 {
		return nil, errs
	}
	// Parse memory name first (function-style)
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	// Parse address args: (type param, ...)
	if address, errs = p.parseMemoryArgsList(variable.PARAMETER); len(errs) > 0 {
		return nil, errs
	}
	// Parse ->
	if _, errs = p.expect(RIGHTARROW); len(errs) > 0 {
		return nil, errs
	}
	// Parse data args: (type result, ...)
	if data, errs = p.parseMemoryArgsList(variable.RETURN); len(errs) > 0 {
		return nil, errs
	}
	// Done
	mem := decl.NewRandomAccessMemory[symbol.Unresolved](name, address, data)
	//
	p.srcmap.Put(mem, p.spanOf(start, p.index-1))
	//
	return mem, nil
}

func (p *Parser) parseTypeAlias() (decl.Unresolved, []source.SyntaxError) {
	var (
		start    = p.index
		errs     []source.SyntaxError
		datatype Type
		name     string
	)
	// Parse type declaration
	if _, errs := p.expect(KEYWORD_TYPE); len(errs) > 0 {
		return nil, errs
	} else if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(EQUALS); len(errs) > 0 {
		return nil, errs
	} else if datatype, errs = p.parseType(); len(errs) > 0 {
		return nil, errs
	}
	// Save for source map
	end := p.index
	component := decl.NewTypeAlias[symbol.Unresolved](name, datatype)
	//
	p.srcmap.Put(component, p.spanOf(start, end-1))
	//
	return component, errs
}

// parseMemoryArgsList parses a function-style typed parameter list for memory
// declarations: (type name, type name, ...).  Returns both the combined type
// (for the address/data bus) and the individual named descriptors.
func (p *Parser) parseMemoryArgsList(kind variable.Kind) ([]VariableDescriptor, []source.SyntaxError) {
	var (
		params []VariableDescriptor
		errs   []source.SyntaxError
	)
	if _, errs = p.expect(LBRACE); len(errs) > 0 {
		return nil, errs
	}

	for p.lookahead().Kind != RBRACE {
		var t Type

		if len(params) > 0 {
			if _, errs = p.expect(COMMA); len(errs) > 0 {
				return nil, errs
			}
		}

		var pname string
		if pname, errs = p.parseIdentifier(); len(errs) > 0 {
			return nil, errs
		}

		if _, errs = p.expect(COLON); len(errs) > 0 {
			return nil, errs
		}

		if t, errs = p.parseType(); len(errs) > 0 {
			return nil, errs
		}

		params = append(params, variable.New(kind, pname, t))
	}

	p.match(RBRACE)

	return params, nil
}

func (p *Parser) parseType() (Type, []source.SyntaxError) {
	var (
		name  string
		errs  []source.SyntaxError
		start = p.index
	)
	// Check for the native field element type "𝔽"
	if p.match(FIELD_ELEMENT) {
		return data.NewFieldElement[symbol.Unresolved](), nil
	}
	// Check for arrays
	isArray := p.match(LSQUARE)
	//
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	// Parse to check if bitwidth is present
	bw, bwErr := strconv.Atoi(name[1:])
	switch {
	case isArray && errs == nil:
		// parse array type
		var arrayType Type

		switch {
		case strings.HasPrefix(name, "u") && bwErr == nil:
			arrayType = data.NewUnsignedInt[symbol.Unresolved](uint(bw), false)
		default:
			alias := symbol.NewUnresolved(name, symbol.TYPE_ALIAS, 0)
			arrayType = data.NewAlias[symbol.Unresolved](alias)
			p.srcmap.Put(arrayType, p.spanOf(start+1, p.index-1))
		}
		//
		lookahead := p.lookahead()
		p.srcmap.Put(lookahead, p.spanOf(p.index, p.index))

		if _, errs := p.expect(SEMICOLON); len(errs) != 0 {
			return nil, p.srcmap.SyntaxErrors(lookahead, "expected semicolon to define array size")
		}
		//
		lookahead = p.lookahead()
		p.srcmap.Put(lookahead, p.spanOf(p.index, p.index))

		size, errors := p.parseArraySize(lookahead)
		if len(errors) > 0 {
			return nil, errors
		}
		//
		if !p.match(RSQUARE) {
			return nil, p.srcmap.SyntaxErrors(lookahead, "expected closing bracket")
		}
		//
		fa := data.NewFixedArray[symbol.Unresolved](arrayType, size)
		p.srcmap.Put(fa, p.spanOf(start, p.index-1))

		return fa, nil
	case strings.HasPrefix(name, "u") && bwErr == nil:
		//
		return data.NewUnsignedInt[symbol.Unresolved](uint(bw), false), nil
	// we assume that if not a fundamental type, it is an alias
	default:
		alias := data.NewAlias[symbol.Unresolved](symbol.NewUnresolved(name, symbol.TYPE_ALIAS, 0))
		//
		p.srcmap.Put(alias, p.spanOf(start, p.index-1))
		//
		return alias, nil
	}
}

func (p *Parser) parseArraySize(lookahead lex.Token,
) (size util.Union[uint, symbol.Unresolved], errors []source.SyntaxError) {
	//
	switch lookahead.Kind {
	case NUMBER:
		p.match(NUMBER)

		bound, nbErrs := p.number(lookahead)
		base := p.baserOfNumber(lookahead)

		if len(nbErrs) != 0 || base != 10 {
			return size, p.srcmap.SyntaxErrors(lookahead, "array size is not a number in base 10")
		}

		if bound.BitLen() == 0 {
			return size, p.srcmap.SyntaxErrors(lookahead, "arrays are restricted to non zero constant value")
		}

		return util.Union1[uint, symbol.Unresolved](uint(bound.Uint64())), nil
	case IDENTIFIER:
		name, idErrs := p.parseIdentifier()
		if len(idErrs) != 0 {
			return size, p.srcmap.SyntaxErrors(lookahead, "array size is not a number or a constant")
		}

		return util.Union2[uint](symbol.NewUnresolved(name, symbol.CONSTANT, 0)), nil
	default:
		return size, p.srcmap.SyntaxErrors(lookahead, "array size is not a number or a constant")
	}
}

func (p *Parser) parseStatementBlock(env Environment, inLoop bool,
) (bool, []stmt.Unresolved, []source.SyntaxError) {
	//
	var (
		errs     []source.SyntaxError
		insns    []stmt.Unresolved
		returned bool
	)
	// Clone environment.  This is to ensure that variables declared in this
	// block do not clash with those of the same name declared elsewhere.
	env = env.Clone(inLoop)
	// Parse start of block
	if _, errs = p.expect(LCURLY); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse instructions until end of block
	for p.lookahead().Kind != RCURLY {
		var (
			ith []stmt.Unresolved
			ret bool
		)
		//
		if ret, ith, errs = p.parseStatement(env); len(errs) > 0 {
			return false, nil, errs
		}
		//
		returned = returned || ret
		//
		insns = append(insns, ith...)
	}
	// Advance past "}"
	p.match(RCURLY)
	//
	return returned, insns, errs
}

func (p *Parser) parseStatement(env Environment,
) (bool, []stmt.Unresolved, []source.SyntaxError) {
	//
	var (
		// Save current position for backtracking
		start    = p.index
		errs     []source.SyntaxError
		insns    []stmt.Unresolved
		insn     stmt.Unresolved
		returned bool
		cost     string
	)
	//
	if cost, errs = p.parseStatementCostAnnotation(); len(errs) > 0 {
		return false, nil, errs
	}
	//
	lookahead := p.lookahead()
	//
	switch lookahead.Kind {
	case KEYWORD_BREAK:
		returned, insn, errs = p.parseBreak(env)
	case KEYWORD_CONTINUE:
		returned, insn, errs = p.parseContinue(env)
	case KEYWORD_FAIL:
		returned, insn, errs = p.parseFail(env)
	case KEYWORD_FOR:
		returned, insn, errs = p.parseFor(env)
	case KEYWORD_IF:
		returned, insn, errs = p.parseIfElse(env)
	case KEYWORD_SWITCH:
		returned, insn, errs = p.parseSwitch(env)
	case KEYWORD_PRINTF:
		returned, insn, errs = p.parsePrintf(env)
	case KEYWORD_RETURN:
		returned, insn, errs = p.parseReturn()
	case KEYWORD_WHILE:
		returned, insn, errs = p.parseWhile(env)
	case KEYWORD_VAR:
		insns, errs = p.parseVar(env)
	case IDENTIFIER:
		// Detect a bare function call statement: name(...) with no assignment.
		if p.index+1 < len(p.tokens) && p.tokens[p.index+1].Kind == LBRACE {
			insn, errs = p.parseCallStatement(env)
		} else {
			insn, errs = p.parseAssignment(env)
		}
	default:
		insn, errs = p.parseAssignment(env)
	}
	// Include unit instruction (if applicable)
	if insn != nil {
		insns = append(insns, insn)
	}
	// Attach compile-time cost label to all statements produced by this source
	// statement. This is metadata only; the wrapper delegates statement
	// semantics to its body.
	if cost != "" {
		for i, insn := range insns {
			insns[i] = &stmt.Cost[symbol.Unresolved]{Label: cost, Body: insn}
		}
	}
	// Record source mapping
	for _, insn := range insns {
		// Check whether instruction already added to source map.  This can
		// arise with recursive calls to parseStatement() (e.g. for blocks).
		if !p.srcmap.Has(insn) {
			p.srcmap.Put(insn, p.spanOf(start, p.index-1))
		}
	}
	//
	return returned, insns, errs
}

func (p *Parser) parseStatementCostAnnotation() (string, []source.SyntaxError) {
	if p.lookahead().Kind != HASH {
		return "", nil
	}
	//
	if _, errs := p.expect(HASH); len(errs) > 0 {
		return "", errs
	} else if _, errs := p.expect(LSQUARE); len(errs) > 0 {
		return "", errs
	}
	//
	kind, errs := p.expect(IDENTIFIER)
	if len(errs) > 0 {
		return "", errs
	} else if p.string(kind) != "cost" {
		return "", p.syntaxErrors(kind, "unknown statement annotation")
	} else if _, errs := p.expect(COLON); len(errs) > 0 {
		return "", errs
	}
	//
	label, errs := p.expect(IDENTIFIER)
	if len(errs) > 0 {
		return "", errs
	} else if _, errs := p.expect(RSQUARE); len(errs) > 0 {
		return "", errs
	}
	//
	return p.string(label), nil
}

func (p *Parser) parseAssignment(env Environment) (stmt.Unresolved, []source.SyntaxError) {
	var (
		lhs  []LVal
		rhs  Expr
		errs []source.SyntaxError
	)
	// parse left-hand side
	if lhs, errs = p.parseLVals(env); len(errs) > 0 {
		return nil, errs
	}
	// Parse '='
	if _, errs = p.expect(EQUALS); len(errs) > 0 {
		return nil, errs
	}
	// Parse right-hand side
	if rhs, errs = p.parseTupleExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// Done
	return &stmt.Assign[symbol.Unresolved]{Targets: lhs, Source: rhs}, nil
}

func (p *Parser) parseCallStatement(env Environment) (stmt.Unresolved, []source.SyntaxError) {
	// Parse call as a general expression, since this ensures source mapping is
	// handled.  This means, however, that we need to check afterwards that we
	// actually got a call expression rather than a general expression.
	call, errs := p.parseLogicalExpr(env)
	//
	if len(errs) > 0 {
		return nil, errs
	} else if ea, ok := call.(*expr.ExternAccess[symbol.Unresolved]); ok && ea.Name.Kind == symbol.FUNCTION {
		// Yes, its a function call
		return &stmt.Assign[symbol.Unresolved]{Targets: nil, Source: call}, nil
	}
	// No, its some other kind of expression.
	return nil, p.srcmap.SyntaxErrors(call, "expression unused")
}

// parseSwitch parses a switch statement as a whole.
func (p *Parser) parseSwitch(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	var (
		errs         []source.SyntaxError
		discriminant Expr
	)
	// parse the 'switch' keyword
	if _, errs = p.expect(KEYWORD_SWITCH); len(errs) > 0 {
		return false, nil, errs
	}
	// parse discriminant of the switch statement
	if discriminant, errs = p.parseExpr(env); len(errs) > 0 {
		return false, nil, errs
	}

	var (
		returned bool
		branches []stmt.SwitchBranch[symbol.Unresolved]
	)

	returned, branches, errs = p.parseSwitchBody(env)

	node := &stmt.Switch[symbol.Unresolved]{
		Discriminant: discriminant,
		Branches:     branches,
	}

	return returned, node, errs
}

// parseSwitchBody parses the 'body' of a switch statement:
//
//	switch (discriminant) {
//		...	// the 'switch body'
//	}
//
// Note: parseSwitchBody currently doesn't
//   - scan for / flag label duplication
//   - perform exhaustivity analysis
func (p *Parser) parseSwitchBody(env Environment) (returned bool,
	branches []stmt.SwitchBranch[symbol.Unresolved],
	errs []source.SyntaxError) {
	// we expect an opening curly brace
	if _, errs = p.expect(LCURLY); len(errs) > 0 {
		return false, nil, errs
	}

	var (
		branch                    stmt.SwitchBranch[symbol.Unresolved]
		branchReturns             bool
		everyBranchReturns        = true
		bodyContainsDefaultBranch = false
	)

	for p.lookahead().Kind != RCURLY {
		// 'lookahead' remembers (what is expected to be) the upcoming case or default
		// keyword
		//
		// Note: parseSwitchBranch below expects one of these keywords; if that
		// expectation isn't met 'lookahead' won't get used
		lookahead := p.lookahead()
		if branchReturns, branch, errs = p.parseSwitchBranch(env); len(errs) > 0 {
			return false, nil, errs
		}

		// we flag the presence of multiple default cases
		if branch.IsDefault && bodyContainsDefaultBranch {
			return false, nil, p.syntaxErrors(lookahead, "multiple default cases in switch statement")
		}

		if !branchReturns {
			everyBranchReturns = false
		}

		if branch.IsDefault {
			bodyContainsDefaultBranch = true
		}

		branches = append(branches, branch)
	}

	// we expect a closing curly brace
	p.expect(RCURLY)

	// for a switch statement to 'return' as a whole, and in the absence of
	// exhaustivity analysis, it must hold that
	//	- every branch 'returns'
	//	- its body contains a default branch which similarly 'returns'
	//
	// Note: the required presence of a default branch prevents switch
	// statements with empty body from 'returning'
	returned = bodyContainsDefaultBranch && everyBranchReturns

	return
}

func (p *Parser) parseSwitchBranch(env Environment) (
	returns bool,
	branch stmt.SwitchBranch[symbol.Unresolved],
	errs []source.SyntaxError) {
	var (
		isCase    bool
		isDefault bool
	)

	isCase = p.follows(KEYWORD_CASE)
	isDefault = p.follows(KEYWORD_DEFAULT)

	if !(isCase || isDefault) {
		return false, branch, p.syntaxErrors(p.lookahead(), "'case' or 'default' keyword expected")
	}

	var (
		labels []expr.Expr[symbol.Unresolved]
		body   []stmt.Stmt[symbol.Unresolved]
	)

	if isCase {
		if returns, labels, body, errs = p.parseSwitchCase(env); len(errs) > 0 {
			return
		}
	} else {
		if returns, body, errs = p.parseSwitchDefault(env); len(errs) > 0 {
			return
		}
	}

	branch.IsDefault = isDefault
	branch.Labels = labels
	branch.Body = body

	return
}

// parseSwitchCase deals with the beginning of a switch case, either
//
//	case CASE_1, 42: { ... }
//	default: { ... }
func (p *Parser) parseSwitchCase(env Environment) (
	returns bool,
	labels []expr.Expr[symbol.Unresolved],
	body []stmt.Stmt[symbol.Unresolved],
	errs []source.SyntaxError) {
	if _, errs = p.expect(KEYWORD_CASE); len(errs) > 0 {
		return false, labels, body, p.syntaxErrors(p.previousToken(), "expected 'case' keyword")
	}

	labels, errs = p.parseExprList(COLON, env)
	if len(errs) > 0 {
		return
	}

	if len(labels) == 0 {
		return false, labels, body, p.syntaxErrors(p.previousToken(), "empty switch case list")
	}

	// we reject any label that isn't
	//	- a named constant or
	//	- a numerical constant
	for _, label := range labels {
		switch t := label.(type) {
		case *expr.ExternAccess[symbol.Unresolved]:
			{
				switch t.Name.Kind {
				// the 'named constant' case
				case symbol.CONSTANT:
					continue
				default:
					return false, nil, nil, p.srcmap.SyntaxErrors(label,
						"labels in a switch statement must be constants (named or literal)")
				}
			}
		// the 'numerical constant' case
		case *expr.Const[symbol.Unresolved]:
			{
				continue
			}
		default:
			return false, nil, nil, p.syntaxErrors(p.previousToken(),
				"labels in a switch statement must be constants (named or literal)")
		}
	}

	returns, body, errs = p.parseStatementBlock(env, env.InLoop())
	if len(errs) > 0 {
		return false, nil, nil, errs
	}

	return
}

func (p *Parser) parseSwitchDefault(env Environment) (
	returns bool,
	body []stmt.Stmt[symbol.Unresolved],
	errs []source.SyntaxError) {
	if _, errs = p.expect(KEYWORD_DEFAULT); len(errs) > 0 {
		return
	}

	if _, errs = p.expect(COLON); len(errs) > 0 {
		return
	}

	if returns, body, errs = p.parseStatementBlock(env, env.InLoop()); len(errs) > 0 {
		return
	}

	return
}

func (p *Parser) parseIfElse(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	var (
		errs        []source.SyntaxError
		trueBranch  []stmt.Unresolved
		falseBranch []stmt.Unresolved
		trueRet     bool
		falseRet    bool
		condExpr    Expr
	)
	// Match if
	if _, errs = p.expect(KEYWORD_IF); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse and validate condition expression
	if condExpr, errs = p.parseConditionExpr(env); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse true branch
	if trueRet, trueBranch, errs = p.parseStatementBlock(env, env.InLoop()); len(errs) > 0 {
		return false, nil, errs
	}
	// Check for "else"
	if p.lookahead().Kind == KEYWORD_ELSE {
		_, _ = p.expect(KEYWORD_ELSE)
		// parse false branch (either a block or an else-if chain)
		if p.lookahead().Kind == KEYWORD_IF {
			var falseInsn stmt.Unresolved

			falseRet, falseInsn, errs = p.parseIfElse(env)
			if len(errs) > 0 {
				return false, nil, errs
			}

			falseBranch = []stmt.Unresolved{falseInsn}
		} else {
			falseRet, falseBranch, errs = p.parseStatementBlock(env, env.InLoop())
			if len(errs) > 0 {
				return false, nil, errs
			}
		}
	}

	node := &stmt.IfElse[symbol.Unresolved]{
		Cond:        condExpr,
		TrueBranch:  trueBranch,
		FalseBranch: falseBranch,
	}
	// An if/else definitely returns only when both branches do
	return trueRet && falseRet && len(falseBranch) > 0, node, nil
}

func (p *Parser) parseWhile(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	var (
		errs     []source.SyntaxError
		body     []stmt.Unresolved
		condExpr Expr
	)
	// Match while
	if _, errs = p.expect(KEYWORD_WHILE); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse and validate condition expression
	if condExpr, errs = p.parseConditionExpr(env); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse body block (loop body enables break/continue)
	if _, body, errs = p.parseStatementBlock(env, true); len(errs) > 0 {
		return false, nil, errs
	}

	node := &stmt.While[symbol.Unresolved]{Cond: condExpr, Body: body}
	// A while loop never guarantees a return
	return false, node, nil
}

func (p *Parser) parseFor(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	var (
		errs     []source.SyntaxError
		init     stmt.Unresolved
		post     stmt.Unresolved
		body     []stmt.Unresolved
		condExpr Expr
	)
	// Match 'for'
	if _, errs = p.expect(KEYWORD_FOR); len(errs) > 0 {
		return false, nil, errs
	}
	// Clone the environment so the loop init variable is scoped to this loop only,
	// not the enclosing function scope.  This allows two loops to reuse the same
	// variable name (e.g. for i:u8=0; ...) without a "variable already declared" error.
	env = env.Clone(env.InLoop())
	// Parse init: either an inline variable declaration (name:type = expr) or a plain assignment
	if init, errs = p.parseForInit(env); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse ';'
	if _, errs = p.expect(SEMICOLON); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse and validate condition expression
	if condExpr, errs = p.parseConditionExpr(env); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse ';'
	if _, errs = p.expect(SEMICOLON); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse post assignment
	if post, errs = p.parseAssignment(env); len(errs) > 0 {
		return false, nil, errs
	}
	// Parse body (loop body enables break/continue)
	if _, body, errs = p.parseStatementBlock(env, true); len(errs) > 0 {
		return false, nil, errs
	}

	node := &stmt.For[symbol.Unresolved]{Init: init, Cond: condExpr, Post: post, Body: body}
	// A for loop never guarantees a return
	return false, node, nil
}

// parseForInit parses the initialiser of a for loop.  It accepts either an
// inline variable declaration of the form "name:type = expr" or a plain
// assignment to an already-declared variable.
func (p *Parser) parseForInit(env Environment) (stmt.Unresolved, []source.SyntaxError) {
	// Detect "name:type = expr" by peeking one token ahead.
	if p.index+1 < len(p.tokens) &&
		p.tokens[p.index].Kind == IDENTIFIER &&
		p.tokens[p.index+1].Kind == COLON {
		// Inline variable declaration
		lookahead := p.lookahead()

		name, errs := p.parseIdentifier()
		if len(errs) > 0 {
			return nil, errs
		} else if env.IsDeclared(name) {
			return nil, p.syntaxErrors(lookahead, "variable already declared")
		}

		if _, errs = p.expect(COLON); len(errs) > 0 {
			return nil, errs
		}

		dt, errs := p.parseType()
		if len(errs) > 0 {
			return nil, errs
		}

		if _, errs = p.expect(EQUALS); len(errs) > 0 {
			return nil, errs
		}

		// Parse the initialiser before declaring the variable so that names
		// resolve to the outer scope (same rationale as parseVarDecls).
		rhs, errs := p.parseExpr(env)
		if len(errs) > 0 {
			return nil, errs
		}

		env.DeclareVariable(variable.LOCAL, name, dt)
		id := env.LookupVariable(name)

		return &stmt.VarDecl[symbol.Unresolved]{
			Variables: []variable.Id{id},
			Init:      util.Some[Expr](rhs),
		}, nil
	}
	// Fall back to a plain assignment to an already-declared variable.
	return p.parseAssignment(env)
}

func (p *Parser) parseReturn() (bool, stmt.Unresolved, []source.SyntaxError) {
	if _, errs := p.expect(KEYWORD_RETURN); len(errs) > 0 {
		return true, nil, errs
	}
	//
	return true, &stmt.Return[symbol.Unresolved]{}, nil
}

func (p *Parser) parseFail(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	if _, errs := p.expect(KEYWORD_FAIL); len(errs) > 0 {
		return true, nil, errs
	}
	// An error message is optional; without one, fail produces a bare abort.
	if p.lookahead().Kind != STRING {
		return true, &stmt.Fail[symbol.Unresolved]{}, nil
	}
	//
	chunks, args, errs := p.parseFormattedString(env)
	if len(errs) > 0 {
		return true, nil, errs
	}
	//
	return true, &stmt.Fail[symbol.Unresolved]{Chunks: chunks, Arguments: args}, nil
}

func (p *Parser) parsePrintf(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	if _, errs := p.expect(KEYWORD_PRINTF); len(errs) > 0 {
		return false, nil, errs
	}
	//
	chunks, args, errs := p.parseFormattedString(env)
	if len(errs) > 0 {
		return false, nil, errs
	}
	//
	return false, &stmt.Printf[symbol.Unresolved]{Chunks: chunks, Arguments: args}, nil
}

// parseFormattedString parses a printf-style string literal followed by an
// optional comma-separated list of expression arguments, returning the
// resulting chunks and arguments.  An error is reported if the count of format
// specifiers does not match the count of supplied arguments.
func (p *Parser) parseFormattedString(env Environment) ([]stmt.FormattedChunk, []Expr, []source.SyntaxError) {
	token, errs := p.expect(STRING)
	if len(errs) > 0 {
		return nil, nil, errs
	}
	// Process string
	var (
		str    = p.string(token)
		runes  = []rune(str[1 : len(str)-1])
		nrunes []rune
		chunks []stmt.FormattedChunk
		args   []Expr
		nargs  int
	)
	//
	for i := 0; i < len(runes); {
		switch runes[i] {
		case '%':
			var (
				f  zkc_util.Format
				ok bool
			)
			if i, f, ok = parseFormatting(i+1, runes); !ok {
				return nil, nil, p.syntaxErrors(token, "invalid formatting")
			}
			//
			nargs++
			// append new chunkformat
			chunks = append(chunks, stmt.FormattedChunk{Text: string(nrunes), Format: f})
			// reset to build next chunk
			nrunes = nil
		case '\\':
			if i+1 == len(runes) {
				return nil, nil, p.syntaxErrors(token, "invalid escape")
			}
			// Attempt to parse escape character
			c, ok := escapeCharacter(runes[i+1])
			//
			if !ok {
				return nil, nil, p.syntaxErrors(token, "invalid escape")
			}
			// Continue
			nrunes = append(nrunes, c)
			i = i + 2
		default:
			nrunes = append(nrunes, runes[i])
			i = i + 1
		}
	}
	//
	if len(nrunes) > 0 {
		chunks = append(chunks, stmt.FormattedChunk{Text: string(nrunes), Format: zkc_util.EMPTY_FORMAT})
	}
	// parse expression arguments
	for p.match(COMMA) {
		arg, errs := p.parseLogicalExpr(env)
		//
		if len(errs) > 0 {
			return nil, nil, errs
		}
		//
		args = append(args, arg)
	}
	// Sanity check for matching arguments / chunks
	if len(args) > nargs {
		return nil, nil, p.srcmap.SyntaxErrors(args[nargs], "too many arguments")
	} else if len(args) > 0 && len(args) < nargs {
		n := len(args) - 1
		//
		return nil, nil, p.srcmap.SyntaxErrors(args[n], "insufficient arguments")
	} else if len(args) < nargs {
		return nil, nil, p.syntaxErrors(token, "insufficient arguments")
	}
	//
	return chunks, args, nil
}

func parseFormatting(index int, runes []rune) (int, zkc_util.Format, bool) {
	var (
		zeroPad bool
		width   uint
	)
	// Optional '0' flag selects zero padding.
	if index < len(runes) && runes[index] == '0' {
		zeroPad = true
		index++
	}
	// Optional decimal width.
	for index < len(runes) && runes[index] >= '0' && runes[index] <= '9' {
		width = width*10 + uint(runes[index]-'0')
		index++
	}
	//
	if index >= len(runes) {
		return 0, zkc_util.EMPTY_FORMAT, false
	}
	//
	var format zkc_util.Format
	//
	switch runes[index] {
	case 'd':
		format = zkc_util.DecimalFormat()
	case 'x':
		format = zkc_util.HexFormat()
	case 'b':
		format = zkc_util.BinFormat()
	case 'c':
		// %c renders a single u8 as a character; padding/zero-pad
		// flags don't apply and we reject them rather than silently
		// ignoring.
		if width > 0 || zeroPad {
			return 0, zkc_util.EMPTY_FORMAT, false
		}
		//
		return index + 1, zkc_util.CharFormat(), true
	default:
		return 0, zkc_util.EMPTY_FORMAT, false
	}
	//
	format.Width = width
	format.ZeroPad = zeroPad
	//
	return index + 1, format, true
}

func escapeCharacter(ch rune) (rune, bool) {
	switch ch {
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case '\\':
		return '\\', true
	}
	//
	return ' ', false
}

func (p *Parser) parseBreak(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	tok, errs := p.expect(KEYWORD_BREAK)
	if len(errs) > 0 {
		return true, nil, errs
	}

	if !env.InLoop() {
		return true, nil, p.syntaxErrors(tok, "break outside loop")
	}

	return true, &stmt.Break[symbol.Unresolved]{}, nil
}

func (p *Parser) parseContinue(env Environment) (bool, stmt.Unresolved, []source.SyntaxError) {
	tok, errs := p.expect(KEYWORD_CONTINUE)
	if len(errs) > 0 {
		return true, nil, errs
	}

	if !env.InLoop() {
		return true, nil, p.syntaxErrors(tok, "continue outside loop")
	}

	return true, &stmt.Continue[symbol.Unresolved]{}, nil
}

// parseVar parses a variable declaration i.e. one of
// var name_1:type_1, name_2:type_2, ..., name_n:type_n
// var name:type = <expr>
func (p *Parser) parseVar(env Environment) ([]stmt.Unresolved, []source.SyntaxError) {
	var (
		errs  []source.SyntaxError
		names []string
		types []Type
	)
	// Consume 'var' keyword
	if _, errs = p.expect(KEYWORD_VAR); len(errs) > 0 {
		return nil, errs
	}
	// Parse one or more name:type pairs (comma-separated)
	for len(names) == 0 || p.match(COMMA) {
		// Store lookahead for error reporting
		lookahead := p.lookahead()
		//
		name, errs := p.parseIdentifier()
		//
		if len(errs) > 0 {
			return nil, errs
		} else if env.IsDeclared(name) {
			return nil, p.syntaxErrors(lookahead, "variable already declared")
		}
		// Parse ':'
		if _, errs = p.expect(COLON); len(errs) > 0 {
			return nil, errs
		}
		// Parse type
		dt, errs := p.parseType()
		if len(errs) > 0 {
			return nil, errs
		}
		//
		names = append(names, name)
		types = append(types, dt)
	}
	// Check for optional initialiser
	if !p.match(EQUALS) {
		// No initialiser — declare all variables now.
		varIds := make([]variable.Id, len(names))
		for i, name := range names {
			env.DeclareVariable(variable.LOCAL, name, types[i])
			varIds[i] = env.LookupVariable(name)
		}

		insn := &stmt.VarDecl[symbol.Unresolved]{
			Variables: varIds,
			Init:      util.None[Expr](),
		}

		return []stmt.Unresolved{insn}, nil
	}
	// Initialisers are only supported for single-variable declarations.
	if len(names) > 1 {
		return nil, p.syntaxErrors(p.lookahead(), "initialiser requires single variable declaration")
	}
	// Parse the initialiser expression BEFORE declaring the variable so that
	// names in the initialiser resolve to the outer scope (e.g. a memory with
	// the same name) rather than the not-yet-assigned local.
	rhs, errs := p.parseExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}
	// Now declare the variable.
	env.DeclareVariable(variable.LOCAL, names[0], types[0])
	varId := env.LookupVariable(names[0])
	// Build the variable declaration with initialiser
	insn := &stmt.VarDecl[symbol.Unresolved]{
		Variables: []variable.Id{varId},
		Init:      util.Some[Expr](rhs),
	}
	//
	return []stmt.Unresolved{insn}, nil
}

// parseConditionExpr parses an expression and validates it is a valid condition
// (i.e. a comparison, logical-and, logical-or, or logical-not).
func (p *Parser) parseConditionExpr(env Environment) (Expr, []source.SyntaxError) {
	ex, errs := p.parseLogicalExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}

	if !isValidCondition(ex) {
		return nil, p.srcmap.SyntaxErrors(ex, "invalid condition")
	}

	return ex, nil
}

// parseExpr parses either a ternary expression (cond ? ifTrue : ifFalse)
// where cond is a comparison expression, or a plain arithmetic expression. It is
// the top-level expression parser for all value positions (assignments, var
// declarations, function arguments, etc.).
func (p *Parser) parseExpr(env Environment) (Expr, []source.SyntaxError) {
	start := p.index

	// First parse a full expression, which may already be a comparison expression.
	ex, errs := p.parseLogicalExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}

	// If the next token is not '?', this is just a plain expression.
	if !p.follows(QMARK) {
		return ex, nil
	}

	// This should eventually be deprecated
	if !isValidCondition(ex) {
		return nil, p.srcmap.SyntaxErrors(ex, "invalid condition")
	}

	if _, errs := p.expect(QMARK); len(errs) > 0 {
		return nil, errs
	}

	ifTrue, errs := p.parseExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}

	if _, errs := p.expect(COLON); len(errs) > 0 {
		return nil, errs
	}

	ifFalse, errs := p.parseExpr(env)
	if len(errs) > 0 {
		return nil, errs
	}

	result := expr.NewTernary[symbol.Unresolved](ex, ifTrue, ifFalse)
	p.srcmap.Put(result, p.spanOf(start, p.index-1))

	return result, nil
}

func (p *Parser) parseLogicalExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		start     = p.index
		arg, errs = p.parseArithExpr(env)
		args      = []Expr{arg}
		tmp       Expr
	)
	// initialise lookahead
	kind := p.lookahead().Kind
	//
	for len(errs) == 0 && p.follows(LOGICAL_BINOPS...) {
		// Sanity check
		if !p.follows(kind) {
			return tmp, p.syntaxErrors(p.lookahead(), "braces required")
		}
		// Consume connective
		p.expect(p.lookahead().Kind)
		//
		tmp, errs = p.parseArithExpr(env)
		// Accumulate arguments
		args = append(args, tmp)
	}
	//
	switch {
	case len(errs) != 0:
		return nil, errs
	case len(args) == 1:
		return arg, nil
	case kind == LOGICAL_AND:
		arg = expr.NewLogicalAnd(args...)
	case kind == LOGICAL_OR:
		arg = expr.NewLogicalOr(args...)
	}
	//
	p.srcmap.Put(arg, p.spanOf(start, p.index-1))
	//
	return arg, errs
}

func (p *Parser) parseArithExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		start     = p.index
		arg, errs = p.parseConcatExpr(env)
		args      = []Expr{arg}
		tmp       Expr
		binary    bool
	)
	// initialise lookahead
	kind := p.lookahead().Kind
	//
	for len(errs) == 0 && p.follows(BINOPS...) {
		// Sanity check
		if !p.follows(kind) {
			return tmp, p.syntaxErrors(p.lookahead(), "braces required")
		}
		// Consume connective
		p.expect(p.lookahead().Kind)
		//
		tmp, errs = p.parseConcatExpr(env)
		// Accumulate arguments
		args = append(args, tmp)
	}
	//
	switch {
	case len(errs) != 0:
		return arg, errs
	case len(args) == 1:
		return arg, nil
	case kind == ADD:
		arg = expr.NewAdd(args...)
	case kind == BITWISE_AND:
		arg = expr.NewBitwiseAnd(args...)
	case kind == BITWISE_OR:
		arg = expr.NewBitwiseOr(args...)
	case kind == BITWISE_XOR:
		arg = expr.NewXor(args...)
	case kind == BITWISE_SHL:
		arg = expr.NewShl(args...)
	case kind == BITWISE_SHR:
		arg = expr.NewShr(args...)
	case kind == MUL:
		arg = expr.NewMul(args...)
	case kind == DIV:
		arg = expr.NewDiv(args...)
	case kind == REM:
		arg = expr.NewRem(args...)
	case kind == SUB:
		arg = expr.NewSub(args...)
	case kind == EQUALS_EQUALS:
		binary = true
		arg = expr.NewCmp(expr.EQ, args[0], args[1])
	case kind == NOT_EQUALS:
		binary = true
		arg = expr.NewCmp(expr.NEQ, args[0], args[1])
	case kind == LESS_THAN:
		binary = true
		arg = expr.NewCmp(expr.LT, args[0], args[1])
	case kind == LESS_THAN_EQUALS:
		binary = true
		arg = expr.NewCmp(expr.LTEQ, args[0], args[1])
	case kind == GREATER_THAN_EQUALS:
		binary = true
		arg = expr.NewCmp(expr.GTEQ, args[0], args[1])
	case kind == GREATER_THAN:
		binary = true
		arg = expr.NewCmp(expr.GT, args[0], args[1])
	}
	//
	p.srcmap.Put(arg, p.spanOf(start, p.index-1))
	//
	if binary && len(args) != 2 {
		return nil, p.srcmap.SyntaxErrors(arg, "invalid binary expression")
	}
	//
	return arg, nil
}

func (p *Parser) parseConcatExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		exprs = make([]Expr, 1)
		errs  []source.SyntaxError
		start = p.index
	)
	// Parse initial expression
	exprs[0], errs = p.parseUnitExpr(env)
	// Check for trailing concatenation
	for len(errs) == 0 && p.match(COLONCOLON) {
		var expr Expr
		//
		expr, errs = p.parseUnitExpr(env)
		exprs = append(exprs, expr)
	}
	//
	if len(errs) > 0 {
		return nil, errs
	} else if len(exprs) == 1 {
		return exprs[0], nil
	}
	// Bitwise concatenation
	arg := expr.NewConcat(exprs...)
	// Record span for this new expression
	p.srcmap.Put(arg, p.spanOf(start, p.index-1))
	//
	return arg, nil
}

func (p *Parser) parseUnitExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		lookahead = p.lookahead()
		errors    []source.SyntaxError
		start     = p.index
		nexpr     Expr
	)

	switch lookahead.Kind {
	case IDENTIFIER:
		nexpr, errors = p.parseAccessExpr(env)
	case NUMBER:
		var val big.Int
		//
		p.match(NUMBER)
		//
		val, errors = p.number(lookahead)
		base := p.baserOfNumber(lookahead)
		//
		nexpr = expr.NewUntypedConstant[symbol.Unresolved](val, base)
	case BITWISE_NOT:
		p.match(BITWISE_NOT)

		var operand Expr

		operand, errors = p.parseUnitExpr(env)
		if len(errors) == 0 {
			nexpr = expr.NewBitwiseNot(operand)
		}
	case LOGICAL_NOT:
		p.match(LOGICAL_NOT)

		var operand Expr

		operand, errors = p.parseUnitExpr(env)
		if len(errors) == 0 {
			nexpr = expr.NewLogicalNot(operand)
		}
	case LBRACE:
		p.match(LBRACE)
		nexpr, errors = p.parseTupleExpr(env)
		//
		if len(errors) == 0 && !p.match(RBRACE) {
			return nil, p.syntaxErrors(lookahead, "expected )")
		}
	// Fall through to check for trailing `as` cast.
	default:
		return nil, p.syntaxErrors(lookahead, "unexpected token")
	}
	//
	if len(errors) == 0 && !p.srcmap.Has(nexpr) {
		p.srcmap.Put(nexpr, p.spanOf(start, p.index-1))
	}
	//
	if len(errors) == 0 && p.match(KEYWORD_AS) {
		var castType Type
		//
		if castType, errors = p.parseType(); len(errors) == 0 {
			cast := expr.NewCast(nexpr, castType)
			p.srcmap.Put(cast, p.spanOf(start, p.index-1))
			nexpr = cast
		}
	}
	//
	return nexpr, errors
}

// parseAccessExpr parses an expression of the form
//   - memory[address]
//   - function(arguments)
//   - constant
//   - variable
func (p *Parser) parseTupleExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		start = p.index
		errs  []source.SyntaxError
		exprs = make([]Expr, 1)
	)
	// match initial expression
	if exprs[0], errs = p.parseExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// continue matching whilst there are commas
	for p.match(COMMA) {
		var expr Expr
		//
		if expr, errs = p.parseExpr(env); len(errs) > 0 {
			return nil, errs
		}
		//
		exprs = append(exprs, expr)
	}
	//
	if len(exprs) == 1 {
		return exprs[0], nil
	}
	//
	init := expr.NewTupleInitialiser(exprs...)
	p.srcmap.Put(init, p.spanOf(start, p.index-1))
	//
	return init, nil
}

func (p *Parser) parseAccessExpr(env Environment) (Expr, []source.SyntaxError) {
	var (
		nexpr Expr
		errs  []source.SyntaxError
		name  string
	)
	//
	name, errs = p.parseIdentifier()

	isDeclared := env.IsDeclaredVariable(name)
	if !isDeclared {
		// now, extern access check for function call or memory access
		if len(errs) == 0 && p.match(LSQUARE) {
			var args []Expr
			//
			args, errs = p.parseExprList(RSQUARE, env)
			//
			nexpr = expr.NewExternAccess(symbol.NewUnresolved(name, symbol.READABLE_MEMORY, uint(len(args))), args...)
		} else if len(errs) == 0 && p.match(LBRACE) {
			var args []Expr
			//
			args, errs = p.parseExprList(RBRACE, env)
			//
			nexpr = expr.NewExternAccess(symbol.NewUnresolved(name, symbol.FUNCTION, uint(len(args))), args...)
		} else {
			// Constant access
			nexpr = expr.NewExternAccess(symbol.NewUnresolved(name, symbol.CONSTANT, 0))
		}
	} else {
		rid := env.LookupVariable(name)
		if !p.match(LSQUARE) {
			// Register access
			nexpr = expr.NewLocalAccess[symbol.Unresolved](rid)
		} else {
			// Array access
			var arg Expr
			//
			if arg, errs = p.parseExpr(env); len(errs) > 0 {
				return nil, errs
			} else if _, errs = p.expect(RSQUARE); len(errs) > 0 {
				return nil, errs
			}
			//
			nexpr = expr.NewArrayAccess(rid, arg)
		}
	}
	//
	return nexpr, errs
}

// Parse sequence of one or more expressions separated by a comma.
// nolint
func (p *Parser) parseExprList(terminator uint, env Environment) ([]Expr, []source.SyntaxError) {
	var (
		lhs  = make([]Expr, 0)
		errs []source.SyntaxError
		expr Expr
	)
	// lhs may have additional registers
	for !p.match(terminator) {
		lookahead := p.lookahead()
		// match ",""
		if len(lhs) != 0 && !p.match(COMMA) {
			return nil, p.syntaxErrors(lookahead, "expected ,")
		}
		//
		if expr, errs = p.parseExpr(env); len(errs) > 0 {
			return nil, errs
		}
		// Add register to lhs
		lhs = append(lhs, expr)
	}
	//
	return lhs, nil
}

func (p *Parser) parseLVals(env Environment) ([]LVal, []source.SyntaxError) {
	var (
		lhs  []LVal = make([]LVal, 1)
		errs []source.SyntaxError
		reg  LVal
	)
	// lhs always starts with a register
	if lhs[0], errs = p.parseLVal(env); len(errs) > 0 {
		return nil, errs
	}
	// lhs may have additional registers
	for p.match(COMMA) {
		if reg, errs = p.parseLVal(env); len(errs) > 0 {
			return nil, errs
		}
		// Add register to lhs
		lhs = append(lhs, reg)
	}
	//
	return lhs, nil
}

func (p *Parser) parseLVal(env Environment) (LVal, []source.SyntaxError) {
	var (
		lv         LVal
		start      = p.index
		lookahead  = p.lookahead()
		reg, errs  = p.parseIdentifier()
		isDeclared = env.IsDeclaredVariable(reg)
		index      []Expr
	)
	//
	if len(errs) > 0 {
		return lv, errs
	} else if isDeclared && !p.match(LSQUARE) {
		var vars = []variable.Id{env.LookupVariable(reg)}
		// Look for destructuring lvals
		for p.match(COLONCOLON) {
			// save lookahead for error reporting
			lookahead = p.lookahead()
			//
			if reg, errs = p.parseIdentifier(); len(errs) > 0 {
				return lv, errs
			} else if !env.IsDeclaredVariable(reg) {
				return lv, p.syntaxErrors(lookahead, "unknown variable")
			}
			//
			vars = append(vars, env.LookupVariable(reg))
		}
		//
		lv = lval.NewVariable[symbol.Unresolved](vars...)
	} else if !isDeclared && !p.match(LSQUARE) {
		return lv, p.syntaxErrors(lookahead, "unknown variable")
	} else if index, errs = p.parseExprList(RSQUARE, env); len(errs) > 0 {
		return lv, errs
	} else if isDeclared {
		if len(index) != 1 {
			return lv, p.syntaxErrors(lookahead, "incorrect number of array access arguments")
		}

		lv = lval.NewArray(env.LookupVariable(reg), index[0])
	} else {
		// construct name symbol
		var name = symbol.NewUnresolved(reg, symbol.WRITEABLE_MEMORY, 1)
		// Done
		lv = lval.NewMemAccess(name, index)
	}
	// update source mapping
	p.srcmap.Put(lv, p.spanOf(start, p.index-1))
	//
	return lv, nil
}

// parseIdentifier expects an IDENTIFIER token
func (p *Parser) parseIdentifier() (string, []source.SyntaxError) {
	tok, errs := p.expect(IDENTIFIER)
	//
	if len(errs) > 0 {
		return "", errs
	}
	//
	return p.string(tok), nil
}

// Get the text representing the given token as a string.
func (p *Parser) string(token lex.Token) string {
	start, end := token.Span.Start(), token.Span.End()
	return string(p.srcfile.Contents()[start:end])
}

// Get the text representing the given token as a string.
func (p *Parser) number(token lex.Token) (big.Int, []source.SyntaxError) {
	var (
		number, exponent big.Int
		ok               bool
		numstr           = p.string(token)
		splits           = strings.Split(numstr, "^")
	)
	//
	if len(splits) == 0 || len(splits) > 2 {
		ok = false
	} else if len(splits) == 1 {
		// non-exponent case
		_, ok = number.SetString(numstr, 0)
	} else if len(splits[0]) == 0 || len(splits[1]) == 0 {
		ok = false
	} else {
		_, ok = number.SetString(splits[0], 0)
		exponent.SetString(splits[1], 0)
		number.Exp(&number, &exponent, nil)
	}
	//
	if !ok {
		return number, p.syntaxErrors(token, "malformed numeric literal")
	}
	//
	return number, nil
}

// Get the text representing the given token as a string.
func (p *Parser) baserOfNumber(token lex.Token) uint {
	var str = p.string(token)
	//
	if strings.HasPrefix(str, "0x") {
		return 16
	} else if strings.HasPrefix(str, "0b") {
		return 2
	}
	//
	return 10
}

// Lookahead returns the next token.  This must exist because EOF is always
// appended at the end of the token stream.
func (p *Parser) lookahead() lex.Token {
	return p.tokens[p.index]
}

// Lookahead returns the next token.  This must exist because EOF is always
// appended at the end of the token stream.
func (p *Parser) previousToken() lex.Token {
	if p.index == 0 {
		return p.tokens[p.index]
	}
	//
	return p.tokens[p.index-1]
}

// Expect returns an error if the next token is not of the expected Kind.
// Otherwise it returns the token and increments the index by 1.
func (p *Parser) expect(kind uint) (lex.Token, []source.SyntaxError) {
	lookahead := p.lookahead()
	//
	if lookahead.Kind != kind {
		errs := p.syntaxErrors(lookahead, "unexpected token")
		return lookahead, errs
	}
	//
	p.index++
	//
	return lookahead, nil
}

// Match attempts to match the given token: if the current token matches
// the argument of match(...) it is consumed.
func (p *Parser) match(kind uint) bool {
	if p.lookahead().Kind == kind {
		p.index++
		return true
	}
	//
	return false
}

// Follows checks whether one of the given token kinds is next.
func (p *Parser) follows(options ...uint) bool {
	return slices.Contains(options, p.lookahead().Kind)
}

func (p *Parser) spanOf(firstToken, lastToken int) source.Span {
	//
	start := p.tokens[firstToken].Span.Start()
	end := p.tokens[lastToken].Span.End()
	//
	return source.NewSpan(start, end)
}

func (p *Parser) syntaxErrors(token lex.Token, msg string) []source.SyntaxError {
	return []source.SyntaxError{*p.srcfile.SyntaxError(token.Span, msg)}
}

// Flatten a given condition starting at a given program counter position into
// a sequence of one or more instructions.  The intention is: (1) for positive
// sign, branch to target label if condition holds (otherwise fall through); (2)
// for negative sign, branch to target label if condition does not hold
// (otherwise fall through).
// isValidCondition checks that an expression is a valid condition for use in
// if/else, while, or for statements (i.e. Cmp, LogicalAnd, LogicalOr, LogicalNot).
func isValidCondition(e Expr) bool {
	switch e := e.(type) {
	case *expr.Cmp[symbol.Unresolved]:
		return true
	case *expr.LogicalAnd[symbol.Unresolved]:
		for _, arg := range e.Exprs {
			if !isValidCondition(arg) {
				return false
			}
		}
		//
		return true
	case *expr.LogicalOr[symbol.Unresolved]:
		for _, arg := range e.Exprs {
			if !isValidCondition(arg) {
				return false
			}
		}
		//
		return true
	case *expr.LogicalNot[symbol.Unresolved]:
		return true
	default:
		return false
	}
}
