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

// Intrinsic performs a built-in operation identified by Op, reading a variable
// number of arguments (Sources) and writing a variable number of returns
// (Targets), where each argument and return is a register vector.  The
// supported operations are:
//
//   - DIV_HINT, which reads exactly two arguments (dividend, divisor) and
//     writes exactly three returns (quotient, remainder, range witness) for a
//     division hint (i.e. as produced by the LowerDivisions transform).
//     Specifically, quotient = dividend / divisor, remainder = dividend %
//     divisor and witness = divisor - remainder - 1, with correctness validated
//     by subsequent arithmetic checks.  A zero divisor aborts execution with a
//     division-by-zero error.
//   - WIDE_SHL, which reads exactly two arguments (value, shift amount) and
//     writes exactly one return (result), computing result = value << shift
//     truncated to the total bitwidth of the target vector.  This mirrors the
//     Bitwise SHL instruction but operates over vectored (multi-limb) operands.
//   - WIDE_SHR, which reads exactly two arguments (value, shift amount) and
//     writes exactly one return (result), computing result = value >> shift
//     truncated to the total bitwidth of the target vector.  This mirrors the
//     Bitwise SHR instruction but operates over vectored (multi-limb) operands.
//   - WIDE_DIV, which reads exactly two arguments (dividend, divisor) and writes
//     exactly one return (quotient), computing quotient = dividend / divisor.
//     This mirrors the DIV instruction but operates over vectored (multi-limb)
//     operands.  A zero divisor aborts execution with a division-by-zero error.
//   - WIDE_REM, which reads exactly two arguments (dividend, divisor) and writes
//     exactly one return (remainder), computing remainder = dividend % divisor.
//     This mirrors the REM instruction but operates over vectored (multi-limb)
//     operands.  A zero divisor aborts execution with a division-by-zero error.
type Intrinsic[W word.Word[W]] struct {
	// Op selects the intrinsic operation (e.g. DIV_HINT or WIDE_SHL).
	Op Operation
	// Targets receive the results (returns) written by this intrinsic.
	Targets []RegisterVector
	// Sources are the arguments read by this intrinsic, each either a register
	// vector or a (possibly multi-limb) constant.
	Sources []Operand[W]
}

// NewIntrinsic constructs an intrinsic instruction performing the given operation
// op (e.g. DIV_HINT) which reads the given source (argument) operands and
// writes the given target (return) register vectors.
func NewIntrinsic[W word.Word[W]](op Operation, targets []RegisterVector, sources []Operand[W]) *Intrinsic[W] {
	return &Intrinsic[W]{Op: op, Targets: targets, Sources: sources}
}

// Uses implementation for Bytecode interface.
func (p *Intrinsic[W]) Uses() []RegisterId {
	var uses []RegisterId
	//
	for _, s := range p.Sources {
		if s.IsRegisterVector() {
			uses = append(uses, s.AsRegisters()...)
		}
	}
	//
	return uses
}

// Definitions implementation for Bytecode interface.
func (p *Intrinsic[W]) Definitions() []RegisterId {
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
func (p *Intrinsic[W]) Validate(_ FieldConfig, env Environment[W]) []error {
	errs := validateOperands(env, p.Uses(), p.Definitions())
	// Constant sources must be a single value of any width (limb widths are
	// not recorded, so a multi-limb run could not be reconstructed).
	for _, s := range p.Sources {
		if s.IsConstant() && len(s.AsConstants()) != 1 {
			errs = append(errs, fmt.Errorf("%d constant operand(s) found, expected 1", len(s.AsConstants())))
		}
	}
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
	case WIDE_SHL:
		if len(p.Sources) != 2 {
			errs = append(errs, fmt.Errorf("wide shl hint expects 2 arguments (found %d)", len(p.Sources)))
		}
		//
		if len(p.Targets) != 1 {
			errs = append(errs, fmt.Errorf("wide shl hint expects 1 return (found %d)", len(p.Targets)))
		}
	case WIDE_SHR:
		if len(p.Sources) != 2 {
			errs = append(errs, fmt.Errorf("wide shr hint expects 2 arguments (found %d)", len(p.Sources)))
		}
		//
		if len(p.Targets) != 1 {
			errs = append(errs, fmt.Errorf("wide shr hint expects 1 return (found %d)", len(p.Targets)))
		}
	case WIDE_DIV:
		if len(p.Sources) != 2 {
			errs = append(errs, fmt.Errorf("wide div hint expects 2 arguments (found %d)", len(p.Sources)))
		}
		//
		if len(p.Targets) != 1 {
			errs = append(errs, fmt.Errorf("wide div hint expects 1 return (found %d)", len(p.Targets)))
		}
	case WIDE_REM:
		if len(p.Sources) != 2 {
			errs = append(errs, fmt.Errorf("wide rem hint expects 2 arguments (found %d)", len(p.Sources)))
		}
		//
		if len(p.Targets) != 1 {
			errs = append(errs, fmt.Errorf("wide rem hint expects 1 return (found %d)", len(p.Targets)))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported hint operation (%d)", p.Op))
	}
	//
	return errs
}

func (p *Intrinsic[W]) String(env Environment[W]) string {
	var (
		name    = intrinsicName(p.Op)
		targets = registerVectorsToString(p.Targets, env, ",")
		sources strings.Builder
	)
	//
	for i, s := range p.Sources {
		if i != 0 {
			sources.WriteString(", ")
		}
		//
		sources.WriteString(s.String(env))
	}
	//
	return fmt.Sprintf("%s = hint:%s(%s)", targets, name, sources.String())
}

// intrinsicName returns a human-readable name for the given hint operation op.
// Currently only DIV_HINT, WIDE_SHL, WIDE_SHR, WIDE_DIV and WIDE_REM are
// supported; any other operation panics.
func intrinsicName(op Operation) string {
	switch op {
	case DIV_HINT:
		return "divrem"
	case WIDE_SHL:
		return "shl"
	case WIDE_SHR:
		return "shr"
	case WIDE_DIV:
		return "div"
	case WIDE_REM:
		return "rem"
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
