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
	"bytes"

	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler/format"
	"go.lsp.dev/protocol"
)

// FormattingFor formats the given document text and returns a single TextEdit
// that replaces the entire document with its canonical form. Returns nil (no
// edits) when the document has parse errors or is already correctly formatted.
func FormattingFor(uri protocol.URI, text string) ([]protocol.TextEdit, error) {
	var (
		// temporary buffer for writing output
		buf bytes.Buffer
		// source file representation
		src = source.NewSourceFile(uri.Filename(), []byte(text))
		// construct default formatter
		formatter, _ = format.NewFormatter(&buf, src)
	)
	// apply formatting
	if err := formatter.Format(); err != nil {
		return nil, err
	}

	formatted := buf.String()

	if formatted == text {
		return nil, nil
	}

	// Span covering the whole document; spanToRange handles coordinate encoding.
	wholeDoc := source.NewSpan(0, len(src.Contents()))
	docRange := spanToRange(*src, wholeDoc)

	return []protocol.TextEdit{{
		Range:   docRange,
		NewText: formatted,
	}}, nil
}
