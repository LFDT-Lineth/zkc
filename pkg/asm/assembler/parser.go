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
package assembler

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/lex"
)

// MacroComponent is a component whose instructions (if applicable) are
// themselves macro instructions. A macro unit must be compiled down into a
// micro unit before we can generate constraints.
type MacroComponent = io.Component[macro.Instruction]

// MicroComponent is a component whose instructions (if applicable) are
// themselves micro instructions.  A micro function represents the lowest
// representation of a function, where each instruction is made up of
// microcodes.
type MicroComponent = io.Component[micro.Instruction]

// MacroFunction is a function whose instructions are themselves macro
// instructions.  A macro function must be compiled down into a micro function
// before we can generate constraints.
type MacroFunction = io.Function[macro.Instruction]

// MicroFunction is a function whose instructions are themselves micro
// instructions.  A micro function represents the lowest representation of a
// function, where each instruction is made up of microcodes.
type MicroFunction = io.Function[micro.Instruction]

// Parse accepts a given source file representing an assembly language
// program, and assembles it into an instruction sequence which can then the
// executed.
func Parse(srcfile *source.File) (AssemblyItem, []source.SyntaxError) {
	parser := NewParser(srcfile)
	// Parse functions
	return parser.Parse()
}

// BINOPS captures the set of binary operations
var BINOPS = []uint{SUB, MUL, ADD}

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
func (p *Parser) Parse() (AssemblyItem, []source.SyntaxError) {
	var (
		item      AssemblyItem
		include   *string
		errors    []source.SyntaxError
		component AssemblyComponent
	)
	// Convert source file into tokens
	if p.tokens, errors = Lex(*p.srcfile); len(errors) > 0 {
		return item, errors
	}
	// Continue going until all consumed
	for p.lookahead().Kind != END_OF {
		lookahead := p.lookahead()
		// Determine type of declaration
		switch lookahead.Kind {
		case KEYWORD_CONST:
			component, errors = p.parseConstant()
		case KEYWORD_INCLUDE:
			include, errors = p.parseInclude()
			if len(errors) == 0 {
				item.Includes = append(item.Includes, include)
			}
			// Avoid appending to components
			continue
		case KEYWORD_FN, KEYWORD_PUB:
			component, errors = p.parseFunction()
		default:
			errors = p.syntaxErrors(lookahead, "unknown declaration")
		}
		//
		if len(errors) > 0 {
			return item, errors
		}
		//
		item.Components = append(item.Components, component)
	}
	// Copy over source map
	item.SourceMap = *p.srcmap
	//
	return item, nil
}

func (p *Parser) parseConstant() (*AssemblyConstant, []source.SyntaxError) {
	var (
		start     = p.index
		errs      []source.SyntaxError
		lookahead lex.Token
		name      string
	)
	// Parse include declaration
	if _, errs := p.expect(KEYWORD_CONST); len(errs) > 0 {
		return nil, errs
	} else if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(EQUALS); len(errs) > 0 {
		return nil, errs
	} else if lookahead, errs = p.expect(NUMBER); len(errs) > 0 {
		return nil, errs
	}
	// Save for source map
	end := p.index
	// So far, so good.
	val, errs := p.number(lookahead)
	base := p.baserOfNumber(lookahead)
	//
	component := &AssemblyConstant{
		module.NewName(name, 1), val, base,
	}
	//
	p.srcmap.Put(component, p.spanOf(start, end-1))
	//
	return component, errs
}

func (p *Parser) parseInclude() (*string, []source.SyntaxError) {
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
	str = str[1 : len(str)-1]
	pStr := &str
	// Store for error reporting.
	p.srcmap.Put(pStr, tok.Span)
	// Done
	return pStr, errs
}

