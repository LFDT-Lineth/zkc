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
package lsp

import (
	"github.com/consensys/go-corset/pkg/util/source"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// DiagnosticsByFile groups the given syntax errors by their source file and
// converts each to a protocol.Diagnostic.  The returned map is keyed by the
// URI of every affected file; files with no errors do not appear, so callers
// that need to clear previously-published diagnostics must track that
// separately.
func DiagnosticsByFile(errs []source.SyntaxError) map[protocol.URI][]protocol.Diagnostic {
	out := make(map[protocol.URI][]protocol.Diagnostic)

	for _, err := range errs {
		u := uri.File(err.SourceFile().Filename())
		out[u] = append(out[u], syntaxErrToDiagnostic(err))
	}

	return out
}

// syntaxErrToDiagnostic converts a source.SyntaxError into a protocol.Diagnostic.
// The range is clipped to the first enclosing line, mirroring the behaviour of
// printSyntaxError in pkg/cmd/zkc/util.go.
func syntaxErrToDiagnostic(err source.SyntaxError) protocol.Diagnostic {
	span := err.Span()
	line := err.FirstEnclosingLine()
	startCol := uint32(span.Start() - line.Start())
	// Clip length to the current line so the range stays within bounds.
	length := min(line.Length()-int(startCol), span.Length())
	endCol := startCol + uint32(length)
	lineNo := uint32(line.Number() - 1) // LSP uses 0-indexed line numbers

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: lineNo, Character: startCol},
			End:   protocol.Position{Line: lineNo, Character: endCol},
		},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "zkc",
		Message:  err.Message(),
	}
}
