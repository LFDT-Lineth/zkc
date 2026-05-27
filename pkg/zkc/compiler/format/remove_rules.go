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
	"github.com/consensys/go-corset/pkg/util/collection/iter"
	"github.com/consensys/go-corset/pkg/util/source/lex"
	"github.com/consensys/go-corset/pkg/zkc/compiler/parser"
)

var (
	// DEFAULT_REMOVAL_RULES contains the set of default token removal rules to use.
	DEFAULT_REMOVAL_RULES []RemovalRule
)

func init() {
	DEFAULT_REMOVAL_RULES = make([]RemovalRule, parser.MAX_TOKEN)
	// Collapse runs of more than two consecutive newlines to at most two.
	DEFAULT_REMOVAL_RULES[parser.NEWLINE] = RemoveExcessNewlines()
}

// RemovalRule decides whether a token should be removed from the output stream
// based on the surrounding context.  When either Before or After returns true,
// the token and any insertions that would have been associated with it are
// omitted.
type RemovalRule interface {
	// Before returns true if the token should be removed based on the preceding
	// tokens.  The prev iterator yields already-emitted tokens in reverse order
	// (most recent first).
	Before(prev iter.Iterator[lex.Token]) bool
	// After returns true if the token should be removed based on the following
	// tokens.  The next iterator yields upcoming tokens in forward order
	// (nearest first).
	After(next iter.Iterator[lex.Token]) bool
}

// RemoveExcessNewlines returns a rule that removes a NEWLINE token when it is
// already preceded by two or more NEWLINEs, collapsing runs of blank lines to
// at most one blank line.
func RemoveExcessNewlines() RemovalRule {
	return &removeExcessNewlines{}
}

// ===================================================================
// removeExcessNewlines
// ===================================================================

type removeExcessNewlines struct{}

// Before counts NEWLINEs in the already-emitted output (skipping over any
// SPACES/TABS inserted as indentation).  If two or more are found the current
// NEWLINE is redundant and should be dropped.
func (r *removeExcessNewlines) Before(prev iter.Iterator[lex.Token]) bool {
	count := 0
	//
	for prev.HasNext() {
		tok := prev.Next()
		//
		switch tok.Kind {
		case parser.NEWLINE:
			count++
			if count >= 2 {
				return true
			}
		case parser.SPACES, parser.TABS:
			// skip indentation tokens emitted after previous NEWLINEs
		default:
			return false
		}
	}
	//
	return false
}

func (r *removeExcessNewlines) After(_ iter.Iterator[lex.Token]) bool {
	return false
}