func (p *Parser) parseFunction() (*MacroFunction, []source.SyntaxError) {
	var (
		start  = p.index
		env    Environment
		inst   macro.Instruction
		name   string
		code   []macro.Instruction
		errs   []source.SyntaxError
		pc     uint
		public bool
	)
	// Parse pub modifier (if present)
	public = p.match(KEYWORD_PUB)
	// Parse function declaration
	if _, errs := p.expect(KEYWORD_FN); len(errs) > 0 {
		return nil, errs
	}
	// Parse function name
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	// Parse inputs
	if errs = p.parseArgsList(register.INPUT_REGISTER, &env); len(errs) > 0 {
		return nil, errs
	}
	// Parse optional '->'
	if p.match(RIGHTARROW) {
		// Parse returns
		if errs = p.parseArgsList(register.OUTPUT_REGISTER, &env); len(errs) > 0 {
			return nil, errs
		}
	}
	// Save for source map
	end := p.index
	// Parse start of block
	if _, errs = p.expect(LCURLY); len(errs) > 0 {
		return nil, errs
	}
	// Parse instructions until end of block
	for p.lookahead().Kind != RCURLY {
		if inst, errs = p.parseMacroInstruction(pc, &env); len(errs) > 0 {
			return nil, errs
		}
		//
		if inst != nil {
			code = append(code, inst)
			// inc pc only for real instructions.
			pc = pc + 1
		}
	}
	// Sanity check we parsed something
	if len(code) == 0 {
		return nil, p.syntaxErrors(p.lookahead(), "missing return")
	}
	// Advance past "}"
	p.match(RCURLY)
	// Sanity check labels
	if errs := p.checkLabelsDeclared(&env, code); len(errs) > 0 {
		return nil, errs
	}
	// Finalise labels
	env.BindLabels(code)
	// Construct function
	fn := io.NewFunction(module.NewName(name, 1), public, env.registers, env.buses, code)
	//
	p.srcmap.Put(&fn, p.spanOf(start, end-1))
	// Done
	return &fn, nil
}

func (p *Parser) checkLabelsDeclared(env *Environment, code []macro.Instruction) []source.SyntaxError {
	for _, c := range code {
		var label uint
		//
		switch c := c.(type) {
		case *macro.Goto:
			label = c.Target
		case *macro.IfGoto:
			label = c.Target
		default:
			continue
		}
		//
		if !env.IsLabelBound(label) {
			return p.srcmap.SyntaxErrors(c, "unknown label")
		}
	}
	//
	return nil
}

func (p *Parser) parseArgsList(kind register.Type, env *Environment) []source.SyntaxError {
	var (
		arg     string
		width   uint
		errs    []source.SyntaxError
		first   = true
		padding big.Int
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
		// save lookahead token for syntax errors
		lookahead := p.lookahead()
		// parse name, type & optional padding
		if arg, errs = p.parseIdentifier(); len(errs) > 0 {
			return errs
		} else if padding, errs = p.parseOptionalPadding(); len(errs) > 0 {
			return errs
		} else if width, errs = p.parseType(); len(errs) > 0 {
			return errs
		} else if env.IsRegister(arg) {
			return p.syntaxErrors(lookahead, "variable already declared")
		}
		//
		env.DeclareRegister(kind, arg, width, padding)
	}
	// Advance past "}"
	p.match(RBRACE)
	//
	return nil
}

func (p *Parser) parseOptionalPadding() (big.Int, []source.SyntaxError) {
	var (
		padding   big.Int
		errs      []source.SyntaxError
		lookahead lex.Token
	)
	//
	if !p.match(EQUALS) {
		// no optional padding provided
		return padding, nil
	} else if lookahead, errs = p.expect(NUMBER); len(errs) > 0 {
		return padding, errs
	}
	// Yes, optional padding provided
	return p.number(lookahead)
}

func (p *Parser) parseType() (uint, []source.SyntaxError) {
	var (
		lookahead = p.lookahead()
		name      string
		errs      []source.SyntaxError
	)
	//
	if name, errs = p.parseIdentifier(); len(errs) > 0 {
		return 0, errs
	}
	//
	switch {
	case strings.HasPrefix(name, "u"):
		// Parse bitwidth
		bw, err := strconv.Atoi(name[1:])
		//
		if err != nil {
			return 0, p.syntaxErrors(lookahead, err.Error())
		}
		//
		return uint(bw), nil
	default:
		return 0, p.syntaxErrors(lookahead, "unknown type")
	}
}

