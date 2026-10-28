// Copyright Consensys Incorporated
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
package sexp

import "math"

// FormattingRule provides a generic mechanism for writing custom formatting
// rules.  Whenever a list is encountered during formatting, the formatting
// rules will be given the opportunity to direct formatting of the list.  That
// is, whether to start a new line and indent the list as whole and/or any of
// its children.  A formatting rule should return nil for the formatting chunks
// when it doesn't handle the given list.
type FormattingRule interface {
	Split(*List) ([]FormattingChunk, uint)
}

// LFormatter is a simple formatter which indents a list like so:
//
//	(head
//	  child1
//	  child2
//	  ...
//	  childn)
//
// That is, the head starts on a newly indented line, and each child is indented
// one more position.
type LFormatter struct {
	// Head symbol to match
	Head string
	// Priority to give for matching.
	Priority uint
}

// Split a list using the LFormatter where the list matches.
func (p *LFormatter) Split(list *List) ([]FormattingChunk, uint) {
	if list.Len() == 0 {
		return nil, 0
	} else if sym, ok := list.Get(0).(*Symbol); ok && sym.String(true) != p.Head {
		return nil, 0
	}
	//
	var chunks []FormattingChunk
	//
	for i := 0; i < list.Len(); i++ {
		var chunk FormattingChunk
		//
		chunk.Contents = list.Get(i)
		//
		if i == 0 {
			chunk.Priority = math.MaxUint
		} else {
			chunk.Priority = p.Priority
			chunk.Indent = 1
		}
		//
		chunks = append(chunks, chunk)
	}
	//
	return chunks, 1
}

// SFormatter is a variation on the LFormatter which does not indent the first
// child, thusly:
//
//	(head child1
//	  child2
//	  ...
//	  childn)
//
// That is, the head starts on a newly indented line, and each child is indented
// one more position.
type SFormatter struct {
	// Head symbol to match
	Head string
	// Priority to give for matching.
	Priority uint
}

// Split a list using the LFormatter where the list matches.
func (p *SFormatter) Split(list *List) ([]FormattingChunk, uint) {
	if list.Len() == 0 {
		return nil, 0
	} else if sym, ok := list.Get(0).(*Symbol); ok && sym.String(true) != p.Head {
		return nil, 0
	}
	//
	var chunks []FormattingChunk
	//
	for i := 0; i < list.Len(); i++ {
		var chunk FormattingChunk
		//
		chunk.Contents = list.Get(i)
		//
		if i <= 1 {
			chunk.Priority = math.MaxUint
		} else {
			chunk.Priority = p.Priority
			chunk.Indent = 1
		}
		//
		chunks = append(chunks, chunk)
	}
	//
	return chunks, 1
}

// IFormatter is your basic formatting rule.
type IFormatter struct {
	// Head symbol to match
	Head string
	// Priority to give for matching.
	Priority uint
}

// Split a list using the LFormatter where the list matches.
func (p *IFormatter) Split(list *List) ([]FormattingChunk, uint) {
	if list.Len() == 0 {
		return nil, 0
	} else if sym, ok := list.Get(0).(*Symbol); ok && sym.String(true) != p.Head {
		return nil, 0
	}
	//
	var chunks []FormattingChunk
	//
	for i := 0; i < list.Len(); i++ {
		var chunk FormattingChunk
		//
		chunk.Contents = list.Get(i)
		chunk.Priority = p.Priority
		chunk.Indent = 1
		//
		chunks = append(chunks, chunk)
	}
	//
	return chunks, math.MaxUint
}
