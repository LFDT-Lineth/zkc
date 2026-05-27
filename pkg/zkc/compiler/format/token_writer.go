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
package format

import (
	"bufio"
	"io"
	"strings"

	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
)

// TokenWriter is a simple mechanism for writing tokens to an output stream.
type TokenWriter struct {
	out   *bufio.Writer
	runes []rune
}

// NewTokenWriter constructs a new token writer for tokens generated from the
// given source file.
func NewTokenWriter(out io.Writer, runes []rune) TokenWriter {
	return TokenWriter{
		runes: runes,
		out:   bufio.NewWriter(out),
	}
}

// Flush the writer to the original output stream.
func (p *TokenWriter) Flush() error {
	return p.out.Flush()
}

// WriteTokens writes the tokens to the output writer exactly as is.  This
// returns an error if some issue arises whilst writing the string.
func (p *TokenWriter) WriteTokens(tokens ...lex.Token) error {
	for _, tok := range tokens {
		if _, err := p.out.WriteString(p.String(tok)); err != nil {
			return err
		}
	}
	//
	return nil
}

// String extracts the text string corresponding to a give span in the original
// source file.
func (p *TokenWriter) String(token lex.Token) string {
	var span = token.Span
	// Check what kind of token we have.  This is important because tokens
	// inserted by the formatter do not correspond to anything in the original
	// source file (i.e. their span information is wrong).
	switch token.Kind {
	case parser.NEWLINE:
		return "\n"
	case parser.SPACES:
		return strings.Repeat(" ", span.Length())
	case parser.TABS:
		return strings.Repeat("\t", span.Length())
	default:
		return string(p.runes[span.Start():span.End()])
	}
}
