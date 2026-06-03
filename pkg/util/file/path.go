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
package file

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// Path is a construct for describing paths through trees.  A path can be either
// *absolute* or *relative*.  An absolute path always starts from the root of
// the tree, whilst a relative path can begin from any point within the tree.
type Path struct {
	// Indicates whether or not this is an absolute path.
	absolute bool
	// Segments in the path.
	segments []string
}

// NewAbsolutePath constructs a new absolute path from the given segments.
func NewAbsolutePath(segments ...string) Path {
	return Path{true, segments}
}

// NewRelativePath constructs a new absolute path from the given segments.
func NewRelativePath(segments ...string) Path {
	return Path{false, segments}
}

// Depth returns the number of segments in this path (a.k.a its depth).
func (p *Path) Depth() uint {
	return uint(len(p.segments))
}

// IsAbsolute determines whether or not this is an absolute path.
func (p *Path) IsAbsolute() bool {
	return p.absolute
}

// Head returns the first (i.e. outermost) segment in this path.
func (p *Path) Head() string {
	return p.segments[0]
}

// Dehead removes the head from this path, returning an otherwise identical
// path.  Observe that, if this were absolute, it is no longer!
func (p *Path) Dehead() *Path {
	return &Path{false, p.segments[1:]}
}

// Tail returns the last (i.e. innermost) segment in this path.
func (p *Path) Tail() string {
	n := len(p.segments) - 1
	return p.segments[n]
}

// Get returns the nth segment of this path.
func (p *Path) Get(nth uint) string {
	return p.segments[nth]
}

// Equals determines whether two paths are the same.
func (p *Path) Equals(other Path) bool {
	return p.absolute == other.absolute && slices.Equal(p.segments, other.segments)
}

// Compare two paths lexicographically.
func (p *Path) Compare(other Path) int {
	if p.absolute != other.absolute && p.absolute {
		return 1
	} else if len(p.segments) < len(other.segments) {
		return -1
	} else if len(p.segments) > len(other.segments) {
		return 1
	}
	//
	for i := range p.segments {
		c := strings.Compare(p.segments[i], other.segments[i])
		//
		if c != 0 {
			return c
		}
	}
	//
	return 0
}

// PrefixOf checks whether this path is a prefix of the other.
func (p *Path) PrefixOf(other Path) bool {
	if len(p.segments) > len(other.segments) {
		return false
	}
	//
	for i := range p.segments {
		if p.segments[i] != other.segments[i] {
			return false
		}
	}
	// Looks good
	return true
}

// Slice returns the subpath starting from the given segment.
func (p *Path) Slice(start uint) *Path {
	return &Path{false, p.segments[start:]}
}

// PushRoot converts a relative path into an absolute path by pushing the "root"
// of the tree onto the head (i.e. outermost) position.
func (p *Path) PushRoot(tail string) *Path {
	if p.absolute {
		panic("cannot push root onto absolute path")
	}
	// Prepend root to segments
	nsegments := array.Prepend(tail, p.segments)
	// Convert to absolute path
	return &Path{true, nsegments}
}

// Parent returns the parent of this path.
func (p *Path) Parent() *Path {
	n := p.Depth() - 1
	return &Path{p.absolute, p.segments[0:n]}
}

// Extend returns this path extended with a new innermost segment.
func (p *Path) Extend(tail string) *Path {
	return &Path{p.absolute, array.Append(p.segments, tail)}
}

// String returns a string representation of this path.
func (p *Path) String() string {
	if p.IsAbsolute() {
		switch len(p.segments) {
		case 0:
			return ""
		case 1:
			return p.segments[0]
		case 2:
			return fmt.Sprintf("%s.%s", p.segments[0], p.segments[1])
		default:
			return fmt.Sprintf("%s/%s", p.Parent().String(), p.Tail())
		}
	}
	//
	switch len(p.segments) {
	case 0:
		// Non-sensical case really
		return "/"
	case 1:
		return fmt.Sprintf("/%s", p.segments[0])
	default:
		return fmt.Sprintf("%s/%s", p.Parent().String(), p.Tail())
	}
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (p *Path) GobEncode() (data []byte, err error) {
	var buffer bytes.Buffer
	//
	gobEncoder := gob.NewEncoder(&buffer)
	// absolute flag
	if err := gobEncoder.Encode(&p.absolute); err != nil {
		return nil, err
	}
	// segments
	if err := gobEncoder.Encode(&p.segments); err != nil {
		return nil, err
	}
	// Success
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (p *Path) GobDecode(data []byte) error {
	buffer := bytes.NewBuffer(data)
	gobDecoder := gob.NewDecoder(buffer)
	// absolute flag
	if err := gobDecoder.Decode(&p.absolute); err != nil {
		return err
	}
	// segments
	if err := gobDecoder.Decode(&p.segments); err != nil {
		return err
	}
	// Success!
	return nil
}
