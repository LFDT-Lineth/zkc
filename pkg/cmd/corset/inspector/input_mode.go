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
package inspector

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/bexp"
	"github.com/LFDT-Lineth/zkc/pkg/util/termio"
)

// InputMode is where the user is entering some information (e.g. row for
// executing a goto command).
type InputMode[T any] struct {
	// prompt to show user
	prompt termio.FormattedText
	// input text being accumulated whilst in input mode.
	input []byte
	// current cursor position
	cursor uint
	// history of options for this input mode.
	history []string
	// history index
	index uint
	// parser responsible for checking whether input is valid (or not).
	handler InputHandler[T]
}

// InputHandler provides a generic way of handling input, including a mechanism
// for checking that input is well formed.
type InputHandler[T any] interface {
	// Convert attempts to convert the input string into a valid value.
	Convert(string) (T, error)
	// Apply the given input, which will activate some kind of callback.
	Apply(T) termio.FormattedText
}

func newInputMode[T any](prompt termio.FormattedText, index uint, history []string,
	handler InputHandler[T]) *InputMode[T] {
	var input []byte
	// Determine whether to show item from history
	if index >= uint(len(history)) {
		input = []byte{}
	} else {
		input = []byte(history[index])
	}
	// Done
	return &InputMode[T]{prompt, input, 0, history, index, handler}
}

// Activate navigation mode by setting the command bar to show the navigation
// commands.
func (p *InputMode[T]) Activate(parent *Inspector) {
	parent.cmdBar.Clear()
	parent.cmdBar.AddLeft(p.prompt)
	// Add current filter
	colour := termio.TERM_GREEN
	invColour := termio.TERM_BLACK
	input := string(p.input)
	// indicate validity of input
	if _, err := p.handler.Convert(input); len(input) != 0 && err != nil {
		colour = termio.TERM_RED
		//
		parent.SetStatus(termio.NewColouredText(err.Error(), termio.TERM_RED))
	} else {
		parent.SetStatus(termio.NewText(""))
	}
	// construct cursor escape code
	escape := termio.NewAnsiEscape().FgColour(invColour).BgColour(termio.TERM_YELLOW)
	// handle cursor
	if p.cursor < uint(len(p.input)) {
		// cursor behind text
		parent.cmdBar.AddLeft(termio.NewColouredText(input[:p.cursor], colour))
		parent.cmdBar.AddLeft(termio.NewFormattedText(input[p.cursor:p.cursor+1], escape))
		parent.cmdBar.AddLeft(termio.NewColouredText(input[p.cursor+1:], colour))
	} else {
		// cursor leading text
		parent.cmdBar.AddLeft(termio.NewColouredText(input, colour))
		parent.cmdBar.AddLeft(termio.NewFormattedText(" ", escape))
	}
}

// Clock navitation mode, which does nothing at this time.
func (p *InputMode[T]) Clock(parent *Inspector) {
	// Nothing to do.
}

// KeyPressed in input mode simply updates the input, or exits the mode if
// either "ESC" or enter are pressed.
func (p *InputMode[T]) KeyPressed(parent *Inspector, key uint16) bool {
	switch {
	case key == termio.ESC:
		return true
	case key == termio.BACKSPACE || key == termio.DEL:
		p.deleteCharacterAtCursor()
	case key == termio.CARRIAGE_RETURN:
		input := string(p.input)
		// Attempt conversion
		val, err := p.handler.Convert(input)
		//
		if err != nil {
			parent.SetStatus(termio.NewColouredText(err.Error(), termio.TERM_RED))
			return false
		}
		// Looks good, to fire the value
		outcome := p.handler.Apply(val)
		//
		parent.SetStatus(outcome)
		// Success
		return true
	case key == termio.CURSOR_LEFT:
		if p.cursor > 0 {
			p.cursor--
		}
	case key == termio.CURSOR_RIGHT:
		if p.cursor < uint(len(p.input)) {
			p.cursor++
		}
	case key == termio.CURSOR_UP:
		if p.index > 0 {
			p.index--
			p.input = []byte(p.history[p.index])
		}
	case key == termio.CURSOR_DOWN:
		p.index++
		// Check for end-of-history
		if p.index >= uint(len(p.history)) {
			p.index = uint(len(p.history))
			p.input = []byte{}
		} else {
			p.input = []byte(p.history[p.index])
		}
	case key >= 32 && key <= 126:
		p.insertCharacterAtCursor(byte(key))
	}
	// Update displayed text
	p.Activate(parent)
	//
	return false
}

// Delete character at cursor position
func (p *InputMode[T]) deleteCharacterAtCursor() {
	if p.cursor > 0 {
		p.cursor--
		p.input = array.RemoveAt(p.input, p.cursor)
	}
}

// Insert character at cursor position
func (p *InputMode[T]) insertCharacterAtCursor(char byte) {
	p.input = array.InsertAt(p.input, char, p.cursor)
	// advance cursor
	p.cursor++
}

// ==================================================================
// UintHandler
// ==================================================================

type uintHandler struct {
	callback func(uint) termio.FormattedText
}

func newUintHandler(callback func(uint) termio.FormattedText) InputHandler[uint] {
	return &uintHandler{callback}
}

func (p *uintHandler) Convert(input string) (uint, error) {
	val, err := strconv.Atoi(input)
	//
	if val < 0 || err != nil {
		return 0, errors.New("invalid integer")
	}
	//
	return uint(val), nil
}

func (p *uintHandler) Apply(value uint) termio.FormattedText {
	return p.callback(value)
}

// ==================================================================
// RegexHandler
// ==================================================================

type regexHandler struct {
	callback func(*regexp.Regexp) termio.FormattedText
}

func newRegexHandler(callback func(*regexp.Regexp) termio.FormattedText) InputHandler[*regexp.Regexp] {
	return &regexHandler{callback}
}

func (p *regexHandler) Convert(input string) (*regexp.Regexp, error) {
	return regexp.Compile(input)
}

func (p *regexHandler) Apply(regex *regexp.Regexp) termio.FormattedText {
	return p.callback(regex)
}

// ==================================================================
// Proposition (i.e. Boolean Expression) Handler
// ==================================================================

type queryHandler struct {
	// environment determines which variables are permitted
	env func(string) bool
	//
	callback func(*Query) termio.FormattedText
	//
	promptOffset int
}

func newQueryHandler(env func(string) bool, callback func(*Query) termio.FormattedText,
	offset int) InputHandler[*Query] {
	return &queryHandler{env, callback, offset}
}

func (p *queryHandler) Convert(input string) (*Query, error) {
	prop, errs := bexp.Parse[*Query](input, p.env)
	// Check whether any errors reported
	if len(errs) == 0 {
		return prop, nil
	}
	// Yes, so take the first one only (as no space for anything else).
	return nil, errors.New(query_error(errs[0], p.promptOffset))
}

func (p *queryHandler) Apply(query *Query) termio.FormattedText {
	return p.callback(query)
}

func query_error(err source.SyntaxError, offset int) string {
	var builder strings.Builder
	//
	span := err.Span()
	// Determine start and end
	start, end := span.Start(), span.End()
	//
	if start == end {
		end = end + 1
	}
	//
	for i := 0; i < start+offset; i++ {
		builder.WriteString(" ")
	}
	//
	for i := start; i < end; i++ {
		builder.WriteString("^")
	}
	//
	builder.WriteString(" ")
	builder.WriteString(err.Message())
	//
	return builder.String()
}