func (p *Parser) parseMacroInstruction(pc uint, env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		// Save current position for backtracking
		start = p.index
		errs  []source.SyntaxError
		first string
		insn  macro.Instruction
	)
	//
	if first, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	//
	switch first {
	case "fail":
		insn, errs = &macro.Fail{}, nil
	case "goto":
		insn, errs = p.parseGoto(env)
	case "if":
		insn, errs = p.parseIfGoto(env)
	case "return":
		insn, errs = &macro.Return{}, nil
	case "var":
		return nil, p.parseVar(env)
	default:
		isLabel := p.lookahead().Kind == COLON
		// Backtrack
		p.index = start
		// Distinguish label from assignment
		if isLabel {
			return p.parseLabel(pc, env)
		} else {
			insn, errs = p.parseAssignment(env)
		}
	}
	// Record source mapping
	if insn != nil {
		p.srcmap.Put(insn, p.spanOf(start, p.index-1))
	}
	//
	return insn, errs
}

func (p *Parser) parseGoto(env *Environment) (macro.Instruction, []source.SyntaxError) {
	lab, errs := p.parseIdentifier()
	//
	if len(errs) > 0 {
		return nil, errs
	}
	//
	return &macro.Goto{
		Target: env.BindLabel(lab)}, nil
}

func (p *Parser) parseLabel(pc uint, env *Environment) (macro.Instruction, []source.SyntaxError) {
	// Observe, following cannot fail
	tok, _ := p.expect(IDENTIFIER)
	// Likewise, this cannot fail
	p.expect(COLON)
	//
	lab := p.string(tok)
	//
	if env.IsBoundLabel(lab) {
		return nil, p.syntaxErrors(tok, "label already declared")
	}
	// Declare label at given pc
	env.DeclareLabel(lab, pc)
	// Done
	return nil, nil
}

func (p *Parser) parseVar(env *Environment) []source.SyntaxError {
	var (
		errs    []source.SyntaxError
		names   []string
		width   uint
		padding big.Int
	)
	// Parse name(s)
	for len(names) == 0 || p.match(COMMA) {
		// Store lookahead for error reporting
		lookahead := p.lookahead()
		//
		name, errs := p.parseIdentifier()
		//
		if len(errs) > 0 {
			return errs
		} else if env.IsRegister(name) {
			return p.syntaxErrors(lookahead, "variable already declared")
		}
		//
		names = append(names, name)
	}
	// parse bitwidth
	if width, errs = p.parseType(); len(errs) > 0 {
		return errs
	}
	//
	for _, name := range names {
		env.DeclareRegister(register.COMPUTED_REGISTER, name, width, padding)
	}
	//
	return nil
}

func (p *Parser) parseIfGoto(env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		errs     []source.SyntaxError
		lhs, rhs macro.AtomicExpr
		target   string
		cond     uint8
	)
	// Parse left hand side
	if lhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// save lookahead for error reporting
	if cond, errs = p.parseComparator(); len(errs) > 0 {
		return nil, errs
	}
	// Parse right hand side
	if rhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// Parse "goto"
	if errs = p.parseKeyword("goto"); len(errs) > 0 {
		return nil, errs
	}
	// Parse target label
	if target, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	}
	//
	return &macro.IfGoto{
		Cond:   cond,
		Left:   lhs,
		Right:  rhs,
		Target: env.BindLabel(target),
	}, nil
}

