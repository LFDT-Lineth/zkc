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
package bytecode

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Hint performs a built-in operation identified by Op, reading a variable
// number of arguments (Sources) and writing a variable number of returns
// (Targets), where each argument and return is a register vector.  Currently
// the only supported operation is DIV_HINT, which reads exactly two arguments
// (dividend, divisor) and writes exactly three returns (quotient, remainder,
// range witness) for a division hint (i.e. as produced by the LowerDivisions
// transform).  Specifically, quotient = dividend / divisor, remainder =
// dividend % divisor and witness = divisor - remainder - 1, with correctness
// validated by subsequent arithmetic checks.  A zero divisor aborts execution
// with a division-by-zero error.
type Hint[W word.Word[W]] struct {
	// Op selects the hint operation (currently only DIV_HINT).
	Op Operation
	// Targets receive the results (returns) written by this hint.
	Targets []RegisterVector
	// Sources are the argument register vectors read by this hint.
	Sources []RegisterVector
}

// NewHint constructs a hint instruction performing the given operation op (e.g.
// DIV_HINT) which reads the given source (argument) register vectors and writes
// the given target (return) register vectors.
func NewHint[W word.Word[W]](op Operation, targets, sources []RegisterVector) *Hint[W] {
	return &Hint[W]{Op: op, Targets: targets, Sources: sources}
}

// Uses implementation for Bytecode interface.
func (p *Hint[W]) Uses() []RegisterId {
	var uses []RegisterId
	//
	for _, s := range p.Sources {
		uses = append(uses, s.Registers()...)
	}
	//
	return uses
}

// Definitions implementation for Bytecode interface.
func (p *Hint[W]) Definitions() []RegisterId {
	var defs []RegisterId
	//
	for _, t := range p.Targets {
		defs = append(defs, t.Registers()...)
	}
	//
	return defs
}

// Validate implementation for Bytecode interface.  This checks that the number
// of arguments and returns matches what the selected operation expects.
func (p *Hint[W]) Validate(_ uint, _ FieldConfig, _ Environment[W]) []error {
	var errs []error
	//
	switch p.Op {
	case DIV_HINT:
		if len(p.Sources) != 2 {
			errs = append(errs, fmt.Errorf("div hint expects 2 arguments (found %d)", len(p.Sources)))
		}
		//
		if len(p.Targets) != 3 {
			errs = append(errs, fmt.Errorf("div hint expects 3 returns (found %d)", len(p.Targets)))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported hint operation (%d)", p.Op))
	}
	//
	return errs
}

func (p *Hint[W]) String(env Environment[W]) string {
	var (
		name    = hintName(p.Op)
		targets = registerVectorsToString(p.Targets, env, ",")
		sources = registerVectorsToString(p.Sources, env, ", ")
	)
	//
	return fmt.Sprintf("%s = hint:%s(%s)", targets, name, sources)
}

// hintName returns a human-readable name for the given hint operation op.
// Currently only DIV_HINT is supported; any other operation panics.
func hintName(op Operation) string {
	switch op {
	case DIV_HINT:
		return "divrem"
	default:
		panic("unsupported operation")
	}
}

// registerVectorsToString formats a slice of register vectors, joining their individual
// representations with the given separator.
func registerVectorsToString[W word.Word[W]](vecs []RegisterVector, env Environment[W], separator string) string {
	var builder strings.Builder
	//
	for i, v := range vecs {
		if i != 0 {
			builder.WriteString(separator)
		}
		//
		builder.WriteString(RegisterVectorToString(v, env))
	}
	//
	return builder.String()
}
