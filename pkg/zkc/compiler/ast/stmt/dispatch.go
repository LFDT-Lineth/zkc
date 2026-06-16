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
package stmt

import (
	"fmt"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/expr"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
)

// Dispatch is a flattened multi-way conditional branch, produced when lowering a
// switch statement with multiway-skip code generation enabled.  It compares the
// Discriminant against the (constant) labels of each branch and transfers
// control to that branch's Target; if no label matches, control transfers to
// DefaultTarget.  The branch bodies themselves are emitted as ordinary
// statements following this one (reached via the recorded targets), exactly as
// for the equivalent if-goto chain.
type Dispatch[S symbol.Symbol[S]] struct {
	// Discriminant is the value switched upon.
	Discriminant expr.Expr[S]
	// Branches gives, in order, the constant labels of each (non-default) case
	// together with the target PC of that case's body.
	Branches []DispatchBranch[S]
	// DefaultTarget is the PC reached when no label matches: the default case
	// body, or the instruction following the switch when there is no default.
	DefaultTarget uint
}

// DispatchBranch records the labels selecting a single case together with the
// target PC of that case's body.
type DispatchBranch[S symbol.Symbol[S]] struct {
	// Labels are the (constant) case values selecting this branch.
	Labels []expr.Expr[S]
	// Target is the PC of this branch's body.
	Target uint
}

// Uses implementation for Stmt interface.
func (p *Dispatch[S]) Uses() []variable.Id {
	var (
		reads []variable.Id
		bits  = p.Discriminant.LocalUses()
	)
	// Case labels are constants, so only the discriminant reads variables.
	for iter := bits.Iter(); iter.HasNext(); {
		reads = append(reads, iter.Next())
	}
	//
	return reads
}

// Definitions implementation for Stmt interface.
func (p *Dispatch[S]) Definitions() []variable.Id {
	return nil
}

func (p *Dispatch[S]) String(env variable.Map[S]) string {
	var b strings.Builder
	//
	b.WriteString("switch ")
	b.WriteString(p.Discriminant.String(env))
	b.WriteString(" [")
	//
	for i, branch := range p.Branches {
		if i != 0 {
			b.WriteString(", ")
		}
		//
		for j, label := range branch.Labels {
			if j != 0 {
				b.WriteString("|")
			}
			//
			b.WriteString(label.String(env))
		}
		//
		fmt.Fprintf(&b, "->%d", branch.Target)
	}
	//
	fmt.Fprintf(&b, ", default->%d]", p.DefaultTarget)
	//
	return b.String()
}