func (p *Parser) parseAssignment(env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		lhs  []io.RegisterId
		rhs  macro.Expr
		errs []source.SyntaxError
	)
	// parse left-hand side
	if lhs, errs = p.parseAssignmentLhs(env); len(errs) > 0 {
		return nil, errs
	}
	// Parse '='
	if _, errs = p.expect(EQUALS); len(errs) > 0 {
		return nil, errs
	}
	// Check what we've got
	if p.following(IDENTIFIER, LBRACE) {
		// function call
		return p.parseCallRhs(lhs, env)
	} else if p.following(IDENTIFIER, EQUALS_EQUALS) || p.following(IDENTIFIER, NOT_EQUALS) {
		// ternary assignment
		return p.parseTernaryRhs(lhs, env)
	} else if p.following(IDENTIFIER, DIV) || p.following(NUMBER, DIV) {
		// division assignment
		return p.parseDivisionRhs(lhs, env)
	} else if cast, ok := p.parseCast(env); ok {
		// parse cast
		return p.parseCastRhs(lhs, cast, env)
	}
	// Parse right-hand side
	if rhs, errs = p.parseExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// Done
	return &macro.Assign{Targets: lhs, Source: rhs}, nil
}

func (p *Parser) parseAssignmentLhs(env *Environment) ([]io.RegisterId, []source.SyntaxError) {
	lhs, errs := p.parseRegisterList(env)
	// Reverse items so that least significant comes first.
	lhs = array.Reverse(lhs)
	//
	return lhs, errs
}

func (p *Parser) parseCast(env *Environment) (uint, bool) {
	var start = p.index
	//
	if p.following(LBRACE, IDENTIFIER, RBRACE) {
		p.match(LBRACE)

		if v, errs := p.parseType(); len(errs) == 0 {
			p.match(RBRACE)
			return v, true
		}
		// failed parsing to back up
		p.index = start
	}
	// No cast to parse
	return 0, false
}

func (p *Parser) parseCastRhs(lhs []io.RegisterId, cast uint, env *Environment,
) (macro.Instruction, []source.SyntaxError) {
	//
	var (
		errs []source.SyntaxError
		rhs  register.Id
	)
	//
	if rhs, errs = p.parseVariable(env); len(errs) > 0 {
		return nil, errs
	}
	//
	return macro.NewCast(lhs, cast, rhs), nil
}

func (p *Parser) parseCallRhs(lhs []io.RegisterId, env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		errs []source.SyntaxError
		rhs  []macro.Expr
		fn   string
	)
	//
	if fn, errs = p.parseIdentifier(); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(LBRACE); len(errs) > 0 {
		return nil, errs
	} else if rhs, errs = p.parseExprList(env); len(errs) > 0 {
		return nil, errs
	} else if _, errs = p.expect(RBRACE); len(errs) > 0 {
		return nil, errs
	}
	// Generate temporary bus identifier
	bus := env.BindBus(module.NewName(fn, 1))
	// Done
	return macro.NewCall(bus, lhs, rhs), nil
}

func (p *Parser) parseTernaryRhs(targets []io.RegisterId, env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		errs     []source.SyntaxError
		tb, fb   macro.Expr
		lhs, rhs macro.AtomicExpr
		cond     uint8
	)
	// Parse left hand side
	if lhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// save lookahead for error reporting
	if cond, errs = p.parseComparator(); len(errs) > 0 {
		return nil, errs
	}
	// Parse right hand side
	if rhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// sanity check
	if _, lhsConst := lhs.(*expr.Const); lhsConst {
		if _, rhsConst := rhs.(*expr.Const); rhsConst {
			return nil, p.syntaxErrors(p.tokens[p.index-1], "branch always (or never) taken")
		}
	}
	// expect question mark
	if _, errs = p.expect(QMARK); len(errs) > 0 {
		return nil, errs
	}
	// true branch
	if tb, errs = p.parseExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// expect column
	if _, errs = p.expect(COLON); len(errs) > 0 {
		return nil, errs
	}
	// false branch
	if fb, errs = p.parseExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// Done
	return &macro.IfThenElse{
		Targets: targets,
		Cond:    cond,
		Left:    lhs,
		Right:   rhs,
		Then:    tb,
		Else:    fb,
	}, nil
}

