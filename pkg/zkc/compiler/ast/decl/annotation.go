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
package decl

// DeclarationKind identifies the kind of a top-level declaration.  The values
// are bit flags so they can be combined with bitwise OR when an annotation is
// permitted on more than one kind of declaration.
type DeclarationKind uint

const (
	// FUNCTION_KIND identifies a function declaration.
	FUNCTION_KIND DeclarationKind = 1 << iota
	// MEMORY_KIND identifies a memory declaration.
	MEMORY_KIND
	// CONSTANT_KIND identifies a constant declaration.
	CONSTANT_KIND
	// TYPE_ALIAS_KIND identifies a type alias declaration.
	TYPE_ALIAS_KIND
	// INCLUDE_KIND identifies an include declaration.
	INCLUDE_KIND
)

// Annotation is a schema for a source-level annotation.  It records the
// annotation's name (without the surrounding "#[" and "]") and the set of
// declaration kinds on which it may legally appear.
type Annotation struct {
	// name is the annotation identifier (e.g. "inline" for "#[inline]").
	name string
	// description explains what the annotation is for, used in documentation
	// and error messages.
	description string
	// permitted is a bitmask of the DeclarationKind values on which this
	// annotation is allowed.
	permitted DeclarationKind
}

// NewAnnotation constructs an Annotation schema with the given name,
// description, and the set of declaration kinds on which it is permitted.
func NewAnnotation(name, description string, permitted DeclarationKind) Annotation {
	return Annotation{name: name, description: description, permitted: permitted}
}

// Name returns the annotation's name without the surrounding "#[" and "]".
func (a Annotation) Name() string {
	return a.name
}

// Description returns a human-readable description of what the annotation is
// for.
func (a Annotation) Description() string {
	return a.description
}

// Permits reports whether this annotation is allowed on a declaration of the
// given kind.
func (a Annotation) Permits(kind DeclarationKind) bool {
	return a.permitted&kind != 0
}

// String returns a human-readable label for a DeclarationKind, used in error
// messages.
func (k DeclarationKind) String() string {
	switch k {
	case FUNCTION_KIND:
		return "function"
	case MEMORY_KIND:
		return "memory"
	case CONSTANT_KIND:
		return "constant"
	case TYPE_ALIAS_KIND:
		return "type alias"
	case INCLUDE_KIND:
		return "include"
	default:
		return "unknown"
	}
}

// ANNOTATIONS is the global registry of known annotations.  Each entry
// describes one valid annotation and the declaration kinds on which it may
// appear.
var ANNOTATIONS = []Annotation{
	// #[inline] is permitted only on function declarations.
	NewAnnotation("inline", "marks a function to be inlined at every call site", FUNCTION_KIND),
	// #[native] is permitted only on function declarations.
	NewAnnotation("native", "marks a function as backed by a native circuit", FUNCTION_KIND),
	// #[debug] is permitted only on function declarations.
	NewAnnotation("debug",
		"marks a function as debug-only, whose calls are elided (like printf) in quiet mode", FUNCTION_KIND),
	// #[bipartite] is permitted only on memory declarations.
	NewAnnotation("bipartite",
		"marks a read/write memory to use the bipartite (split heap/stack) layout", MEMORY_KIND),
}
