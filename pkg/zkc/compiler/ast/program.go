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
package ast

import (
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/data"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// RawProgram encapsulates one of more functions together, such that one may call
// another, etc.  Furthermore, it provides an interface between assembly
// components and the notion of a Schema.
type RawProgram[I any] struct {
	declarations []decl.Declaration[I]
}

// Component returns the ith entity in this program.
func (p *RawProgram[I]) Component(id uint) decl.Declaration[I] {
	return p.declarations[id]
}

// Components returns all functions making up this program.
func (p *RawProgram[I]) Components() []decl.Declaration[I] {
	return p.declarations
}

// Program represents a program whose declarations contain only resolved
// external identifiers. As such, it should not be possible that any
// declarations contained within refer to unknown (or otherwise incorrect)
// external components.
type Program struct {
	RawProgram[symbol.Resolved]
	srcmaps source.Maps[any]
}

// NewProgram constructs a new program using a given level of instruction.
func NewProgram(components []decl.Resolved, srcmaps source.Maps[any]) Program {
	//
	decls := make([]decl.Resolved, len(components))
	copy(decls, components)

	return Program{RawProgram[symbol.Resolved]{decls}, srcmaps}
}

// Environment creates a fresh environment for this program
func (p *Program) Environment() Environment {
	return NewEnvironment(p.declarations...)
}

// DecodeInputsOutputs configures a given set of input / output bytes appropriately
// for the boot program, whilst separating inputs from outputs.  If there are
// unknown or conflicting inputs / outputs, then errors are returned.
func (p *Program) DecodeInputsOutputs(input map[string][]byte) (inputs, outputs map[string][]vm.Uint, errs []error) {
	//
	var (
		visited = make(map[string]bool)
		env     = p.Environment()
	)
	// Initialise inputs / outputs
	inputs = make(map[string][]vm.Uint)
	outputs = make(map[string][]vm.Uint)
	// Initialise components
	for _, c := range p.declarations {
		switch c := c.(type) {
		case *decl.ResolvedFunction:
			// ignore
		case *decl.ResolvedConstant:
			// ignore
		case *decl.ResolvedTypeAlias:
			// ignore
		case *decl.ResolvedMemory:
			// Record this memory has seen
			visited[c.Name()] = true
			//
			switch c.Kind {
			case decl.PRIVATE_READ_ONLY_MEMORY, decl.PUBLIC_READ_ONLY_MEMORY:
				if bytes, ok := input[c.Name()]; ok {
					inputs[c.Name()] = data.DecodeAll(variable.DescriptorsToType(c.Data...), bytes, env)
				}
			case decl.PRIVATE_WRITE_ONCE_MEMORY, decl.PUBLIC_WRITE_ONCE_MEMORY:
				if bytes, ok := input[c.Name()]; ok {
					outputs[c.Name()] = data.DecodeAll(variable.DescriptorsToType(c.Data...), bytes, env)
				}
			default:
				if _, ok := input[c.Name()]; ok {
					errs = append(errs, fmt.Errorf("unexpected input \"%s\"", c.Name()))
				}
			}
		default:
			panic(fmt.Sprintf("unknown declaration %s", c.Name()))
		}
	}
	// Sanity check for extraneous inputs
	for k := range input {
		if _, ok := visited[k]; !ok {
			errs = append(errs, fmt.Errorf("unknown input/output \"%s\"", k))
		}
	}
	//
	return inputs, outputs, errs
}

// EncodeInputsOutputs encodes a given set of input / output word values back
// into raw bytes, producing the inverse of DecodeInputsOutputs.  If there are
// unknown or conflicting entries, then errors are returned.
func (p *Program) EncodeInputsOutputs(values map[string][]vm.Uint) (map[string][]byte, []error) {
	var (
		visited = make(map[string]bool)
		env     = p.Environment()
		errs    []error
	)

	result := make(map[string][]byte)

	for _, c := range p.declarations {
		switch c := c.(type) {
		case *decl.ResolvedFunction:
			// ignore
		case *decl.ResolvedConstant:
			// ignore
		case *decl.ResolvedTypeAlias:
			// ignore
		case *decl.ResolvedMemory:
			visited[c.Name()] = true

			switch c.Kind {
			case decl.PRIVATE_READ_ONLY_MEMORY, decl.PUBLIC_READ_ONLY_MEMORY,
				decl.PRIVATE_WRITE_ONCE_MEMORY, decl.PUBLIC_WRITE_ONCE_MEMORY:
				if words, ok := values[c.Name()]; ok {
					result[c.Name()] = data.EncodeAll(variable.DescriptorsToType(c.Data...), words, env)
				}
			default:
				if _, ok := values[c.Name()]; ok {
					errs = append(errs, fmt.Errorf("unexpected input/output \"%s\"", c.Name()))
				}
			}
		default:
			panic(fmt.Sprintf("unknown declaration %s", c.Name()))
		}
	}
	// Sanity check for extraneous entries
	for k := range values {
		if _, ok := visited[k]; !ok {
			errs = append(errs, fmt.Errorf("unknown input/output \"%s\"", k))
		}
	}

	return result, errs
}

// Compile attempts to compile a given high-level program into a low-level
// machine which can be used (for example) to execute this program with some
// given inputs.
func (p *Program) Compile(config codegen.Config) (*vm.WordMachine[vm.Uint], []source.SyntaxError) {
	var compiler = codegen.NewCompiler(config, p.Environment(), p.srcmaps)
	// Compile all decalarations
	return compiler.Compile(p.declarations)
}