func (p *Parser) parseDivisionRhs(targets []io.RegisterId, env *Environment) (macro.Instruction, []source.SyntaxError) {
	var (
		errs     []source.SyntaxError
		lhs, rhs macro.AtomicExpr
	)
	//
	if len(targets) < 2 {
		return nil, p.syntaxErrors(p.tokens[p.index-2], "missing target register for remainder")
	} else if len(targets) < 3 {
		return nil, p.syntaxErrors(p.tokens[p.index-2], "missing target register for witness")
	} else if len(targets) < 4 {
		return nil, p.syntaxErrors(p.tokens[p.index-2], "missing target register for sum")
	} else if len(targets) < 5 {
		return nil, p.syntaxErrors(p.tokens[p.index-2], "missing target register for witness sum")
	} else if len(targets) > 5 {
		return nil, p.syntaxErrors(p.tokens[p.index-2], "unexpected target register")
	}
	// Parse left hand side
	if lhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// expect division operator
	if _, errs = p.expect(DIV); len(errs) > 0 {
		return nil, errs
	}
	// Parse right hand side
	if rhs, errs = p.parseAtomicExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// NOTE: target registers are in reverse order due to being sorted in
	// parseAssignmentLhs().
	array.ReverseInPlace(targets)
	//
	return &macro.Division{
		Quotient:  expr.RegAccess{Register: targets[0]},
		Remainder: expr.RegAccess{Register: targets[1]},
		Witness:   expr.RegAccess{Register: targets[2]},
		Sum:       expr.RegAccess{Register: targets[3]},
		WitSum:    expr.RegAccess{Register: targets[4]},
		Dividend:  lhs,
		Divisor:   rhs,
	}, nil
}

func (p *Parser) parseExpr(env *Environment) (macro.Expr, []source.SyntaxError) {
	var (
		start      = p.index
		expr, errs = p.parseUnitExpr(env)
		exprs      = []macro.Expr{expr}
		tmp        macro.Expr
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
		tmp, errs = p.parseUnitExpr(env)
		// Accumulate arguments
		exprs = append(exprs, tmp)
	}
	//
	switch {
	case len(errs) != 0:
		return expr, errs
	case len(exprs) == 1:
		return expr, nil
	case kind == ADD:
		expr = macro.Sum(exprs...)
	case kind == MUL:
		expr = macro.Product(exprs...)
	case kind == SUB:
		expr = macro.Subtract(exprs...)
	}
	//
	p.srcmap.Put(expr, p.spanOf(start, p.index-1))
	//
	return expr, nil
}

func (p *Parser) parseUnitExpr(env *Environment) (macro.Expr, []source.SyntaxError) {
	var lookahead = p.lookahead()

	switch lookahead.Kind {
	case IDENTIFIER, NUMBER:
		return p.parseAtomicExpr(env)
	case LBRACE:
		p.match(LBRACE)
		expr, errs := p.parseExpr(env)
		//
		if len(errs) > 0 {
			return nil, errs
		} else if _, errs = p.expect(RBRACE); len(errs) > 0 {
			return nil, errs
		}
		// Don't add to source map, since it will already have been added.
		return expr, nil
	default:
		return nil, p.syntaxErrors(lookahead, "unexpected token")
	}
}

func (p *Parser) parseAtomicExpr(env *Environment) (macro.AtomicExpr, []source.SyntaxError) {
	var (
		start     = p.index
		lookahead = p.lookahead()
		expr      macro.AtomicExpr
		errs      []source.SyntaxError
	)

	switch lookahead.Kind {
	case IDENTIFIER:
		var reg string
		//
		reg, errs = p.parseIdentifier()
		//
		if len(errs) > 0 {
			return nil, errs
		} else if !env.IsRegister(reg) {
			expr = macro.ConstantAccess(reg)
		} else {
			// Register access
			rid := env.LookupRegister(reg)
			// Done
			expr = macro.RegisterAccess(rid)
		}
	case NUMBER:
		var val big.Int
		//
		p.match(NUMBER)
		//
		val, errs = p.number(lookahead)
		base := p.baserOfNumber(lookahead)
		//
		expr = macro.Constant(val, base)
	default:
		return nil, p.syntaxErrors(lookahead, "expected register or constant")
	}
	//
	p.srcmap.Put(expr, p.spanOf(start, p.index-1))
	//
	return expr, errs
}

