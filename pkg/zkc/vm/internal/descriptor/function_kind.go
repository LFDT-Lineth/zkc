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
package descriptor

import (
	"bytes"
	"encoding/gob"
)

// FunctionKind captures the execution-relevant properties of a function.
type FunctionKind struct {
	native, unsafeArgs, inline bool
}

// IsNative reports whether this function is backed by a native circuit rather
// than bytecode instructions.
func (p FunctionKind) IsNative() bool {
	return p.native
}

// CanInline reports whether or not this function was marked as inlineable or
// not.
func (p FunctionKind) CanInline() bool {
	return p.inline
}

// AllowsUnsafeArgs reports whether calls may supply arguments which are undefined
// on some paths reaching the call.
func (p FunctionKind) AllowsUnsafeArgs() bool {
	return p.unsafeArgs
}

// WithInline updates this kind as to whether it was marked as inlineable or
// not.
func (p FunctionKind) WithInline(flag bool) FunctionKind {
	p.inline = flag
	//
	return p
}

// WithNative updates this kind as to whether it represents a native function,
// or not.
func (p FunctionKind) WithNative(flag bool) FunctionKind {
	p.native = flag
	//
	return p
}

// WithUnsafeArgs updates the specification as to whether this support unsafe
// args, or not.
func (p FunctionKind) WithUnsafeArgs(flag bool) FunctionKind {
	p.unsafeArgs = flag
	//
	return p
}

// GobEncode marshals this function kind.
//
// nolint
func (p *FunctionKind) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.native); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.unsafeArgs); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// GobDecode unmarshals this function kind.
//
// nolint
func (p *FunctionKind) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.native); err != nil {
		return err
	}
	//
	return gobDecoder.Decode(&p.unsafeArgs)
}

var (
	// BYTECODE_FUNCTION represents a safe function implemented by bytecode.
	BYTECODE_FUNCTION = FunctionKind{false, false, false}
)
