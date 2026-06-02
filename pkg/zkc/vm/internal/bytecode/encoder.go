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

import "github.com/consensys/go-corset/pkg/schema/register"

type Encoder struct {
	bytecodes []uint32
}

func (p *Encoder) Fail() {
	panic("todo")
}

func (p *Encoder) Jump(target uint32) {
	panic("todo")
}

func (p *Encoder) JumpIf(target uint32, condition Condition, left, right register.Id) {
	panic("todo")
}

func (p *Encoder) Call(target uint32, inputs uint) {
	panic("todo")
}

func (p *Encoder) Ret(target uint32, ninputs, width uint) {
	panic("todo")
}
