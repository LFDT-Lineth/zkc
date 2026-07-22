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
package constraints

import (
	mirc "github.com/LFDT-Lineth/zkc/pkg/asm/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro/dfa"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// TranslateBranchCondition translates a given branch condition within the
// context of a given state reader.
func TranslateBranchCondition[W vm.Word[W], F field.Element[F], E Expr[F]](p dfa.Path[W],
	reader RegisterReader[F]) Expr[F] {
	//
	return mirc.TranslateBranchCondition(p.Condition(), reader)
}

// TranslateNegatedBranchCondition translates a negated branch condition within the
// context of a given state reader.
func TranslateNegatedBranchCondition[W vm.Word[W], F field.Element[F], E Expr[F]](p dfa.Path[W],
	reader RegisterReader[F]) Expr[F] {
	//
	var condition = p.Condition()
	//
	return mirc.TranslateBranchCondition(condition.Negate(), reader)
}