// Parse sequence of one or more expressions separated by a comma.
func (p *Parser) parseExprList(env *Environment) ([]macro.Expr, []source.SyntaxError) {
	var (
		lhs  = make([]macro.Expr, 1)
		errs []source.SyntaxError
		expr macro.Expr
	)
	// lhs always starts with a register
	if lhs[0], errs = p.parseExpr(env); len(errs) > 0 {
		return nil, errs
	}
	// lhs may have additional registers
	for p.match(COMMA) {
		if expr, errs = p.parseExpr(env); len(errs) > 0 {
			return nil, errs
		}
		// Add register to lhs
		lhs = append(lhs, expr)
	}
	//
	return lhs, nil
}

// Parse sequence of one or more registers separated by a comma.
func (p *Parser) parseRegisterList(env *Environment) ([]io.RegisterId, []source.SyntaxError) {
	var (
		lhs  []io.RegisterId = make([]io.RegisterId, 1)
		errs []source.SyntaxError
		reg  io.RegisterId
	)
	// lhs always starts with a register
	if lhs[0], errs = p.parseVariable(env); len(errs) > 0 {
		return nil, errs
	}
	// lhs may have additional registers
	for p.match(COMMA) {
		if reg, errs = p.parseVariable(env); len(errs) > 0 {
			return nil, errs
		}
		// Add register to lhs
		lhs = append(lhs, reg)
	}
	//
	return lhs, nil
}

func (p *Parser) parseVariable(env *Environment) (io.RegisterId, []source.SyntaxError) {
	lookahead := p.lookahead()
	reg, errs := p.parseIdentifier()
	//
	if len(errs) > 0 {
		return io.RegisterId{}, errs
	} else if !env.IsRegister(reg) {
		return io.RegisterId{}, p.syntaxErrors(lookahead, "unknown register")
	}
	// Done
	return env.LookupRegister(reg), nil
}

func (p *Parser) parseKeyword(keyword string) []source.SyntaxError {
	tok, errs := p.expect(IDENTIFIER)
	//
	if len(errs) > 0 {
		return errs
	} else if p.string(tok) != keyword {
		return p.syntaxErrors(tok, fmt.Sprintf("expected \"%s\"", keyword))
	}
	//
	return nil
}

func (p *Parser) parseIdentifier() (string, []source.SyntaxError) {
	tok, errs := p.expect(IDENTIFIER)
	//
	if len(errs) > 0 {
		return "", errs
	}
	//
	return p.string(tok), nil
}

func (p *Parser) parseComparator() (uint8, []source.SyntaxError) {
	var (
		lookahead = p.lookahead()
		op        uint8
	)
	// Parse operation
	switch lookahead.Kind {
	case EQUALS_EQUALS:
		op = macro.EQ
	case NOT_EQUALS:
		op = macro.NEQ
	case LESS_THAN:
		op = macro.LT
	case LESS_THAN_EQUALS:
		op = macro.LTEQ
	case GREATER_THAN:
		op = macro.GT
	case GREATER_THAN_EQUALS:
		op = macro.GTEQ
	default:
		return math.MaxUint8, p.syntaxErrors(lookahead, "unknown comparator")
	}
	//
	p.match(lookahead.Kind)
	//
	return op, nil
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

// Expect reurns an arror if the next token is not what was expected.
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

// Match attempts to match the given token.
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

// Following attempts to check what follows the current position.
func (p *Parser) following(kinds ...uint) bool {
	for i, kind := range kinds {
		n := i + p.index
		if n >= len(p.tokens) {
			return false
		} else if p.tokens[n].Kind != kind {
			return false
		}
	}
	//
	return true
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

// Label represents a potentially unresolved label within an assembly.
type Label struct {
	// Name of the label
	name string
	// PC position the label represents.  This will be math.MaxUint until the
	// label is officially declared.
	pc uint
}

// UnboundLabel constructs a label whose PC location is (as yet) unknown.
func UnboundLabel(name string) Label {
	return Label{name, math.MaxUint}
}

// BoundLabel constructs a label whose PC location is known.
func BoundLabel(name string, pc uint) Label {
	return Label{name, pc}
}
